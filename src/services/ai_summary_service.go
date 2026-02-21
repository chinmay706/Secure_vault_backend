package services

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"securevault-backend/src/models"

	"github.com/google/uuid"
	"github.com/ledongthuc/pdf"
)

const (
	summaryDailyLimit              = 20
	summaryRefinementCost          = 0.5
	SummaryRefinementCostExported  = summaryRefinementCost
	summaryMaxHistoryDepth         = 10
	summaryMaxTokenChars    = 120000 // ~30K tokens at ~4 chars/token for 32K context
	summaryMaxPDFSize       = 20 * 1024 * 1024
	summaryMaxTextSize      = 10 * 1024 * 1024
	defaultGroqSummaryModel = "groq/compound"
	summaryChunkSize        = 100000 // chars per chunk for map-reduce
	summaryChunkOverlap     = 500
)

type AiSummaryResult struct {
	Summary         string   `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

type AiSummaryService struct {
	db             *sql.DB
	storageService *StorageService
	groqAPIKey     string
	groqModel      string
	httpClient     *http.Client
}

func NewAiSummaryService(db *sql.DB, storageService *StorageService, groqAPIKey, groqModel string) *AiSummaryService {
	if groqModel == "" {
		groqModel = defaultGroqSummaryModel
	}
	return &AiSummaryService{
		db:             db,
		storageService: storageService,
		groqAPIKey:     groqAPIKey,
		groqModel:      groqModel,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (s *AiSummaryService) IsEnabled() bool {
	return s.groqAPIKey != ""
}

// GetSummary returns the existing AI summary for a file+user pair, or nil if none exists.
func (s *AiSummaryService) GetSummary(fileID, userID uuid.UUID) (*models.AiSummary, error) {
	var summary models.AiSummary
	var recs pgStringArray
	err := s.db.QueryRow(`
		SELECT id, file_id, user_id, summary, recommendations, status, error_message, history, created_at, updated_at
		FROM ai_summaries
		WHERE file_id = $1 AND user_id = $2
	`, fileID, userID).Scan(
		&summary.ID, &summary.FileID, &summary.UserID,
		&summary.Summary, &recs, &summary.Status,
		&summary.ErrorMessage, &summary.History,
		&summary.CreatedAt, &summary.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query ai_summaries: %w", err)
	}
	summary.Recommendations = []string(recs)
	if summary.Recommendations == nil {
		summary.Recommendations = []string{}
	}
	if summary.History == nil {
		summary.History = json.RawMessage("[]")
	}
	return &summary, nil
}

// GenerateSummary runs asynchronously to create a new summary for a file.
func (s *AiSummaryService) GenerateSummary(fileID, userID uuid.UUID) {
	log.Printf("[AI-SUMMARY] Starting summary generation for file %s, user %s", fileID, userID)

	fail := func(msg string) {
		log.Printf("[AI-SUMMARY] Failed: %s", msg)
		s.db.Exec(`
			UPDATE ai_summaries SET status = 'failed', error_message = $1, updated_at = NOW()
			WHERE file_id = $2 AND user_id = $3
		`, msg, fileID, userID)
	}

	// Upsert the job row as processing
	_, err := s.db.Exec(`
		INSERT INTO ai_summaries (file_id, user_id, status)
		VALUES ($1, $2, 'processing')
		ON CONFLICT (file_id, user_id) DO UPDATE SET
			status = 'processing', error_message = '', updated_at = NOW()
	`, fileID, userID)
	if err != nil {
		log.Printf("[AI-SUMMARY] Failed to upsert job: %v", err)
		return
	}

	// Fetch file metadata
	var blobHash, mimeType, filename string
	var sizeBytes int64
	var ownerID uuid.UUID
	err = s.db.QueryRow(`
		SELECT blob_hash, mime_type, original_filename, size_bytes, owner_id
		FROM files WHERE id = $1 AND deleted_at IS NULL
	`, fileID).Scan(&blobHash, &mimeType, &filename, &sizeBytes, &ownerID)
	if err != nil {
		fail(fmt.Sprintf("file not found: %v", err))
		return
	}

	if ownerID != userID {
		fail("access denied: not file owner")
		return
	}

	// Extract text content from the file
	text, err := s.extractText(blobHash, mimeType, filename, sizeBytes)
	if err != nil {
		fail(fmt.Sprintf("text extraction failed: %v", err))
		return
	}

	if strings.TrimSpace(text) == "" {
		fail("no text content could be extracted from this file")
		return
	}

	// Summarize (with chunking for large docs)
	var result *AiSummaryResult
	if len(text) > summaryMaxTokenChars {
		result, err = s.summarizeChunked(text, filename)
	} else {
		result, err = s.summarizeDirect(text, filename)
	}
	if err != nil {
		fail(fmt.Sprintf("AI summarization failed: %v", err))
		return
	}

	recsVal, _ := pgStringArray(result.Recommendations).Value()

	_, err = s.db.Exec(`
		UPDATE ai_summaries
		SET summary = $1, recommendations = $2, status = 'completed', error_message = '', updated_at = NOW()
		WHERE file_id = $3 AND user_id = $4
	`, result.Summary, recsVal, fileID, userID)
	if err != nil {
		log.Printf("[AI-SUMMARY] Failed to store result: %v", err)
		return
	}

	log.Printf("[AI-SUMMARY] Completed summary for file %s (%d chars summary, %d recommendations)",
		fileID, len(result.Summary), len(result.Recommendations))
}

// RefineSummary updates an existing summary based on user instructions (synchronous).
func (s *AiSummaryService) RefineSummary(fileID, userID uuid.UUID, command string) (*models.AiSummary, error) {
	existing, err := s.GetSummary(fileID, userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("no summary found for this file")
	}
	if existing.Status != "completed" {
		return nil, fmt.Errorf("summary is not in completed state (current: %s)", existing.Status)
	}

	// Push current summary into history
	var history []models.SummaryHistoryEntry
	if err := json.Unmarshal(existing.History, &history); err != nil {
		history = []models.SummaryHistoryEntry{}
	}
	history = append(history, models.SummaryHistoryEntry{
		Summary:   existing.Summary,
		Command:   command,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if len(history) > summaryMaxHistoryDepth {
		history = history[len(history)-summaryMaxHistoryDepth:]
	}
	historyJSON, _ := json.Marshal(history)

	// Build refinement prompt
	prompt := fmt.Sprintf(`You are a document analyst. Here is the current summary of a document:

---
%s
---

The user has requested the following change:
%s

Provide an updated summary that incorporates the user's request. Keep the summary in markdown format.
Also provide updated recommendations if they have changed.

Respond in JSON format only, no markdown fences:
{"summary": "...", "recommendations": ["...", "..."]}`, existing.Summary, command)

	result, err := s.callGroqForSummary(prompt)
	if err != nil {
		return nil, fmt.Errorf("refinement failed: %w", err)
	}

	recsVal, _ := pgStringArray(result.Recommendations).Value()

	_, err = s.db.Exec(`
		UPDATE ai_summaries
		SET summary = $1, recommendations = $2, history = $3, updated_at = NOW()
		WHERE file_id = $4 AND user_id = $5
	`, result.Summary, recsVal, string(historyJSON), fileID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to store refined summary: %w", err)
	}

	return s.GetSummary(fileID, userID)
}

// CheckRateLimit returns nil if the user is within their daily summary limit.
// cost should be 1.0 for a full summary or summaryRefinementCost for refinements.
func (s *AiSummaryService) CheckRateLimit(userID uuid.UUID, cost float64) error {
	var totalCost float64
	err := s.db.QueryRow(`
		SELECT COALESCE(SUM(
			CASE WHEN jsonb_array_length(history) > prev_len THEN 0.5 ELSE 1.0 END
		), 0)
		FROM ai_summaries
		WHERE user_id = $1 AND updated_at > NOW() - INTERVAL '24 hours'
	`, userID).Scan(&totalCost)
	if err != nil {
		// On error, count rows as a simpler fallback
		var count int
		s.db.QueryRow(`
			SELECT COUNT(*) FROM ai_summaries
			WHERE user_id = $1 AND created_at > NOW() - INTERVAL '24 hours'
		`, userID).Scan(&count)
		totalCost = float64(count)
	}

	if totalCost+cost > float64(summaryDailyLimit) {
		return fmt.Errorf("daily summary limit reached (%d/day)", summaryDailyLimit)
	}
	return nil
}

// HasExistingCompleted checks if a completed summary already exists.
func (s *AiSummaryService) HasExistingCompleted(fileID, userID uuid.UUID) bool {
	var status string
	err := s.db.QueryRow(`
		SELECT status FROM ai_summaries WHERE file_id = $1 AND user_id = $2
	`, fileID, userID).Scan(&status)
	return err == nil && status == "completed"
}

// ============================================================
// Text extraction
// ============================================================

func (s *AiSummaryService) extractText(blobHash, mimeType, filename string, sizeBytes int64) (string, error) {
	switch {
	case mimeType == "application/pdf" && sizeBytes <= summaryMaxPDFSize:
		return s.extractPDFText(blobHash)
	case mimeType == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" && sizeBytes <= summaryMaxTextSize:
		return s.extractDOCXText(blobHash)
	case isTextualMimeType(mimeType) && sizeBytes <= summaryMaxTextSize:
		return s.extractRawText(blobHash)
	case strings.HasSuffix(strings.ToLower(filename), ".md") ||
		strings.HasSuffix(strings.ToLower(filename), ".txt") ||
		strings.HasSuffix(strings.ToLower(filename), ".csv"):
		return s.extractRawText(blobHash)
	default:
		return "", fmt.Errorf("unsupported file type for summarization: %s (%s)", mimeType, filename)
	}
}

func (s *AiSummaryService) extractRawText(blobHash string) (string, error) {
	reader, err := s.storageService.DownloadFile(blobHash)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(io.LimitReader(reader, summaryMaxTextSize))
	if err != nil {
		return "", fmt.Errorf("read failed: %w", err)
	}
	return string(data), nil
}

func (s *AiSummaryService) extractPDFText(blobHash string) (string, error) {
	reader, err := s.storageService.DownloadFile(blobHash)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(io.LimitReader(reader, summaryMaxPDFSize))
	if err != nil {
		return "", fmt.Errorf("read failed: %w", err)
	}

	pdfReader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("PDF parse failed: %w", err)
	}

	var sb strings.Builder
	for i := 1; i <= pdfReader.NumPage(); i++ {
		page := pdfReader.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		sb.WriteString(text)
		sb.WriteString("\n")
	}

	result := sb.String()
	if strings.TrimSpace(result) == "" {
		return "", fmt.Errorf("PDF contains no extractable text (may be scanned/image-based)")
	}
	return result, nil
}

func (s *AiSummaryService) extractDOCXText(blobHash string) (string, error) {
	reader, err := s.storageService.DownloadFile(blobHash)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(io.LimitReader(reader, summaryMaxTextSize))
	if err != nil {
		return "", fmt.Errorf("read failed: %w", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("DOCX zip open failed: %w", err)
	}

	for _, file := range zipReader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return "", fmt.Errorf("DOCX document.xml open failed: %w", err)
		}
		defer rc.Close()

		xmlData, err := io.ReadAll(rc)
		if err != nil {
			return "", fmt.Errorf("DOCX document.xml read failed: %w", err)
		}

		return stripXMLTags(string(xmlData)), nil
	}

	return "", fmt.Errorf("DOCX missing word/document.xml")
}

// stripXMLTags removes XML tags and returns just the text content.
func stripXMLTags(xmlContent string) string {
	var sb strings.Builder
	inTag := false
	for _, ch := range xmlContent {
		switch {
		case ch == '<':
			inTag = true
		case ch == '>':
			inTag = false
		case !inTag:
			sb.WriteRune(ch)
		}
	}
	// Collapse multiple whitespace runs
	result := sb.String()
	for strings.Contains(result, "  ") {
		result = strings.ReplaceAll(result, "  ", " ")
	}
	return strings.TrimSpace(result)
}

// ============================================================
// Groq API for summaries
// ============================================================

func (s *AiSummaryService) callGroqForSummary(prompt string) (*AiSummaryResult, error) {
	apiURL := "https://api.groq.com/openai/v1/chat/completions"

	payload := map[string]interface{}{
		"model": s.groqModel,
		"messages": []map[string]interface{}{
			{
				"role": "system",
				"content": "You are a document analyst. Always respond with valid JSON only, no markdown fences or extra text.",
			},
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": 0.3,
		"max_tokens":  4096,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal failed: %w", err)
	}

	maxRetries := 3
	var resp *http.Response
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("request creation failed: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+s.groqAPIKey)

		resp, err = s.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("Groq API call failed: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			if attempt < maxRetries {
				backoff := time.Duration(5*(attempt+1)) * time.Second
				log.Printf("[AI-SUMMARY] Groq rate limited (429), retrying in %v (attempt %d/%d)", backoff, attempt+1, maxRetries)
				time.Sleep(backoff)
				continue
			}
			return nil, fmt.Errorf("Groq API rate limit exceeded after %d retries", maxRetries)
		}
		break
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Groq API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var groqResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&groqResp); err != nil {
		return nil, fmt.Errorf("Groq response decode failed: %w", err)
	}
	if len(groqResp.Choices) == 0 {
		return nil, fmt.Errorf("empty Groq response")
	}

	return parseSummaryResponse(groqResp.Choices[0].Message.Content)
}

func parseSummaryResponse(text string) (*AiSummaryResult, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var result AiSummaryResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		start := strings.Index(text, "{")
		end := strings.LastIndex(text, "}")
		if start >= 0 && end > start {
			if err2 := json.Unmarshal([]byte(text[start:end+1]), &result); err2 != nil {
				return nil, fmt.Errorf("failed to parse summary JSON: %w (raw: %s)", err2, text[:min(200, len(text))])
			}
		} else {
			return nil, fmt.Errorf("no JSON found in response: %s", text[:min(200, len(text))])
		}
	}

	if result.Recommendations == nil {
		result.Recommendations = []string{}
	}
	return &result, nil
}

// ============================================================
// Direct and chunked summarization
// ============================================================

func (s *AiSummaryService) summarizeDirect(text, filename string) (*AiSummaryResult, error) {
	prompt := fmt.Sprintf(`Analyze the following document and provide:
1. A comprehensive summary in markdown format
2. A list of key recommendations or notable points

Document filename: "%s"
Document content:

%s

Respond in JSON format only, no markdown fences:
{"summary": "...", "recommendations": ["...", "..."]}`, filename, text)

	return s.callGroqForSummary(prompt)
}

func (s *AiSummaryService) summarizeChunked(text, filename string) (*AiSummaryResult, error) {
	chunks := splitIntoChunks(text, summaryChunkSize, summaryChunkOverlap)
	log.Printf("[AI-SUMMARY] Document too large for single call, splitting into %d chunks", len(chunks))

	var chunkSummaries []string
	for i, chunk := range chunks {
		prompt := fmt.Sprintf(`Summarize the following section (part %d of %d) of the document "%s".
Provide a concise summary of this section's key points.

Content:
%s

Respond in JSON format only, no markdown fences:
{"summary": "...", "recommendations": []}`, i+1, len(chunks), filename, chunk)

		result, err := s.callGroqForSummary(prompt)
		if err != nil {
			return nil, fmt.Errorf("chunk %d/%d failed: %w", i+1, len(chunks), err)
		}
		chunkSummaries = append(chunkSummaries, result.Summary)

		if i < len(chunks)-1 {
			time.Sleep(1 * time.Second)
		}
	}

	// Reduce: combine chunk summaries
	combined := strings.Join(chunkSummaries, "\n\n---\n\n")
	reducePrompt := fmt.Sprintf(`Below are summaries of different sections of the document "%s".
Combine them into one comprehensive summary in markdown format, and provide key recommendations.

Section summaries:
%s

Respond in JSON format only, no markdown fences:
{"summary": "...", "recommendations": ["...", "..."]}`, filename, combined)

	return s.callGroqForSummary(reducePrompt)
}

func splitIntoChunks(text string, chunkSize, overlap int) []string {
	if len(text) <= chunkSize {
		return []string{text}
	}
	var chunks []string
	start := 0
	for start < len(text) {
		end := start + chunkSize
		if end > len(text) {
			end = len(text)
		}
		chunks = append(chunks, text[start:end])
		start = end - overlap
		if start >= len(text) {
			break
		}
	}
	return chunks
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
