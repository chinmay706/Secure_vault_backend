package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"securevault-backend/src/services"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// AdminHandlers handles admin-only endpoints
type AdminHandlers struct {
	statsService *services.StatsService
	fileService  *services.FileService
	authService  *services.AuthService
}

// NewAdminHandlers creates a new AdminHandlers instance
func NewAdminHandlers(statsService *services.StatsService, fileService *services.FileService, authService *services.AuthService) *AdminHandlers {
	return &AdminHandlers{
		statsService: statsService,
		fileService:  fileService,
		authService:  authService,
	}
}

// AdminFilesResponse represents the admin files list response
type AdminFilesResponse struct {
	Files      []AdminFileEntry `json:"files"`
	Pagination PaginationInfo   `json:"pagination"`
}

// AdminFileEntry represents a file entry with admin-specific fields
type AdminFileEntry struct {
	ID           uuid.UUID `json:"id"`
	Filename     string    `json:"filename"`
	Size         int64     `json:"size"`
	MimeType     string    `json:"mime_type"`
	UploadDate   time.Time `json:"upload_date"`
	UserEmail    string    `json:"user_email"`
	UserID       uuid.UUID `json:"user_id"`
	IsPublic     bool      `json:"is_public"`
	DownloadCount int64    `json:"download_count"`
}

// PaginationInfo represents pagination metadata
type PaginationInfo struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// AdminStatsResponse represents the admin statistics response
type AdminStatsResponse struct {
	TotalUsers                int                    `json:"total_users"`
	TotalFiles                int                    `json:"total_files"`
	TotalSizeBytes            int64                  `json:"total_size_bytes"`
	TotalQuotaBytes           int64                  `json:"total_quota_bytes"`
	QuotaUtilizationPercent   float64                `json:"quota_utilization_percent"`
	FilesByType               map[string]int         `json:"files_by_type"`
	TotalUserRegistrations    int                    `json:"total_user_registrations"`
	UsersByRegistrationDate   []RegistrationEntry    `json:"users_by_registration_date"`
	StorageByUser             []UserStorageEntry     `json:"storage_by_user"`
	MostActiveUsers           []ActiveUserEntry      `json:"most_active_users"`
}

// RegistrationEntry represents user registration statistics by date
type RegistrationEntry struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// UserStorageEntry represents storage usage by user
type UserStorageEntry struct {
	UserID         uuid.UUID `json:"user_id"`
	UserEmail      string    `json:"user_email"`
	FileCount      int       `json:"file_count"`
	TotalSizeBytes int64     `json:"total_size_bytes"`
	QuotaBytes     int64     `json:"quota_bytes"`
}

// ActiveUserEntry represents user activity statistics
type ActiveUserEntry struct {
	UserID        uuid.UUID  `json:"user_id"`
	UserEmail     string     `json:"user_email"`
	FileCount     int        `json:"file_count"`
	LastUpload    *time.Time `json:"last_upload"`
	TotalDownloads int64     `json:"total_downloads"`
}

// HandleAdminFiles handles GET /api/v1/admin/files - list all files with admin view
// @Summary List all files (Admin)
// @Description Get a paginated list of all files in the system with admin-level details
// @Tags Admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param user_id query string false "Filter by specific user UUID"
// @Param user_email query string false "Filter by user email (partial match)"
// @Param uploaded_after query string false "Filter files uploaded after date (YYYY-MM-DD)"
// @Param uploaded_before query string false "Filter files uploaded before date (YYYY-MM-DD)"
// @Param sort query string false "Sort field (filename, size, upload_date, user_email)"
// @Param order query string false "Sort order (asc, desc)"
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page (max 100)" default(20)
// @Success 200 {object} AdminFilesResponse "List of all files with admin details"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 403 {object} ErrorResponse "Admin privileges required"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /admin/files [get]
func (h *AdminHandlers) HandleAdminFiles(w http.ResponseWriter, r *http.Request) {
	// Validate admin authentication
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		h.writeErrorResponse(w, "UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
		return
	}

	// Check if user is admin
	isAdmin, err := h.authService.IsAdmin(userID)
	if err != nil {
		h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to verify admin privileges", http.StatusInternalServerError)
		return
	}

	if !isAdmin {
		h.writeErrorResponse(w, "ADMIN_REQUIRED", "Admin privileges required to access this endpoint", http.StatusForbidden)
		return
	}

	// Parse query parameters
	queryParams, validationErr := h.parseAdminFilesQuery(r)
	if validationErr != nil {
		h.writeErrorResponse(w, validationErr.Code, validationErr.Message, validationErr.StatusCode)
		return
	}

	// Get admin files list
	files, pagination, err := h.getAdminFiles(queryParams)
	if err != nil {
		h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to retrieve admin files", http.StatusInternalServerError)
		return
	}

	response := AdminFilesResponse{
		Files:      files,
		Pagination: pagination,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// HandleAdminStats handles GET /api/v1/admin/stats - get system-wide statistics
// @Summary Get system statistics (Admin)
// @Description Get comprehensive system-wide statistics including users, files, storage, and analytics
// @Tags Admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param from query string false "Start date for time-based data (YYYY-MM-DD)"
// @Param to query string false "End date for time-based data (YYYY-MM-DD)"
// @Param group_by query string false "Time grouping (day, week, month, year)"
// @Success 200 {object} AdminStatsResponse "System-wide statistics"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 403 {object} ErrorResponse "Admin privileges required"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /admin/stats [get]
func (h *AdminHandlers) HandleAdminStats(w http.ResponseWriter, r *http.Request) {
	// Validate admin authentication
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		h.writeErrorResponse(w, "UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
		return
	}

	// Check if user is admin
	isAdmin, err := h.authService.IsAdmin(userID)
	if err != nil {
		h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to verify admin privileges", http.StatusInternalServerError)
		return
	}

	if !isAdmin {
		h.writeErrorResponse(w, "ADMIN_REQUIRED", "Admin privileges required to access this endpoint", http.StatusForbidden)
		return
	}

	// Parse query parameters
	queryParams, validationErr := h.parseAdminStatsQuery(r)
	if validationErr != nil {
		h.writeErrorResponse(w, validationErr.Code, validationErr.Message, validationErr.StatusCode)
		return
	}

	// Get system stats
	systemStats, err := h.statsService.GetSystemStats()
	if err != nil {
		h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to retrieve system statistics", http.StatusInternalServerError)
		return
	}

	// Get additional admin stats
	filesByType, err := h.getSystemFilesByType()
	if err != nil {
		h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to retrieve file type statistics", http.StatusInternalServerError)
		return
	}

	usersByRegistration, err := h.getUserRegistrationHistory(queryParams)
	if err != nil {
		h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to retrieve user registration history", http.StatusInternalServerError)
		return
	}

	storageByUser, err := h.getStorageByUser()
	if err != nil {
		h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to retrieve user storage statistics", http.StatusInternalServerError)
		return
	}

	activeUsers, err := h.getMostActiveUsers()
	if err != nil {
		h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to retrieve active user statistics", http.StatusInternalServerError)
		return
	}

	// Calculate quota utilization
	totalQuotaBytes := int64(0)
	for _, user := range storageByUser {
		totalQuotaBytes += user.QuotaBytes
	}

	quotaUtilization := float64(0)
	if totalQuotaBytes > 0 {
		quotaUtilization = float64(systemStats.TotalSizeBytes) / float64(totalQuotaBytes) * 100
	}

	// Get total user registrations (all-time count)
	totalRegistrations, err := h.getTotalUserRegistrations()
	if err != nil {
		h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to retrieve total user registrations", http.StatusInternalServerError)
		return
	}

	response := AdminStatsResponse{
		TotalUsers:                systemStats.TotalUsers,
		TotalFiles:                systemStats.TotalFiles,
		TotalSizeBytes:            systemStats.TotalSizeBytes,
		TotalQuotaBytes:           totalQuotaBytes,
		QuotaUtilizationPercent:   quotaUtilization,
		FilesByType:               filesByType,
		TotalUserRegistrations:    totalRegistrations,
		UsersByRegistrationDate:   usersByRegistration,
		StorageByUser:             storageByUser,
		MostActiveUsers:           activeUsers,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// HandleAdminDeleteFile handles DELETE /api/v1/admin/files/{id} - admin delete any file
// @Summary Delete any file (Admin)
// @Description Admin can delete any file regardless of ownership (includes S3 cleanup)
// @Tags Admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "File ID (UUID)"
// @Success 204 "File successfully deleted"
// @Failure 400 {object} ErrorResponse "Invalid file ID format"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 403 {object} ErrorResponse "Admin privileges required"
// @Failure 404 {object} ErrorResponse "File not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /admin/files/{id} [delete]
func (h *AdminHandlers) HandleAdminDeleteFile(w http.ResponseWriter, r *http.Request) {
	// Validate admin authentication
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		h.writeErrorResponse(w, "UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
		return
	}

	// Check if user is admin
	isAdmin, err := h.authService.IsAdmin(userID)
	if err != nil {
		h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to verify admin privileges", http.StatusInternalServerError)
		return
	}

	if !isAdmin {
		h.writeErrorResponse(w, "ADMIN_REQUIRED", "Admin privileges required to access this endpoint", http.StatusForbidden)
		return
	}

	// Get file ID from URL
	vars := mux.Vars(r)
	fileIDStr := vars["id"]
	
	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		h.writeErrorResponse(w, "INVALID_FILE_ID", "Invalid file ID format", http.StatusBadRequest)
		return
	}

	// Delete file (admin can delete any file)
	err = h.fileService.DeleteFileAsAdmin(fileID)
	if err != nil {
		if err == services.ErrFileNotFound {
			h.writeErrorResponse(w, "FILE_NOT_FOUND", "File not found", http.StatusNotFound)
		} else {
			h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to delete file", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AdminQueryParams represents parsed query parameters for admin endpoints
type AdminQueryParams struct {
	UserID         *uuid.UUID
	UserEmail      string
	UploadedAfter  *time.Time
	UploadedBefore *time.Time
	Sort           string
	Order          string
	FromDate       *time.Time
	ToDate         *time.Time
	GroupBy        string
	Page           int
	PageSize       int
}

// ValidationError represents a validation error with specific code
type ValidationError struct {
	Code       string
	Message    string
	StatusCode int
}

// parseAdminFilesQuery parses and validates query parameters for admin files endpoint
func (h *AdminHandlers) parseAdminFilesQuery(r *http.Request) (*AdminQueryParams, *ValidationError) {
	params := &AdminQueryParams{
		Page:     1,
		PageSize: 50,
		Sort:     "created_at", // This will be the actual column name
		Order:    "desc",
	}

	// Parse user_id filter
	if userIDStr := r.URL.Query().Get("user_id"); userIDStr != "" {
		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			return nil, &ValidationError{
				Code:       "INVALID_USER_ID",
				Message:    "Invalid user ID format",
				StatusCode: http.StatusBadRequest,
			}
		}
		params.UserID = &userID
	}

	// Parse user_email filter
	params.UserEmail = r.URL.Query().Get("user_email")

	// Parse date filters
	if uploadedAfterStr := r.URL.Query().Get("uploaded_after"); uploadedAfterStr != "" {
		uploadedAfter, err := time.Parse("2006-01-02", uploadedAfterStr)
		if err != nil {
			return nil, &ValidationError{
				Code:       "INVALID_DATE_FORMAT",
				Message:    "Invalid uploaded_after date format, expected YYYY-MM-DD",
				StatusCode: http.StatusBadRequest,
			}
		}
		params.UploadedAfter = &uploadedAfter
	}

	if uploadedBeforeStr := r.URL.Query().Get("uploaded_before"); uploadedBeforeStr != "" {
		uploadedBefore, err := time.Parse("2006-01-02", uploadedBeforeStr)
		if err != nil {
			return nil, &ValidationError{
				Code:       "INVALID_DATE_FORMAT",
				Message:    "Invalid uploaded_before date format, expected YYYY-MM-DD",
				StatusCode: http.StatusBadRequest,
			}
		}
		params.UploadedBefore = &uploadedBefore
	}

	// Parse sort and order
	if sort := r.URL.Query().Get("sort"); sort != "" {
		// Map user-friendly sort names to actual column names
		sortColumns := map[string]string{
			"created_at": "created_at",
			"size":       "size_bytes", 
			"filename":   "original_filename",
		}
		if column, valid := sortColumns[sort]; !valid {
			return nil, &ValidationError{
				Code:       "INVALID_SORT",
				Message:    "Invalid sort field, must be one of: created_at, size, filename",
				StatusCode: http.StatusBadRequest,
			}
		} else {
			params.Sort = column
		}
	}

	if order := r.URL.Query().Get("order"); order != "" {
		if order != "asc" && order != "desc" {
			return nil, &ValidationError{
				Code:       "INVALID_ORDER",
				Message:    "Invalid order, must be 'asc' or 'desc'",
				StatusCode: http.StatusBadRequest,
			}
		}
		params.Order = order
	}

	// Parse pagination
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		page, err := strconv.Atoi(pageStr)
		if err != nil || page < 1 {
			return nil, &ValidationError{
				Code:       "INVALID_PAGE",
				Message:    "Invalid page number",
				StatusCode: http.StatusBadRequest,
			}
		}
		params.Page = page
	}

	if pageSizeStr := r.URL.Query().Get("page_size"); pageSizeStr != "" {
		pageSize, err := strconv.Atoi(pageSizeStr)
		if err != nil || pageSize < 1 || pageSize > 100 {
			return nil, &ValidationError{
				Code:       "INVALID_PAGE_SIZE",
				Message:    "Invalid page size, must be between 1 and 100",
				StatusCode: http.StatusBadRequest,
			}
		}
		params.PageSize = pageSize
	}

	return params, nil
}

// parseAdminStatsQuery parses and validates query parameters for admin stats endpoint
func (h *AdminHandlers) parseAdminStatsQuery(r *http.Request) (*AdminQueryParams, *ValidationError) {
	params := &AdminQueryParams{}

	// Parse from date
	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		fromDate, err := time.Parse("2006-01-02", fromStr)
		if err != nil {
			return nil, &ValidationError{
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
			return nil, &ValidationError{
				Code:       "INVALID_DATE_FORMAT",
				Message:    "Invalid to date format, expected YYYY-MM-DD",
				StatusCode: http.StatusBadRequest,
			}
		}
		params.ToDate = &toDate
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
			return nil, &ValidationError{
				Code:       "INVALID_GROUPING",
				Message:    "Invalid grouping parameter, must be one of: day, week, month, year",
				StatusCode: http.StatusBadRequest,
			}
		}
		params.GroupBy = groupBy
	}

	return params, nil
}

// Helper methods for data retrieval

// getAdminFiles retrieves files with admin-specific information
func (h *AdminHandlers) getAdminFiles(params *AdminQueryParams) ([]AdminFileEntry, PaginationInfo, error) {
	// Build WHERE clause based on filters
	var whereClauses []string
	var args []interface{}
	argIndex := 1

	if params.UserID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("f.owner_id = $%d", argIndex))
		args = append(args, *params.UserID)
		argIndex++
	}

	if params.UserEmail != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("u.email ILIKE $%d", argIndex))
		args = append(args, "%"+params.UserEmail+"%")
		argIndex++
	}

	if params.UploadedAfter != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("f.created_at >= $%d", argIndex))
		args = append(args, *params.UploadedAfter)
		argIndex++
	}

	if params.UploadedBefore != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("f.created_at <= $%d", argIndex))
		args = append(args, *params.UploadedBefore)
		argIndex++
	}

	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Get total count
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM files f
		JOIN users u ON f.owner_id = u.id
		%s
	`, whereClause)

	var total int
	err := h.statsService.GetDB().QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, PaginationInfo{}, fmt.Errorf("failed to get total count: %w", err)
	}

	// Build main query with pagination
	offset := (params.Page - 1) * params.PageSize
	
	query := fmt.Sprintf(`
		SELECT 
			f.id, f.original_filename, f.size_bytes, f.mime_type, 
			f.created_at, u.email, u.id, f.is_public, f.download_count
		FROM files f
		JOIN users u ON f.owner_id = u.id
		%s
		ORDER BY f.%s %s
		LIMIT $%d OFFSET $%d
	`, whereClause, params.Sort, strings.ToUpper(params.Order), argIndex, argIndex+1)

	args = append(args, params.PageSize, offset)

	rows, err := h.statsService.GetDB().Query(query, args...)
	if err != nil {
		return nil, PaginationInfo{}, fmt.Errorf("failed to query admin files: %w", err)
	}
	defer rows.Close()

	files := make([]AdminFileEntry, 0) // Initialize as empty slice instead of nil
	for rows.Next() {
		var file AdminFileEntry
		err := rows.Scan(
			&file.ID, &file.Filename, &file.Size, &file.MimeType,
			&file.UploadDate, &file.UserEmail, &file.UserID, &file.IsPublic, &file.DownloadCount,
		)
		if err != nil {
			return nil, PaginationInfo{}, fmt.Errorf("failed to scan file row: %w", err)
		}
		files = append(files, file)
	}

	if err := rows.Err(); err != nil {
		return nil, PaginationInfo{}, fmt.Errorf("error iterating file rows: %w", err)
	}

	// Build pagination info
	totalPages := (total + params.PageSize - 1) / params.PageSize
	pagination := PaginationInfo{
		Page:       params.Page,
		PageSize:   params.PageSize,
		Total:      total,
		TotalPages: totalPages,
	}

	return files, pagination, nil
}

// getSystemFilesByType retrieves system-wide file type statistics
func (h *AdminHandlers) getSystemFilesByType() (map[string]int, error) {
	query := `
		SELECT mime_type, COUNT(*) as count
		FROM files 
		GROUP BY mime_type
		ORDER BY count DESC
	`

	rows, err := h.statsService.GetDB().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query system files by type: %w", err)
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

	return filesByType, nil
}

// getTotalUserRegistrations retrieves the total count of all user registrations (all-time)
func (h *AdminHandlers) getTotalUserRegistrations() (int, error) {
	query := `SELECT COUNT(*) FROM users`
	var total int
	err := h.statsService.GetDB().QueryRow(query).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to get total user registrations: %w", err)
	}
	return total, nil
}

// getUserRegistrationHistory retrieves user registration statistics
func (h *AdminHandlers) getUserRegistrationHistory(params *AdminQueryParams) ([]RegistrationEntry, error) {
	// Default to all-time if no date range specified
	var fromDate, toDate time.Time
	hasDateRange := false

	if params.FromDate != nil {
		fromDate = *params.FromDate
		hasDateRange = true
	}
	if params.ToDate != nil {
		toDate = *params.ToDate
		hasDateRange = true
	} else if hasDateRange {
		toDate = time.Now()
	}

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

	var rows *sql.Rows
	var err error

	if hasDateRange {
		query := fmt.Sprintf(`
			SELECT 
				DATE_TRUNC('%s', created_at) as date_group,
				COUNT(*) as count
			FROM users 
			WHERE created_at >= $1 AND created_at <= $2
			GROUP BY date_group
			ORDER BY date_group ASC
		`, truncFormat)
		rows, err = h.statsService.GetDB().Query(query, fromDate, toDate)
	} else {
		// No date range - return all-time registrations grouped
		query := fmt.Sprintf(`
			SELECT 
				DATE_TRUNC('%s', created_at) as date_group,
				COUNT(*) as count
			FROM users 
			GROUP BY date_group
			ORDER BY date_group ASC
		`, truncFormat)
		rows, err = h.statsService.GetDB().Query(query)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query user registration history: %w", err)
	}
	defer rows.Close()

	var history []RegistrationEntry
	for rows.Next() {
		var dateGroup time.Time
		var count int
		
		err := rows.Scan(&dateGroup, &count)
		if err != nil {
			return nil, fmt.Errorf("failed to scan registration history row: %w", err)
		}

		entry := RegistrationEntry{
			Date:  dateGroup.Format(dateFormat),
			Count: count,
		}
		history = append(history, entry)
	}

	return history, nil
}

// getStorageByUser retrieves storage usage statistics by user
func (h *AdminHandlers) getStorageByUser() ([]UserStorageEntry, error) {
	query := `
		SELECT 
			u.id, u.email, u.storage_quota_bytes,
			COUNT(f.id) as file_count,
			COALESCE(SUM(DISTINCT b.size_bytes), 0) as total_size
		FROM users u
		LEFT JOIN files f ON f.owner_id = u.id
		LEFT JOIN blobs b ON f.blob_hash = b.hash
		GROUP BY u.id, u.email, u.storage_quota_bytes
		ORDER BY total_size DESC
		LIMIT 50
	`

	rows, err := h.statsService.GetDB().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query storage by user: %w", err)
	}
	defer rows.Close()

	var users []UserStorageEntry
	for rows.Next() {
		var user UserStorageEntry
		err := rows.Scan(
			&user.UserID, &user.UserEmail, &user.QuotaBytes,
			&user.FileCount, &user.TotalSizeBytes,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user storage row: %w", err)
		}
		users = append(users, user)
	}

	return users, nil
}

// getMostActiveUsers retrieves most active users by upload activity
func (h *AdminHandlers) getMostActiveUsers() ([]ActiveUserEntry, error) {
	query := `
		SELECT 
			u.id, u.email,
			COUNT(f.id) as file_count,
			MAX(f.created_at) as last_upload,
			COALESCE(SUM(f.download_count), 0) as total_downloads
		FROM users u
		LEFT JOIN files f ON f.owner_id = u.id
		GROUP BY u.id, u.email
		HAVING COUNT(f.id) > 0
		ORDER BY file_count DESC, total_downloads DESC
		LIMIT 20
	`

	rows, err := h.statsService.GetDB().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query active users: %w", err)
	}
	defer rows.Close()

	var users []ActiveUserEntry
	for rows.Next() {
		var user ActiveUserEntry
		var lastUpload *time.Time
		err := rows.Scan(
			&user.UserID, &user.UserEmail, &user.FileCount, 
			&lastUpload, &user.TotalDownloads,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan active user row: %w", err)
		}
		user.LastUpload = lastUpload
		users = append(users, user)
	}

	return users, nil
}

// extractUserFromAuth extracts user ID from JWT token in Authorization header
func (h *AdminHandlers) extractUserFromAuth(r *http.Request) (uuid.UUID, error) {
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

// HandleAdminUpdateUserQuota handles PATCH /admin/users/{id}/quota
func (h *AdminHandlers) HandleAdminUpdateUserQuota(w http.ResponseWriter, r *http.Request) {
	// Extract user from authentication context
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		h.writeErrorResponse(w, "UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
		return
	}

	// Check admin privileges
	isAdmin, err := h.authService.IsAdmin(userID)
	if err != nil {
		h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to verify admin privileges", http.StatusInternalServerError)
		return
	}
	if !isAdmin {
		h.writeErrorResponse(w, "ADMIN_REQUIRED", "Admin privileges required to access this endpoint", http.StatusForbidden)
		return
	}

	// TODO: Implement quota update logic
	h.writeErrorResponse(w, "NOT_IMPLEMENTED", "User quota update not yet implemented", http.StatusNotImplemented)
}

// HandleAdminSuspendUser handles POST /admin/users/{id}/suspend
func (h *AdminHandlers) HandleAdminSuspendUser(w http.ResponseWriter, r *http.Request) {
	// Extract user from authentication context
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		h.writeErrorResponse(w, "UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
		return
	}

	// Check admin privileges
	isAdmin, err := h.authService.IsAdmin(userID)
	if err != nil {
		h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to verify admin privileges", http.StatusInternalServerError)
		return
	}
	if !isAdmin {
		h.writeErrorResponse(w, "ADMIN_REQUIRED", "Admin privileges required to access this endpoint", http.StatusForbidden)
		return
	}

	// TODO: Implement user suspension logic
	h.writeErrorResponse(w, "NOT_IMPLEMENTED", "User suspension not yet implemented", http.StatusNotImplemented)
}

// PromoteUserRequest represents the request to promote a user to admin
type PromoteUserRequest struct {
	UserID string `json:"user_id"`
}

// HandleAdminSignup handles POST /api/v1/admin/signup
// @Summary Register a new admin user
// @Description Create a new admin user account with email and password
// @Tags Admin
// @Accept json
// @Produce json
// @Param request body SignupRequest true "Admin registration data"
// @Success 201 {object} AuthResponse "Admin user successfully registered"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 409 {object} ErrorResponse "Email already exists"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /admin/signup [post]
func (h *AdminHandlers) HandleAdminSignup(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, "INVALID_JSON", "Invalid JSON in request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Email == "" {
		h.writeErrorResponse(w, "MISSING_EMAIL", "Email is required", http.StatusBadRequest)
		return
	}

	if req.Password == "" {
		h.writeErrorResponse(w, "MISSING_PASSWORD", "Password is required", http.StatusBadRequest)
		return
	}

	// Validate email format (basic validation)
	if !strings.Contains(req.Email, "@") {
		h.writeErrorResponse(w, "INVALID_EMAIL", "Invalid email format", http.StatusBadRequest)
		return
	}

	// Validate password strength
	if len(req.Password) < 8 {
		h.writeErrorResponse(w, "WEAK_PASSWORD", "Password must be at least 8 characters long", http.StatusBadRequest)
		return
	}

	// Normalize email (lowercase)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	// Try to sign up the admin user
	signupReq := &services.SignUpRequest{
		Email:    req.Email,
		Password: req.Password,
	}
	
	authResp, err := h.authService.SignUpAdmin(signupReq)
	if err != nil {
		// Check for duplicate email error
		if err == services.ErrUserExists || strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			h.writeErrorResponse(w, "EMAIL_EXISTS", "A user with this email already exists", http.StatusConflict)
			return
		}

		h.writeErrorResponse(w, "SIGNUP_FAILED", "Failed to create admin account", http.StatusInternalServerError)
		return
	}

	// Prepare response
	response := AuthResponse{
		Token: authResp.Token,
		User: &UserResponseData{
			ID:                authResp.User.ID,
			Email:             authResp.User.Email,
			Name:              req.Name,
			Role:              authResp.User.Role,
			RateLimitRps:      authResp.User.RateLimitRps,
			StorageQuotaBytes: authResp.User.StorageQuotaBytes,
			CreatedAt:         authResp.User.CreatedAt,
		},
	}

	// Send successful response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// HandlePromoteToAdmin handles POST /api/v1/admin/promote
// @Summary Promote user to admin
// @Description Promote an existing user to admin role (admin only)
// @Tags Admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body PromoteUserRequest true "User ID to promote"
// @Success 200 {object} UserResponseData "User successfully promoted to admin"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Forbidden - admin access required"
// @Failure 404 {object} ErrorResponse "User not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /admin/promote [post]
func (h *AdminHandlers) HandlePromoteToAdmin(w http.ResponseWriter, r *http.Request) {
	// Validate admin authentication
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		h.writeErrorResponse(w, "UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
		return
	}

	// Check if user is admin
	isAdmin, err := h.authService.IsAdmin(userID)
	if err != nil {
		h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to verify admin privileges", http.StatusInternalServerError)
		return
	}

	if !isAdmin {
		h.writeErrorResponse(w, "FORBIDDEN", "Admin privileges required", http.StatusForbidden)
		return
	}

	// Parse request body
	var req PromoteUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, "INVALID_JSON", "Invalid JSON in request body", http.StatusBadRequest)
		return
	}

	// Validate user ID
	if req.UserID == "" {
		h.writeErrorResponse(w, "MISSING_USER_ID", "User ID is required", http.StatusBadRequest)
		return
	}

	// Parse user ID
	targetUserID, err := uuid.Parse(req.UserID)
	if err != nil {
		h.writeErrorResponse(w, "INVALID_USER_ID", "Invalid user ID format", http.StatusBadRequest)
		return
	}

	// Promote user to admin
	promotedUser, err := h.authService.PromoteToAdmin(targetUserID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.writeErrorResponse(w, "USER_NOT_FOUND", "User not found", http.StatusNotFound)
			return
		}

		h.writeErrorResponse(w, "PROMOTION_FAILED", "Failed to promote user to admin", http.StatusInternalServerError)
		return
	}

	// Prepare response
	response := UserResponseData{
		ID:                promotedUser.ID,
		Email:             promotedUser.Email,
		Role:              promotedUser.Role,
		RateLimitRps:      promotedUser.RateLimitRps,
		StorageQuotaBytes: promotedUser.StorageQuotaBytes,
		CreatedAt:         promotedUser.CreatedAt,
	}

	// Send successful response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// writeErrorResponse writes a standardized error response
func (h *AdminHandlers) writeErrorResponse(w http.ResponseWriter, code, message string, statusCode int) {
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