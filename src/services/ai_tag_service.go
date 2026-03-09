package services

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ledongthuc/pdf"
)

// AiTagJob represents an AI tag generation job (V2: includes description + folder)
type AiTagJob struct {
	ID               uuid.UUID  `json:"id"`
	FileID           uuid.UUID  `json:"file_id"`
	Status           string     `json:"status"`
	SuggestedTags    []string   `json:"suggested_tags"`
	ConfidenceScores []float64  `json:"confidence_scores"`
	AiDescription    string     `json:"ai_description"`
	SuggestedFolder  string     `json:"suggested_folder"`
	ErrorMessage     *string    `json:"error_message,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

// AiAnalysisResult holds combined AI response (tags + description + folder)
type AiAnalysisResult struct {
	Tags            []string `json:"tags"`
	Description     string   `json:"description"`
	SuggestedFolder string   `json:"suggested_folder"`
}

// AiDescriptionResult is the response for description generation
type AiDescriptionResult struct {
	FileID      uuid.UUID `json:"file_id"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
}

// BulkAiTagResult is the response for bulk AI tag generation
type BulkAiTagResult struct {
	QueuedCount  int    `json:"queued_count"`
	SkippedCount int    `json:"skipped_count"`
	Status       string `json:"status"`
	Message      string `json:"message"`
}

// AiTagService handles AI-powered file tagging using Gemini or Groq
type AiTagService struct {
	db             *sql.DB
	storageService *StorageService
	aiProvider     string // "gemini" or "groq"
	geminiAPIKey   string
	groqAPIKey     string
	groqModel      string
	httpClient     *http.Client
	dailyLimit     int
}

// NewAiTagService creates a new AiTagService.
// aiProvider should be "gemini" or "groq". Falls back to whichever key is set.
func NewAiTagService(db *sql.DB, storageService *StorageService, aiProvider, geminiAPIKey, groqAPIKey, groqModel string, dailyLimit int) *AiTagService {
	if dailyLimit <= 0 {
		dailyLimit = 100
	}

	// Auto-detect provider if not explicitly set
	aiProvider = strings.ToLower(strings.TrimSpace(aiProvider))
	if aiProvider == "" {
		if groqAPIKey != "" {
			aiProvider = "groq"
		} else if geminiAPIKey != "" {
			aiProvider = "gemini"
		}
	}

	if groqModel == "" {
		groqModel = defaultGroqTextModel
	}

	return &AiTagService{
		db:             db,
		storageService: storageService,
		aiProvider:     aiProvider,
		geminiAPIKey:   geminiAPIKey,
		groqAPIKey:     groqAPIKey,
		groqModel:      groqModel,
		dailyLimit:     dailyLimit,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// IsEnabled returns true if the active AI provider's key is configured
func (s *AiTagService) IsEnabled() bool {
	switch s.aiProvider {
	case "groq":
		return s.groqAPIKey != ""
	case "gemini":
		return s.geminiAPIKey != ""
	default:
		return s.geminiAPIKey != "" || s.groqAPIKey != ""
	}
}

// Provider returns the active AI provider name
func (s *AiTagService) Provider() string {
	return s.aiProvider
}

// maxSizes for AI analysis per file type
const (
	maxImageSizeForAI      = 10 * 1024 * 1024 // 10 MB
	maxTextSizeForAI       = 100 * 1024        // 100 KB
	maxPDFSizeForAI        = 5 * 1024 * 1024   // 5 MB
	maxDOCXSizeForAI       = 5 * 1024 * 1024   // 5 MB
	maxDailyAIDescriptions = 50
	bulkDelayBetweenFiles  = 1500 * time.Millisecond
)

// Groq model defaults (used when GROQ_MODEL env var is not set)
const (
	defaultGroqTextModel   = "groq/compound-mini"
	defaultGroqVisionModel = "llama-3.2-11b-vision-preview"
)

// ============================================================
// GenerateTagsForFile - V2: combined analysis (tags+desc+folder)
// ============================================================

func (s *AiTagService) GenerateTagsForFile(fileID, ownerID uuid.UUID) {
	log.Printf("[AI-TAG] Starting combined analysis for file %s (provider: %s)", fileID, s.aiProvider)

	if !s.IsEnabled() {
		log.Printf("[AI-TAG] AI provider not configured, skipping")
		return
	}

	// Guard: skip if a job is already processing for this file (prevents polling storm)
	var existingStatus string
	err := s.db.QueryRow(`SELECT status FROM ai_tag_jobs WHERE file_id = $1`, fileID).Scan(&existingStatus)
	if err == nil && existingStatus == "processing" {
		log.Printf("[AI-TAG] Job already processing for file %s, skipping duplicate", fileID)
		return
	}

	// Check per-user daily limit
	if exceeded, err := s.isDailyLimitExceeded(ownerID); err != nil {
		log.Printf("[AI-TAG] Failed to check daily limit: %v", err)
		return
	} else if exceeded {
		log.Printf("[AI-TAG] Daily AI limit exceeded for user %s", ownerID)
		return
	}

	// Upsert job record (one job per file)
	jobID, err := s.upsertJob(fileID, "processing")
	if err != nil {
		log.Printf("[AI-TAG] Failed to upsert job: %v", err)
		return
	}

	// Fetch file metadata
	var mimeType, originalFilename, blobHash string
	var sizeBytes int64
	err = s.db.QueryRow(`
		SELECT mime_type, original_filename, blob_hash, size_bytes
		FROM files WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL
	`, fileID, ownerID).Scan(&mimeType, &originalFilename, &blobHash, &sizeBytes)
	if err != nil {
		s.failJob(jobID, fmt.Sprintf("file not found: %v", err))
		return
	}

	// Choose strategy based on mime type — all return AiAnalysisResult
	var result *AiAnalysisResult
	var analysisErr error

	switch {
	case strings.HasPrefix(mimeType, "image/") && sizeBytes <= maxImageSizeForAI:
		result, analysisErr = s.analyzeImage(blobHash, mimeType)
	case isTextualMimeType(mimeType) && sizeBytes <= maxTextSizeForAI:
		result, analysisErr = s.analyzeText(blobHash, originalFilename)
	case mimeType == "application/pdf" && sizeBytes <= maxPDFSizeForAI:
		result, analysisErr = s.analyzePDF(blobHash, originalFilename)
	case mimeType == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" && sizeBytes <= maxDOCXSizeForAI:
		result, analysisErr = s.analyzeDOCX(blobHash, originalFilename)
	default:
		result, analysisErr = s.analyzeMetadataOnly(originalFilename, mimeType, sizeBytes)
	}

	if analysisErr != nil {
		s.failJob(jobID, fmt.Sprintf("analysis failed: %v", analysisErr))
		return
	}

	if result == nil || len(result.Tags) == 0 {
		s.failJob(jobID, "no tags generated")
		return
	}

	// Limit to 4 tags max
	if len(result.Tags) > 4 {
		result.Tags = result.Tags[:4]
	}

	// Truncate description to 500 chars
	if len(result.Description) > 500 {
		result.Description = result.Description[:500]
	}

	// Assign confidence scores (descending from 0.95)
	confidences := make([]float64, len(result.Tags))
	for i := range result.Tags {
		confidences[i] = 0.95 - float64(i)*0.05
		if confidences[i] < 0.5 {
			confidences[i] = 0.5
		}
	}

	// Complete the job with all fields
	err = s.completeJob(jobID, result.Tags, confidences, result.Description, result.SuggestedFolder)
	if err != nil {
		log.Printf("[AI-TAG] Failed to complete job: %v", err)
		return
	}

	// Remove old AI-generated tags before inserting fresh ones (enforces the limit)
	_, err = s.db.Exec(`DELETE FROM file_tags WHERE file_id = $1 AND is_ai_generated = true`, fileID)
	if err != nil {
		log.Printf("[AI-TAG] Failed to clean old AI tags for file %s: %v", fileID, err)
	}

	// Insert tags into file_tags table
	for i, tag := range result.Tags {
		_, err = s.db.Exec(`
			INSERT INTO file_tags (file_id, name, is_ai_generated, confidence)
			VALUES ($1, $2, true, $3)
			ON CONFLICT (file_id, name) DO UPDATE SET
				is_ai_generated = true,
				confidence = EXCLUDED.confidence
		`, fileID, tag, confidences[i])
		if err != nil {
			log.Printf("[AI-TAG] Failed to insert tag '%s' for file %s: %v", tag, fileID, err)
		}
	}

	// Also store description in file_descriptions table
	if result.Description != "" {
		_, err = s.db.Exec(`
			INSERT INTO file_descriptions (file_id, description, generated_by)
			VALUES ($1, $2, 'ai')
			ON CONFLICT (file_id) DO UPDATE SET
				description = EXCLUDED.description,
				generated_by = 'ai',
				updated_at = NOW()
		`, fileID, result.Description)
		if err != nil {
			log.Printf("[AI-TAG] Failed to store description for file %s: %v", fileID, err)
		}
	}

	log.Printf("[AI-TAG] Successfully analyzed file %s: %d tags, desc=%d chars, folder=%s",
		fileID, len(result.Tags), len(result.Description), result.SuggestedFolder)
}

// ============================================================
// GetAiTagJob - V2: returns description + folder
// ============================================================

func (s *AiTagService) GetAiTagJob(fileID, ownerID uuid.UUID) (*AiTagJob, error) {
	// Verify ownership
	var exists bool
	err := s.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM files WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL)`,
		fileID, ownerID,
	).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("failed to verify file ownership: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("file not found")
	}

	var job AiTagJob
	var aiDesc, sugFolder sql.NullString
	err = s.db.QueryRow(`
		SELECT id, file_id, status, suggested_tags, confidence_scores,
		       COALESCE(ai_description, ''), COALESCE(suggested_folder, ''),
		       error_message, created_at, completed_at
		FROM ai_tag_jobs
		WHERE file_id = $1
	`, fileID).Scan(
		&job.ID, &job.FileID, &job.Status,
		(*pgStringArray)(&job.SuggestedTags),
		(*pgFloat64Array)(&job.ConfidenceScores),
		&aiDesc, &sugFolder,
		&job.ErrorMessage, &job.CreatedAt, &job.CompletedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No job exists
		}
		return nil, fmt.Errorf("failed to get AI tag job: %w", err)
	}

	job.AiDescription = aiDesc.String
	job.SuggestedFolder = sugFolder.String

	return &job, nil
}

// ============================================================
// GenerateDescription - synchronous, cache-aware
// ============================================================

func (s *AiTagService) GenerateDescription(fileID, ownerID uuid.UUID) (*AiDescriptionResult, error) {
	if !s.IsEnabled() {
		return nil, fmt.Errorf("AI service unavailable")
	}

	// Verify ownership
	var exists bool
	err := s.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM files WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL)`,
		fileID, ownerID,
	).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("failed to verify file ownership: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("file not found")
	}

	// 1. Check ai_tag_jobs for cached description
	var cachedDesc string
	err = s.db.QueryRow(`
		SELECT COALESCE(ai_description, '')
		FROM ai_tag_jobs
		WHERE file_id = $1 AND status = 'completed' AND ai_description != ''
	`, fileID).Scan(&cachedDesc)
	if err == nil && cachedDesc != "" {
		return &AiDescriptionResult{
			FileID:      fileID,
			Description: cachedDesc,
			Status:      "completed",
		}, nil
	}

	// 2. Check file_descriptions table
	err = s.db.QueryRow(`
		SELECT description FROM file_descriptions WHERE file_id = $1
	`, fileID).Scan(&cachedDesc)
	if err == nil && cachedDesc != "" {
		return &AiDescriptionResult{
			FileID:      fileID,
			Description: cachedDesc,
			Status:      "completed",
		}, nil
	}

	// 3. No cache — trigger full analysis synchronously
	log.Printf("[AI-DESC] No cached description for file %s, running AI analysis (provider: %s)", fileID, s.aiProvider)

	// Fetch file metadata
	var mimeType, originalFilename, blobHash string
	var sizeBytes int64
	err = s.db.QueryRow(`
		SELECT mime_type, original_filename, blob_hash, size_bytes
		FROM files WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL
	`, fileID, ownerID).Scan(&mimeType, &originalFilename, &blobHash, &sizeBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to get file metadata: %w", err)
	}

	var result *AiAnalysisResult
	switch {
	case strings.HasPrefix(mimeType, "image/") && sizeBytes <= maxImageSizeForAI:
		result, err = s.analyzeImage(blobHash, mimeType)
	case isTextualMimeType(mimeType) && sizeBytes <= maxTextSizeForAI:
		result, err = s.analyzeText(blobHash, originalFilename)
	case mimeType == "application/pdf" && sizeBytes <= maxPDFSizeForAI:
		result, err = s.analyzeText(blobHash, originalFilename)
	default:
		result, err = s.analyzeMetadataOnly(originalFilename, mimeType, sizeBytes)
	}

	if err != nil {
		return &AiDescriptionResult{
			FileID:      fileID,
			Description: "",
			Status:      "failed",
		}, nil
	}

	desc := result.Description
	if len(desc) > 500 {
		desc = desc[:500]
	}

	// Store description
	if desc != "" {
		_, _ = s.db.Exec(`
			INSERT INTO file_descriptions (file_id, description, generated_by)
			VALUES ($1, $2, 'ai')
			ON CONFLICT (file_id) DO UPDATE SET
				description = EXCLUDED.description, updated_at = NOW()
		`, fileID, desc)

		// Also update ai_tag_jobs if a row exists
		_, _ = s.db.Exec(`
			UPDATE ai_tag_jobs SET ai_description = $1 WHERE file_id = $2
		`, desc, fileID)
	}

	return &AiDescriptionResult{
		FileID:      fileID,
		Description: desc,
		Status:      "completed",
	}, nil
}

// ============================================================
// BulkGenerateTags - queue multiple files for AI analysis
// ============================================================

func (s *AiTagService) BulkGenerateTags(fileIDs []uuid.UUID, ownerID uuid.UUID) (*BulkAiTagResult, error) {
	if !s.IsEnabled() {
		return nil, fmt.Errorf("AI service unavailable")
	}

	// Verify all files belong to user
	var ownedIDs []uuid.UUID
	for _, fid := range fileIDs {
		var exists bool
		err := s.db.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM files WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL)`,
			fid, ownerID,
		).Scan(&exists)
		if err == nil && exists {
			ownedIDs = append(ownedIDs, fid)
		}
	}

	if len(ownedIDs) == 0 {
		return &BulkAiTagResult{
			QueuedCount:  0,
			SkippedCount: len(fileIDs),
			Status:       "completed",
			Message:      "No valid files to process",
		}, nil
	}

	// Filter out files that already have completed AI tag jobs
	var toProcess []uuid.UUID
	skipped := 0
	for _, fid := range ownedIDs {
		var status string
		err := s.db.QueryRow(`SELECT status FROM ai_tag_jobs WHERE file_id = $1`, fid).Scan(&status)
		if err == sql.ErrNoRows || status == "failed" {
			toProcess = append(toProcess, fid)
		} else {
			skipped++
		}
	}

	// Check daily rate limit — cap at remaining quota
	remaining, err := s.dailyRemaining(ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to check daily limit: %w", err)
	}

	if remaining <= 0 {
		return &BulkAiTagResult{
			QueuedCount:  0,
			SkippedCount: len(fileIDs),
			Status:       "failed",
			Message:      "Daily AI limit reached",
		}, nil
	}

	if len(toProcess) > remaining {
		skipped += len(toProcess) - remaining
		toProcess = toProcess[:remaining]
	}

	// Create pending jobs for each file
	for _, fid := range toProcess {
		_, err := s.upsertJob(fid, "pending")
		if err != nil {
			log.Printf("[AI-BULK] Failed to create job for file %s: %v", fid, err)
		}
	}

	TryRunBackground(func() {
		for i, fid := range toProcess {
			if i > 0 {
				time.Sleep(bulkDelayBetweenFiles)
			}
			s.GenerateTagsForFile(fid, ownerID)
		}
		log.Printf("[AI-BULK] Completed bulk processing of %d files for user %s", len(toProcess), ownerID)
	})

	return &BulkAiTagResult{
		QueuedCount:  len(toProcess),
		SkippedCount: skipped,
		Status:       "processing",
		Message:      fmt.Sprintf("%d files queued for AI analysis", len(toProcess)),
	}, nil
}

// ResetAndTrigger resets an existing job and re-triggers analysis
func (s *AiTagService) ResetAndTrigger(fileID, ownerID uuid.UUID) error {
	if !s.IsEnabled() {
		return fmt.Errorf("AI service unavailable")
	}

	// Verify ownership
	var exists bool
	err := s.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM files WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL)`,
		fileID, ownerID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to verify file ownership: %w", err)
	}
	if !exists {
		return fmt.Errorf("file not found")
	}

	// Check daily limit
	if exceeded, err := s.isDailyLimitExceeded(ownerID); err != nil {
		return fmt.Errorf("failed to check daily limit: %w", err)
	} else if exceeded {
		return fmt.Errorf("daily AI limit reached")
	}

	// Upsert job as pending
	_, err = s.upsertJob(fileID, "pending")
	if err != nil {
		return fmt.Errorf("failed to reset job: %w", err)
	}

	TryRunBackground(func() { s.GenerateTagsForFile(fileID, ownerID) })
	return nil
}

// ============================================================
// Provider-agnostic analyze methods
// ============================================================

// Shared prompt used by all providers
const analysisPromptImage = `Analyze this image and return a JSON object with exactly these keys:

1. "tags": array of exactly 3-4 descriptive tags (lowercase, short phrases). Keep it concise.
2. "description": a 1-2 sentence description of the image content
3. "suggested_folder": suggest ONE folder name where this file would logically belong (e.g. "Photos/Vacation", "Work/Screenshots", "Design/Logos"). Use forward slashes for nested folders.

Return ONLY valid JSON, no markdown fences. Example:
{"tags": ["sunset", "beach", "nature"], "description": "A colorful sunset over a sandy beach.", "suggested_folder": "Photos/Nature"}`

func (s *AiTagService) analyzeImage(blobHash, mimeType string) (*AiAnalysisResult, error) {
	log.Printf("[AI-TAG] Analyzing image (provider: %s, hash: %s)", s.aiProvider, blobHash[:8])

	reader, err := s.storageService.DownloadFile(blobHash)
	if err != nil {
		return nil, fmt.Errorf("failed to download blob: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read blob: %w", err)
	}

	b64Data := base64.StdEncoding.EncodeToString(data)

	return s.callAI(analysisPromptImage, b64Data, mimeType)
}

func (s *AiTagService) analyzeText(blobHash, filename string) (*AiAnalysisResult, error) {
	log.Printf("[AI-TAG] Analyzing text content (provider: %s, hash: %s)", s.aiProvider, blobHash[:8])

	reader, err := s.storageService.DownloadFile(blobHash)
	if err != nil {
		return nil, fmt.Errorf("failed to download blob: %w", err)
	}
	defer reader.Close()

	data := make([]byte, maxTextSizeForAI)
	n, err := reader.Read(data)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to read blob: %w", err)
	}
	content := string(data[:n])

	prompt := fmt.Sprintf(`Analyze the following document and return a JSON object with exactly these keys:

1. "tags": array of exactly 3-4 descriptive tags (lowercase, short phrases) capturing main topics. Keep it concise.
2. "description": a 1-2 sentence summary of the document
3. "suggested_folder": suggest ONE folder name where this file would logically belong (e.g. "Work/Reports", "School/Homework", "Finance/Invoices"). Use forward slashes for nested folders.

Document filename: "%s"
Document content:

%s

Return ONLY valid JSON, no markdown fences.`, filename, content)

	return s.callAI(prompt, "", "")
}

func (s *AiTagService) analyzeMetadataOnly(filename, mimeType string, sizeBytes int64) (*AiAnalysisResult, error) {
	log.Printf("[AI-TAG] Analyzing metadata only (provider: %s, file: %s)", s.aiProvider, filename)

	prompt := fmt.Sprintf(`Given a file with the following metadata, return a JSON object with exactly these keys:

1. "tags": array of exactly 3-4 relevant category tags (lowercase). Keep it concise.
2. "description": a brief 1-sentence description based on the filename and type
3. "suggested_folder": suggest ONE folder name where this file would logically belong

Filename: "%s"
Type: "%s"
Size: %d bytes

Return ONLY valid JSON, no markdown fences.`, filename, mimeType, sizeBytes)

	return s.callAI(prompt, "", "")
}

func (s *AiTagService) analyzePDF(blobHash, filename string) (*AiAnalysisResult, error) {
	log.Printf("[AI-TAG] Analyzing PDF content (provider: %s, hash: %s)", s.aiProvider, blobHash[:8])

	reader, err := s.storageService.DownloadFile(blobHash)
	if err != nil {
		return nil, fmt.Errorf("failed to download blob: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(io.LimitReader(reader, maxPDFSizeForAI))
	if err != nil {
		return nil, fmt.Errorf("failed to read blob: %w", err)
	}

	pdfReader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("PDF parse failed: %w", err)
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

	content := sb.String()
	if strings.TrimSpace(content) == "" {
		return s.analyzeMetadataOnly(filename, "application/pdf", int64(len(data)))
	}

	if len(content) > maxTextSizeForAI {
		content = content[:maxTextSizeForAI]
	}

	prompt := fmt.Sprintf(`Analyze the following document and return a JSON object with exactly these keys:

1. "tags": array of exactly 3-4 descriptive tags (lowercase, short phrases) capturing main topics. Keep it concise.
2. "description": a 1-2 sentence summary of the document
3. "suggested_folder": suggest ONE folder name where this file would logically belong (e.g. "Work/Reports", "School/Homework", "Finance/Invoices"). Use forward slashes for nested folders.

Document filename: "%s"
Document content:

%s

Return ONLY valid JSON, no markdown fences.`, filename, content)

	return s.callAI(prompt, "", "")
}

func (s *AiTagService) analyzeDOCX(blobHash, filename string) (*AiAnalysisResult, error) {
	log.Printf("[AI-TAG] Analyzing DOCX content (provider: %s, hash: %s)", s.aiProvider, blobHash[:8])

	reader, err := s.storageService.DownloadFile(blobHash)
	if err != nil {
		return nil, fmt.Errorf("failed to download blob: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(io.LimitReader(reader, maxDOCXSizeForAI))
	if err != nil {
		return nil, fmt.Errorf("failed to read blob: %w", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("DOCX zip open failed: %w", err)
	}

	var content string
	for _, file := range zipReader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("DOCX document.xml open failed: %w", err)
		}
		xmlData, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("DOCX document.xml read failed: %w", err)
		}
		content = stripXMLTagsForTagging(string(xmlData))
		break
	}

	if strings.TrimSpace(content) == "" {
		return s.analyzeMetadataOnly(filename, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", int64(len(data)))
	}

	if len(content) > maxTextSizeForAI {
		content = content[:maxTextSizeForAI]
	}

	prompt := fmt.Sprintf(`Analyze the following document and return a JSON object with exactly these keys:

1. "tags": array of exactly 3-4 descriptive tags (lowercase, short phrases) capturing main topics. Keep it concise.
2. "description": a 1-2 sentence summary of the document
3. "suggested_folder": suggest ONE folder name where this file would logically belong (e.g. "Work/Reports", "School/Homework", "Finance/Invoices"). Use forward slashes for nested folders.

Document filename: "%s"
Document content:

%s

Return ONLY valid JSON, no markdown fences.`, filename, content)

	return s.callAI(prompt, "", "")
}

func stripXMLTagsForTagging(xmlContent string) string {
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
	result := sb.String()
	for strings.Contains(result, "  ") {
		result = strings.ReplaceAll(result, "  ", " ")
	}
	return strings.TrimSpace(result)
}

// ============================================================
// callAI - dispatches to the active provider
// ============================================================

// callAI routes the request to the configured AI provider.
// base64Image and imageMimeType are only used for image analysis (empty for text-only).
func (s *AiTagService) callAI(prompt, base64Image, imageMimeType string) (*AiAnalysisResult, error) {
	switch s.aiProvider {
	case "groq":
		return s.callGroqAPI(prompt, base64Image, imageMimeType)
	case "gemini":
		return s.callGeminiAPI(prompt, base64Image, imageMimeType)
	default:
		return nil, fmt.Errorf("unknown AI provider: %s", s.aiProvider)
	}
}

// ============================================================
// Groq API (OpenAI-compatible chat completions)
// ============================================================

func (s *AiTagService) callGroqAPI(prompt, base64Image, imageMimeType string) (*AiAnalysisResult, error) {
	apiURL := "https://api.groq.com/openai/v1/chat/completions"

	// Choose model based on whether we have an image
	model := s.groqModel
	if base64Image != "" {
		model = defaultGroqVisionModel
	}

	// Build message content
	var content interface{}
	if base64Image != "" {
		// Multimodal: text + image
		content = []map[string]interface{}{
			{"type": "text", "text": prompt},
			{"type": "image_url", "image_url": map[string]interface{}{
				"url": fmt.Sprintf("data:%s;base64,%s", imageMimeType, base64Image),
			}},
		}
	} else {
		// Text only
		content = prompt
	}

	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": content,
			},
		},
		"temperature": 0.3,
		"max_tokens":  1024,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Groq request: %w", err)
	}

	// Retry logic for 429
	maxRetries := 3
	var resp *http.Response

	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("failed to create Groq request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+s.groqAPIKey)

		resp, err = s.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to call Groq API: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			if attempt < maxRetries {
				backoff := time.Duration(5*(attempt+1)) * time.Second // 5s, 10s, 15s
				log.Printf("[AI-TAG] Groq rate limited (429), retrying in %v (attempt %d/%d)", backoff, attempt+1, maxRetries)
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

	// Parse OpenAI-compatible response
	var groqResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&groqResp); err != nil {
		return nil, fmt.Errorf("failed to decode Groq response: %w", err)
	}

	if len(groqResp.Choices) == 0 {
		return nil, fmt.Errorf("empty Groq response")
	}

	rawText := groqResp.Choices[0].Message.Content
	return parseAIAnalysis(rawText)
}

// ============================================================
// Gemini API
// ============================================================

func (s *AiTagService) callGeminiAPI(prompt, base64Image, imageMimeType string) (*AiAnalysisResult, error) {
	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=%s", s.geminiAPIKey)

	// Build Gemini-specific payload
	var parts []map[string]interface{}
	parts = append(parts, map[string]interface{}{"text": prompt})

	if base64Image != "" {
		parts = append(parts, map[string]interface{}{
			"inline_data": map[string]interface{}{
				"mime_type": imageMimeType,
				"data":      base64Image,
			},
		})
	}

	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": parts},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Gemini request: %w", err)
	}

	// Retry logic for 429
	maxRetries := 3
	var resp *http.Response

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err = s.httpClient.Post(apiURL, "application/json", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("failed to call Gemini API: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			if attempt < maxRetries {
				backoff := time.Duration(10*(attempt+1)) * time.Second // 10s, 20s, 30s
				log.Printf("[AI-TAG] Gemini rate limited (429), retrying in %v (attempt %d/%d)", backoff, attempt+1, maxRetries)
				time.Sleep(backoff)
				continue
			}
			return nil, fmt.Errorf("Gemini API rate limit exceeded after %d retries", maxRetries)
		}
		break
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gemini API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, fmt.Errorf("failed to decode Gemini response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty Gemini response")
	}

	rawText := geminiResp.Candidates[0].Content.Parts[0].Text
	return parseAIAnalysis(rawText)
}

// ============================================================
// Shared response parser (works for both Gemini and Groq)
// ============================================================

// parseAIAnalysis parses a JSON object {tags, description, suggested_folder} from any AI provider
func parseAIAnalysis(text string) (*AiAnalysisResult, error) {
	// Strip markdown code fences if present
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	// Try parsing as JSON object first
	var result AiAnalysisResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		// Try to find a JSON object in the text
		start := strings.Index(text, "{")
		end := strings.LastIndex(text, "}")
		if start >= 0 && end > start {
			if err2 := json.Unmarshal([]byte(text[start:end+1]), &result); err2 != nil {
				// Last resort: try to parse as plain array (backward compat)
				return parseFallbackArray(text)
			}
		} else {
			return parseFallbackArray(text)
		}
	}

	// Normalize tags
	seen := make(map[string]bool)
	var normalized []string
	for _, tag := range result.Tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag != "" && !seen[tag] {
			seen[tag] = true
			normalized = append(normalized, tag)
		}
	}
	result.Tags = normalized
	result.Description = strings.TrimSpace(result.Description)
	result.SuggestedFolder = strings.TrimSpace(result.SuggestedFolder)

	return &result, nil
}

// parseFallbackArray handles backward compat if AI returns plain tag array
func parseFallbackArray(text string) (*AiAnalysisResult, error) {
	text = strings.TrimSpace(text)
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no JSON found in response: %s", text)
	}

	var tags []string
	if err := json.Unmarshal([]byte(text[start:end+1]), &tags); err != nil {
		return nil, fmt.Errorf("failed to parse tags from response: %w (raw: %s)", err, text)
	}

	seen := make(map[string]bool)
	var normalized []string
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag != "" && !seen[tag] {
			seen[tag] = true
			normalized = append(normalized, tag)
		}
	}

	return &AiAnalysisResult{
		Tags:            normalized,
		Description:     "",
		SuggestedFolder: "",
	}, nil
}

// ============================================================
// Database helpers
// ============================================================

func (s *AiTagService) upsertJob(fileID uuid.UUID, status string) (uuid.UUID, error) {
	var jobID uuid.UUID
	err := s.db.QueryRow(`
		INSERT INTO ai_tag_jobs (file_id, status, suggested_tags, confidence_scores, ai_description, suggested_folder, error_message, completed_at)
		VALUES ($1, $2, '{}', '{}', '', '', NULL, NULL)
		ON CONFLICT (file_id) DO UPDATE SET
			status = EXCLUDED.status,
			suggested_tags = '{}',
			confidence_scores = '{}',
			ai_description = '',
			suggested_folder = '',
			error_message = NULL,
			completed_at = NULL,
			created_at = NOW()
		RETURNING id
	`, fileID, status).Scan(&jobID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to upsert AI tag job: %w", err)
	}
	return jobID, nil
}

func (s *AiTagService) completeJob(jobID uuid.UUID, tags []string, confidences []float64, description, suggestedFolder string) error {
	_, err := s.db.Exec(`
		UPDATE ai_tag_jobs
		SET status = 'completed',
		    suggested_tags = $1,
		    confidence_scores = $2,
		    ai_description = $3,
		    suggested_folder = $4,
		    completed_at = NOW()
		WHERE id = $5
	`, pgStringArray(tags), pgFloat64Array(confidences), description, suggestedFolder, jobID)
	return err
}

func (s *AiTagService) failJob(jobID uuid.UUID, errorMsg string) {
	log.Printf("[AI-TAG] Job %s failed: %s", jobID, errorMsg)
	_, err := s.db.Exec(`
		UPDATE ai_tag_jobs
		SET status = 'failed', error_message = $1, completed_at = NOW()
		WHERE id = $2
	`, errorMsg, jobID)
	if err != nil {
		log.Printf("[AI-TAG] Failed to update job status: %v", err)
	}
}

func (s *AiTagService) isDailyLimitExceeded(ownerID uuid.UUID) (bool, error) {
	remaining, err := s.dailyRemaining(ownerID)
	if err != nil {
		return false, err
	}
	return remaining <= 0, nil
}

func (s *AiTagService) dailyRemaining(ownerID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM ai_tag_jobs aj
		JOIN files f ON f.id = aj.file_id
		WHERE f.owner_id = $1 AND aj.created_at > NOW() - INTERVAL '24 hours'
	`, ownerID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return s.dailyLimit - count, nil
}

// ============================================================
// Utility functions
// ============================================================

func isTextualMimeType(mimeType string) bool {
	textTypes := []string{
		"text/",
		"application/json",
		"application/xml",
		"application/javascript",
		"application/typescript",
		"application/x-yaml",
		"application/yaml",
		"application/toml",
		"application/x-sh",
	}
	for _, t := range textTypes {
		if strings.HasPrefix(mimeType, t) || mimeType == t {
			return true
		}
	}
	return false
}

// ============================================================
// PostgreSQL array type helpers
// ============================================================

type pgStringArray []string

func (a pgStringArray) Value() (driver.Value, error) {
	if a == nil {
		return "{}", nil
	}
	parts := make([]string, len(a))
	for i, s := range a {
		escaped := strings.ReplaceAll(s, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		parts[i] = `"` + escaped + `"`
	}
	return "{" + strings.Join(parts, ",") + "}", nil
}

func (a *pgStringArray) Scan(src interface{}) error {
	if src == nil {
		*a = []string{}
		return nil
	}

	var str string
	switch v := src.(type) {
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		return fmt.Errorf("unsupported type for pgStringArray: %T", src)
	}

	str = strings.TrimPrefix(str, "{")
	str = strings.TrimSuffix(str, "}")
	if str == "" {
		*a = []string{}
		return nil
	}

	*a = parsePostgresArray(str)
	return nil
}

type pgFloat64Array []float64

func (a pgFloat64Array) Value() (driver.Value, error) {
	if a == nil {
		return "{}", nil
	}
	parts := make([]string, len(a))
	for i, f := range a {
		parts[i] = fmt.Sprintf("%g", f)
	}
	return "{" + strings.Join(parts, ",") + "}", nil
}

func (a *pgFloat64Array) Scan(src interface{}) error {
	if src == nil {
		*a = []float64{}
		return nil
	}

	var str string
	switch v := src.(type) {
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		return fmt.Errorf("unsupported type for pgFloat64Array: %T", src)
	}

	str = strings.TrimPrefix(str, "{")
	str = strings.TrimSuffix(str, "}")
	if str == "" {
		*a = []float64{}
		return nil
	}

	parts := strings.Split(str, ",")
	result := make([]float64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var f float64
		if _, err := fmt.Sscanf(p, "%g", &f); err == nil {
			result = append(result, f)
		}
	}
	*a = result
	return nil
}

func parsePostgresArray(s string) []string {
	var result []string
	var current strings.Builder
	inQuote := false
	escaped := false

	for _, ch := range s {
		if escaped {
			current.WriteRune(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			inQuote = !inQuote
			continue
		}
		if ch == ',' && !inQuote {
			result = append(result, current.String())
			current.Reset()
			continue
		}
		current.WriteRune(ch)
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	return result
}
