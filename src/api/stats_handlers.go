package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"securevault-backend/src/services"

	"github.com/google/uuid"
)

// StatsHandlers handles user statistics endpoints
type StatsHandlers struct {
	statsService *services.StatsService
	authService  *services.AuthService
}

// NewStatsHandlers creates a new StatsHandlers instance
func NewStatsHandlers(statsService *services.StatsService, authService *services.AuthService) *StatsHandlers {
	return &StatsHandlers{
		statsService: statsService,
		authService:  authService,
	}
}

// StatsResponse represents the user statistics response format expected by tests
type StatsResponse struct {
	TotalFiles           int                    `json:"total_files"`
	TotalSizeBytes       int64                  `json:"total_size_bytes"`
	QuotaBytes          int64                  `json:"quota_bytes"`
	QuotaUsedBytes      int64                  `json:"quota_used_bytes"`
	QuotaAvailableBytes int64                  `json:"quota_available_bytes"`
	FilesByType         map[string]int         `json:"files_by_type"`
	UploadHistory       []UploadHistoryEntry   `json:"upload_history"`
}

// UploadHistoryEntry represents daily upload statistics
type UploadHistoryEntry struct {
	Date      string `json:"date"`
	Count     int    `json:"count"`
	TotalSize int64  `json:"total_size"`
}

// HandleStatsMe handles GET /api/v1/stats/me - get user statistics
// @Summary Get user statistics
// @Description Get comprehensive statistics for the authenticated user including storage usage, file counts, and upload history
// @Tags Statistics
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param from query string false "Start date for upload history (YYYY-MM-DD)"
// @Param to query string false "End date for upload history (YYYY-MM-DD)"
// @Param group_by query string false "Grouping for upload history (day, week, month, year)"
// @Success 200 {object} StatsResponse "User statistics"
// @Failure 400 {object} ErrorResponse "Invalid query parameters"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /stats/me [get]
func (h *StatsHandlers) HandleStatsMe(w http.ResponseWriter, r *http.Request) {
	// Validate authentication
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		h.writeErrorResponse(w, "UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
		return
	}

	// Parse and validate query parameters
	queryParams, validationErr := h.parseStatsQuery(r)
	if validationErr != nil {
		h.writeErrorResponse(w, validationErr.Code, validationErr.Message, validationErr.StatusCode)
		return
	}

	// Get user statistics
	userStats, err := h.statsService.GetUserStats(userID)
	if err != nil {
		h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to retrieve statistics", http.StatusInternalServerError)
		return
	}

	// Get files by type
	filesByType, err := h.getFilesByType(userID)
	if err != nil {
		h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to retrieve file type statistics", http.StatusInternalServerError)
		return
	}

	// Get upload history based on query parameters
	uploadHistory, err := h.getUploadHistory(userID, queryParams)
	if err != nil {
		h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to retrieve upload history", http.StatusInternalServerError)
		return
	}

	// Ensure upload history is never null in JSON
	if uploadHistory == nil {
		uploadHistory = []UploadHistoryEntry{}
	}

	// Ensure files by type is never null in JSON
	if filesByType == nil {
		filesByType = make(map[string]int)
	}

	// Build response according to test expectations
	response := StatsResponse{
		TotalFiles:           userStats.TotalFiles,
		TotalSizeBytes:       userStats.TotalSizeBytes,
		QuotaBytes:          userStats.StorageQuotaBytes,
		QuotaUsedBytes:      userStats.TotalSizeBytes,
		QuotaAvailableBytes: userStats.StorageQuotaBytes - userStats.TotalSizeBytes,
		FilesByType:         filesByType,
		UploadHistory:       uploadHistory,
	}

	// Ensure quota available is not negative
	if response.QuotaAvailableBytes < 0 {
		response.QuotaAvailableBytes = 0
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// StatsQueryParams represents parsed query parameters for stats endpoints
type StatsQueryParams struct {
	FromDate *time.Time
	ToDate   *time.Time
	GroupBy  string
}

// StatsValidationError represents a validation error with specific code
type StatsValidationError struct {
	Code       string
	Message    string
	StatusCode int
}

func (e *StatsValidationError) Error() string {
	return e.Message
}

// parseStatsQuery parses and validates query parameters for stats endpoints
func (h *StatsHandlers) parseStatsQuery(r *http.Request) (*StatsQueryParams, *StatsValidationError) {
	params := &StatsQueryParams{}

	// Parse from date
	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		fromDate, err := time.Parse("2006-01-02", fromStr)
		if err != nil {
			return nil, &StatsValidationError{
				Code:       "INVALID_DATE_FORMAT",
				Message:    "Invalid from date format, expected YYYY-MM-DD",
				StatusCode: http.StatusBadRequest,
			}
		}
		params.FromDate = &fromDate
	}

	// Parse to date
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		toDate, err := time.Parse("2006-01-02", toStr)
		if err != nil {
			return nil, &StatsValidationError{
				Code:       "INVALID_DATE_FORMAT",
				Message:    "Invalid to date format, expected YYYY-MM-DD",
				StatusCode: http.StatusBadRequest,
			}
		}
		params.ToDate = &toDate
	}

	// Validate date range
	if params.FromDate != nil && params.ToDate != nil {
		if params.FromDate.After(*params.ToDate) {
			return nil, &StatsValidationError{
				Code:       "INVALID_DATE_RANGE",
				Message:    "From date cannot be after to date",
				StatusCode: http.StatusBadRequest,
			}
		}

		// Check if date range is too large (more than 4 years)
		duration := params.ToDate.Sub(*params.FromDate)
		if duration > 4*365*24*time.Hour {
			return nil, &StatsValidationError{
				Code:       "DATE_RANGE_TOO_LARGE",
				Message:    "Date range cannot exceed 4 years",
				StatusCode: http.StatusBadRequest,
			}
		}
	}

	// Parse grouping parameter
	if groupBy := r.URL.Query().Get("group_by"); groupBy != "" {
		validGroups := map[string]bool{
			"day":   true,
			"week":  true,
			"month": true,
			"year":  true,
		}
		if !validGroups[groupBy] {
			return nil, &StatsValidationError{
				Code:       "INVALID_GROUPING",
				Message:    "Invalid grouping parameter, must be one of: day, week, month, year",
				StatusCode: http.StatusBadRequest,
			}
		}
		params.GroupBy = groupBy
	}

	return params, nil
}

// getFilesByType retrieves file counts grouped by MIME type for a user
func (h *StatsHandlers) getFilesByType(userID uuid.UUID) (map[string]int, error) {
	query := `
		SELECT mime_type, COUNT(*) as count
		FROM files 
		WHERE owner_id = $1
		GROUP BY mime_type
		ORDER BY count DESC
	`

	rows, err := h.statsService.GetDB().Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query files by type: %w", err)
	}
	defer rows.Close()

	filesByType := make(map[string]int)
	for rows.Next() {
		var mimeType string
		var count int
		err := rows.Scan(&mimeType, &count)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file type row: %w", err)
		}
		filesByType[mimeType] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating file type rows: %w", err)
	}

	return filesByType, nil
}

// getUploadHistory retrieves daily upload statistics for a user
func (h *StatsHandlers) getUploadHistory(userID uuid.UUID, params *StatsQueryParams) ([]UploadHistoryEntry, error) {
	// Default to last 30 days if no date range specified
	fromDate := time.Now().AddDate(0, 0, -30)
	toDate := time.Now()

	if params.FromDate != nil {
		fromDate = *params.FromDate
	}
	if params.ToDate != nil {
		toDate = *params.ToDate
	}

	// Determine grouping format based on params.GroupBy or default to day
	groupBy := "day"
	if params.GroupBy != "" {
		groupBy = params.GroupBy
	}

	var dateFormat string
	var truncFormat string
	
	switch groupBy {
	case "year":
		dateFormat = "2006"
		truncFormat = "year"
	case "month":
		dateFormat = "2006-01"
		truncFormat = "month"
	case "week":
		dateFormat = "2006-01-02"
		truncFormat = "week"
	default: // day
		dateFormat = "2006-01-02"
		truncFormat = "day"
	}

	query := fmt.Sprintf(`
		SELECT 
			DATE_TRUNC('%s', created_at) as date_group,
			COUNT(*) as count,
			COALESCE(SUM(size_bytes), 0) as total_size
		FROM files 
		WHERE owner_id = $1 
		AND created_at >= $2 
		AND created_at <= $3
		GROUP BY date_group
		ORDER BY date_group ASC
	`, truncFormat)

	rows, err := h.statsService.GetDB().Query(query, userID, fromDate, toDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query upload history: %w", err)
	}
	defer rows.Close()

	var history []UploadHistoryEntry
	for rows.Next() {
		var dateGroup time.Time
		var count int
		var totalSize int64
		
		err := rows.Scan(&dateGroup, &count, &totalSize)
		if err != nil {
			return nil, fmt.Errorf("failed to scan upload history row: %w", err)
		}

		entry := UploadHistoryEntry{
			Date:      dateGroup.Format(dateFormat),
			Count:     count,
			TotalSize: totalSize,
		}
		history = append(history, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating upload history rows: %w", err)
	}

	return history, nil
}

// writeErrorResponse writes a standardized error response
func (h *StatsHandlers) writeErrorResponse(w http.ResponseWriter, code, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	errorResponse := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
	
	json.NewEncoder(w).Encode(errorResponse)
}

// extractUserFromAuth extracts user ID from JWT token in Authorization header
func (h *StatsHandlers) extractUserFromAuth(r *http.Request) (uuid.UUID, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return uuid.Nil, fmt.Errorf("missing authorization header")
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return uuid.Nil, fmt.Errorf("invalid authorization header format")
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	
	claims, err := h.authService.ValidateToken(token)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid token: %w", err)
	}

	return claims.UserID, nil
}