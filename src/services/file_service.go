package services

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"securevault-backend/src/models"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

var (
	ErrFileNotFound         = errors.New("file not found")
	ErrFileAlreadyExists    = errors.New("file already exists")
	ErrInvalidFileID        = errors.New("invalid file ID")
	ErrUnauthorizedAccess   = errors.New("unauthorized access to file")
	ErrInvalidSearchParams  = errors.New("invalid search parameters")
	ErrShareLinkNotFound    = errors.New("share link not found")
	ErrShareLinkExpired     = errors.New("share link has expired")
	ErrShareLinkInactive    = errors.New("share link is inactive")
	ErrInvalidShareToken    = errors.New("invalid share token format")
)

// FileService handles file-related business logic
type FileService struct {
	db             *sql.DB
	storageService *StorageService
}

// NewFileService creates a new FileService
func NewFileService(db *sql.DB, storageService *StorageService) *FileService {
	return &FileService{
		db:             db,
		storageService: storageService,
	}
}

// CreateFile creates a new file in the database
func (s *FileService) CreateFile(ownerID uuid.UUID, blobHash, originalFilename, mimeType string, sizeBytes int64, tags []string) (*models.File, error) {
	return s.CreateFileInFolder(ownerID, blobHash, originalFilename, mimeType, sizeBytes, tags, nil)
}

// CreateFileInFolder creates a new file in the database with optional folder placement
func (s *FileService) CreateFileInFolder(ownerID uuid.UUID, blobHash, originalFilename, mimeType string, sizeBytes int64, tags []string, folderID *uuid.UUID) (*models.File, error) {
	if ownerID == uuid.Nil {
		return nil, errors.New("owner ID cannot be nil")
	}
	
	if blobHash == "" {
		return nil, errors.New("blob hash cannot be empty")
	}

	if originalFilename == "" {
		return nil, errors.New("original filename cannot be empty")
	}

	if sizeBytes <= 0 {
		return nil, errors.New("file size must be positive")
	}

	// If folderID is provided, verify it exists and belongs to the same owner
	if folderID != nil {
		var folderOwnerID uuid.UUID
		err := s.db.QueryRow(
			"SELECT owner_id FROM folders WHERE id = $1",
			*folderID,
		).Scan(&folderOwnerID)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, fmt.Errorf("folder not found")
			}
			return nil, fmt.Errorf("failed to validate folder: %w", err)
		}
		
		if folderOwnerID != ownerID {
			return nil, fmt.Errorf("folder does not belong to the same owner")
		}
	}

	file := models.NewFile(ownerID, blobHash, originalFilename, mimeType, sizeBytes)
	
	// Set folder if provided
	if folderID != nil {
		file.FolderID = folderID
	}
	
	// Set tags if provided
	if tags != nil {
		file.Tags = tags
	}

	query := `
		INSERT INTO files (id, owner_id, blob_hash, original_filename, mime_type, size_bytes, 
		                  is_public, download_count, tags, folder_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	_, err := s.db.Exec(query,
		file.ID,
		file.OwnerID,
		file.BlobHash,
		file.OriginalFilename,
		file.MimeType,
		file.SizeBytes,
		file.IsPublic,
		file.DownloadCount,
		pq.Array(file.Tags),
		file.FolderID,
		file.CreatedAt,
		file.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	return file, nil
}

// GetFileByID retrieves a file by its ID
func (s *FileService) GetFileByID(fileID uuid.UUID) (*models.File, error) {
	if fileID == uuid.Nil {
		return nil, ErrInvalidFileID
	}

	var file models.File
	query := `
		SELECT id, owner_id, blob_hash, original_filename, mime_type, size_bytes,
		       is_public, download_count, tags, folder_id, created_at, updated_at, deleted_at
		FROM files 
		WHERE id = $1 AND deleted_at IS NULL
	`

	err := s.db.QueryRow(query, fileID).Scan(
		&file.ID,
		&file.OwnerID,
		&file.BlobHash,
		&file.OriginalFilename,
		&file.MimeType,
		&file.SizeBytes,
		&file.IsPublic,
		&file.DownloadCount,
		pq.Array(&file.Tags),
		&file.FolderID,
		&file.CreatedAt,
		&file.UpdatedAt,
		&file.DeletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("failed to get file by ID: %w", err)
	}

	return &file, nil
}

// GetFilesByOwner retrieves all files owned by a user with pagination
func (s *FileService) GetFilesByOwner(ownerID uuid.UUID, offset, limit int) ([]*models.File, error) {
	if ownerID == uuid.Nil {
		return nil, errors.New("owner ID cannot be nil")
	}

	if limit <= 0 {
		limit = 50 // Default limit
	}
	if limit > 1000 {
		limit = 1000 // Maximum limit
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT id, owner_id, blob_hash, original_filename, mime_type, size_bytes,
		       is_public, download_count, tags, folder_id, created_at, updated_at, deleted_at
		FROM files 
		WHERE owner_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.db.Query(query, ownerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get files by owner: %w", err)
	}
	defer rows.Close()

	var files []*models.File
	for rows.Next() {
		var file models.File
		err := rows.Scan(
			&file.ID,
			&file.OwnerID,
			&file.BlobHash,
			&file.OriginalFilename,
			&file.MimeType,
			&file.SizeBytes,
			&file.IsPublic,
			&file.DownloadCount,
			pq.Array(&file.Tags),
			&file.FolderID,
			&file.CreatedAt,
			&file.UpdatedAt,
			&file.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file: %w", err)
		}
		files = append(files, &file)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate files: %w", err)
	}

	return files, nil
}

// UpdateFile updates an existing file's metadata
func (s *FileService) UpdateFile(file *models.File, requesterID uuid.UUID) error {
	if file == nil {
		return errors.New("file cannot be nil")
	}

	if file.ID == uuid.Nil {
		return ErrInvalidFileID
	}

	// Check ownership
	if !file.IsOwnedBy(requesterID) {
		return ErrUnauthorizedAccess
	}

	// Update timestamp
	file.UpdateTimestamp()

	query := `
		UPDATE files 
		SET original_filename = $2, mime_type = $3, is_public = $4, 
		    download_count = $5, tags = $6, updated_at = $7
		WHERE id = $1 AND owner_id = $8
	`

	result, err := s.db.Exec(query,
		file.ID,
		file.OriginalFilename,
		file.MimeType,
		file.IsPublic,
		file.DownloadCount,
		pq.Array(file.Tags),
		file.UpdatedAt,
		requesterID, // Additional security check
	)

	if err != nil {
		return fmt.Errorf("failed to update file: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrFileNotFound
	}

	return nil
}

// DeleteFile deletes a file from the database and S3 storage
func (s *FileService) DeleteFile(fileID uuid.UUID, requesterID uuid.UUID) error {
	if fileID == uuid.Nil {
		return ErrInvalidFileID
	}

	if requesterID == uuid.Nil {
		return errors.New("requester ID cannot be nil")
	}

	// First check if file exists and requester owns it
	file, err := s.GetFileByID(fileID)
	if err != nil {
		return err
	}

	if !file.IsOwnedBy(requesterID) {
		return ErrUnauthorizedAccess
	}

	// Start a transaction for atomic deletion
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete the file record
	query := `DELETE FROM files WHERE id = $1 AND owner_id = $2`

	result, err := tx.Exec(query, fileID, requesterID)
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrFileNotFound
	}

	// Decrement blob reference count
	blobQuery := `
		UPDATE blobs 
		SET ref_count = ref_count - 1
		WHERE hash = $1 AND ref_count > 0
	`

	_, err = tx.Exec(blobQuery, file.BlobHash)
	if err != nil {
		return fmt.Errorf("failed to decrement blob reference count: %w", err)
	}

	// Check if blob should be deleted (ref_count = 0)
	var refCount int
	checkQuery := `SELECT ref_count FROM blobs WHERE hash = $1`
	err = tx.QueryRow(checkQuery, file.BlobHash).Scan(&refCount)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check blob reference count: %w", err)
	}

	// Commit the transaction first
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// If blob has no more references, delete it from storage
	if refCount == 0 {
		err = s.storageService.DeleteBlob(file.BlobHash)
		if err != nil {
			// Log the error but don't fail the entire operation
			// The file record is already deleted from the database
			fmt.Printf("Warning: failed to delete blob from storage: %v\n", err)
		}
	}

	return nil
}

// SearchFiles searches for files based on various criteria
func (s *FileService) SearchFiles(params *SearchParams, requesterID uuid.UUID) ([]*models.File, error) {
	if params == nil {
		return nil, ErrInvalidSearchParams
	}

	// Validate pagination parameters
	if params.Limit <= 0 {
		params.Limit = 50 // Default limit
	}
	if params.Limit > 1000 {
		params.Limit = 1000 // Maximum limit
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	// Build the query dynamically based on search parameters
	var conditions []string
	var args []interface{}
	argCount := 0

	// Base query with owner filter (users can only search their own files)
	baseQuery := `
		SELECT id, owner_id, blob_hash, original_filename, mime_type, size_bytes,
		       is_public, download_count, tags, folder_id, created_at, updated_at, deleted_at
		FROM files 
		WHERE owner_id = $1 AND deleted_at IS NULL
	`
	
	args = append(args, requesterID)
	argCount = 1

	// Add filename search
	if params.Filename != "" {
		argCount++
		conditions = append(conditions, fmt.Sprintf("original_filename ILIKE $%d", argCount))
		args = append(args, "%"+params.Filename+"%")
	}

	// Add MIME type filter
	if params.MimeType != "" {
		argCount++
		conditions = append(conditions, fmt.Sprintf("mime_type = $%d", argCount))
		args = append(args, params.MimeType)
	}

	// Add size range filters
	if params.MinSize > 0 {
		argCount++
		conditions = append(conditions, fmt.Sprintf("size_bytes >= $%d", argCount))
		args = append(args, params.MinSize)
	}

	if params.MaxSize > 0 {
		argCount++
		conditions = append(conditions, fmt.Sprintf("size_bytes <= $%d", argCount))
		args = append(args, params.MaxSize)
	}

	// Add date range filters
	if !params.CreatedAfter.IsZero() {
		argCount++
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argCount))
		args = append(args, params.CreatedAfter)
	}

	if !params.CreatedBefore.IsZero() {
		argCount++
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", argCount))
		args = append(args, params.CreatedBefore)
	}

	// Add tag filter
	if len(params.Tags) > 0 {
		argCount++
		conditions = append(conditions, fmt.Sprintf("tags && $%d", argCount))
		args = append(args, pq.Array(params.Tags))
	}

	// Add public filter
	if params.IsPublic != nil {
		argCount++
		conditions = append(conditions, fmt.Sprintf("is_public = $%d", argCount))
		args = append(args, *params.IsPublic)
	}

	// Combine all conditions
	if len(conditions) > 0 {
		baseQuery += " AND " + strings.Join(conditions, " AND ")
	}

	// Add ordering
	orderBy := "created_at DESC" // Default ordering
	if params.OrderBy != "" {
		switch params.OrderBy {
		case "filename":
			orderBy = "original_filename ASC"
		case "size":
			orderBy = "size_bytes DESC"
		case "created_at":
			orderBy = "created_at DESC"
		case "download_count":
			orderBy = "download_count DESC"
		}
	}

	baseQuery += fmt.Sprintf(" ORDER BY %s", orderBy)

	// Add pagination
	argCount++
	baseQuery += fmt.Sprintf(" LIMIT $%d", argCount)
	args = append(args, params.Limit)

	argCount++
	baseQuery += fmt.Sprintf(" OFFSET $%d", argCount)
	args = append(args, params.Offset)

	// Execute query
	rows, err := s.db.Query(baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search files: %w", err)
	}
	defer rows.Close()

	var files []*models.File
	for rows.Next() {
		var file models.File
		err := rows.Scan(
			&file.ID,
			&file.OwnerID,
			&file.BlobHash,
			&file.OriginalFilename,
			&file.MimeType,
			&file.SizeBytes,
			&file.IsPublic,
			&file.DownloadCount,
			pq.Array(&file.Tags),
			&file.FolderID,
			&file.CreatedAt,
			&file.UpdatedAt,
			&file.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file: %w", err)
		}
		files = append(files, &file)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate files: %w", err)
	}

	return files, nil
}

// SearchParams represents search parameters for file search
type SearchParams struct {
	Filename      string    `json:"filename"`
	MimeType      string    `json:"mime_type"`
	MinSize       int64     `json:"min_size"`
	MaxSize       int64     `json:"max_size"`
	CreatedAfter  time.Time `json:"created_after"`
	CreatedBefore time.Time `json:"created_before"`
	Tags          []string  `json:"tags"`
	IsPublic      *bool     `json:"is_public"`
	OrderBy       string    `json:"order_by"`
	Limit         int       `json:"limit"`
	Offset        int       `json:"offset"`
}

// IncrementDownloadCount increments the download count for a file
func (s *FileService) IncrementDownloadCount(fileID uuid.UUID) error {
	if fileID == uuid.Nil {
		return ErrInvalidFileID
	}

	query := `
		UPDATE files 
		SET download_count = download_count + 1, updated_at = NOW()
		WHERE id = $1
	`

	result, err := s.db.Exec(query, fileID)
	if err != nil {
		return fmt.Errorf("failed to increment download count: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrFileNotFound
	}

	return nil
}

// ToggleFilePublic toggles the public status of a file and manages sharelinks
func (s *FileService) ToggleFilePublic(fileID uuid.UUID, requesterID uuid.UUID) (*models.File, *models.ShareLink, error) {
	// Get the current file
	file, err := s.GetFileByID(fileID)
	if err != nil {
		return nil, nil, err
	}

	// Check ownership
	if !file.IsOwnedBy(requesterID) {
		return nil, nil, ErrUnauthorizedAccess
	}

	// Toggle public status and use SetFilePublic to manage sharelinks
	newPublicStatus := !file.IsPublic
	return s.SetFilePublic(fileID, newPublicStatus, requesterID)
}

// SetFilePublic sets the public status of a file and manages sharelinks
func (s *FileService) SetFilePublic(fileID uuid.UUID, isPublic bool, requesterID uuid.UUID) (*models.File, *models.ShareLink, error) {
	// Get the current file
	file, err := s.GetFileByID(fileID)
	if err != nil {
		return nil, nil, err
	}

	// Check ownership
	if !file.IsOwnedBy(requesterID) {
		return nil, nil, ErrUnauthorizedAccess
	}

	// Set public status
	file.SetPublic(isPublic)

	// Update in database
	err = s.UpdateFile(file, requesterID)
	if err != nil {
		return nil, nil, err
	}

	var shareLink *models.ShareLink
	if isPublic {
		// Create or enable sharelink when making file public
		shareLink, err = s.EnableShareLink(fileID, requesterID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to enable share link: %w", err)
		}
	} else {
		// Delete sharelink permanently when making file private
		err = s.DeleteShareLink(fileID, requesterID)
		if err != nil && err != ErrShareLinkNotFound {
			// Log the error but don't fail the operation
			// In some cases, the sharelink might not exist yet
		}
	}

	return file, shareLink, nil
}

// GetFileStats returns statistics about files
func (s *FileService) GetFileStats(userID uuid.UUID) (*FileStats, error) {
	query := `
		SELECT 
			COUNT(*) as total_files,
			COALESCE(SUM(size_bytes), 0) as total_size,
			COUNT(CASE WHEN is_public = true THEN 1 END) as public_files,
			COALESCE(SUM(download_count), 0) as total_downloads
		FROM files
		WHERE owner_id = $1 AND deleted_at IS NULL
	`

	var stats FileStats
	err := s.db.QueryRow(query, userID).Scan(
		&stats.TotalFiles,
		&stats.TotalSize,
		&stats.PublicFiles,
		&stats.TotalDownloads,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get file stats: %w", err)
	}

	return &stats, nil
}

// FileStats represents statistics about files
type FileStats struct {
	TotalFiles     int   `json:"total_files"`
	TotalSize      int64 `json:"total_size"`
	PublicFiles    int   `json:"public_files"`
	TotalDownloads int   `json:"total_downloads"`
}

// FileListRequest represents parameters for listing files with enhanced filtering
type FileListRequest struct {
	// Ownership filtering
	OwnerID *uuid.UUID `json:"owner_id,omitempty"`
	
	// Folder filtering
	FolderID *uuid.UUID `json:"folder_id,omitempty"`     // Filter by folder (nil for root level)
	
	// Search and filtering
	Search       string   `json:"search,omitempty"`         // Search in filename
	MimeTypes    []string `json:"mime_types,omitempty"`     // Filter by MIME types
	Tags         []string `json:"tags,omitempty"`           // Filter by tags (must have all)
	IsPublic     *bool    `json:"is_public,omitempty"`      // Filter by public status
	MinSizeBytes *int64   `json:"min_size_bytes,omitempty"` // Minimum file size
	MaxSizeBytes *int64   `json:"max_size_bytes,omitempty"` // Maximum file size
	
	// Date range filtering
	CreatedAfter  *time.Time `json:"created_after,omitempty"`
	CreatedBefore *time.Time `json:"created_before,omitempty"`
	
	// Sorting
	SortBy    string `json:"sort_by,omitempty"`    // created_at, updated_at, size_bytes, original_filename, download_count
	SortOrder string `json:"sort_order,omitempty"` // asc, desc (default: desc)
	
	// Pagination
	Page     int `json:"page,omitempty"`      // Page number (1-based)
	PageSize int `json:"page_size,omitempty"` // Items per page (default: 20, max: 100)
}

// FileListResponse represents the response for file listing with pagination metadata
type FileListResponse struct {
	Files      []*models.File `json:"files"`
	Pagination PaginationInfo `json:"pagination"`
}

// PaginationInfo contains pagination metadata
type PaginationInfo struct {
	Page         int   `json:"page"`
	PageSize     int   `json:"page_size"`
	TotalItems   int64 `json:"total_items"`
	TotalPages   int   `json:"total_pages"`
	HasNext      bool  `json:"has_next"`
	HasPrevious  bool  `json:"has_previous"`
}

// ListFilesEnhanced provides enhanced file listing with better filtering and pagination
func (s *FileService) ListFilesEnhanced(req FileListRequest) (*FileListResponse, error) {
	log.Printf("[FILE-SERVICE] ListFilesEnhanced called - OwnerID: %v, FolderID: %v, Page: %d", req.OwnerID, req.FolderID, req.Page)
	
	// Set defaults
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	if req.PageSize > 100 {
		req.PageSize = 100
	}
	if req.SortBy == "" {
		req.SortBy = "created_at"
	}
	if req.SortOrder == "" {
		req.SortOrder = "desc"
	}

	// Build WHERE clause using hardcoded approach to avoid prepared statement issues
	whereClause := s.buildHardcodedWhereClause(req)
	
	// Build ORDER BY clause
	orderClause := s.buildOrderByClause(req.SortBy, req.SortOrder)

	// Calculate offset
	offset := (req.Page - 1) * req.PageSize

	// Count total items using hardcoded query to avoid prepared statement conflicts
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM files f %s", whereClause)
	log.Printf("[FILE-SERVICE] Count query: %s", countQuery)
	var totalItems int64
	err := s.db.QueryRow(countQuery).Scan(&totalItems)
	if err != nil {
		log.Printf("[FILE-SERVICE] Count query failed: %v", err)
		return nil, fmt.Errorf("failed to count files: %w", err)
	}
	log.Printf("[FILE-SERVICE] Count query success - total items: %d", totalItems)

	// Query files with pagination using hardcoded approach to avoid prepared statement conflicts
	query := fmt.Sprintf(`
		SELECT f.id, f.owner_id, f.blob_hash, f.original_filename, f.mime_type, f.size_bytes,
		       f.is_public, f.download_count, f.tags, f.folder_id, f.created_at, f.updated_at, f.deleted_at
		FROM files f %s %s
		LIMIT %d OFFSET %d`,
		whereClause, orderClause, req.PageSize, offset)

	log.Printf("[FILE-SERVICE] Files query: %s", query)
	rows, err := s.db.Query(query)
	if err != nil {
		log.Printf("[FILE-SERVICE] Files query failed: %v", err)
		return nil, fmt.Errorf("failed to query files: %w", err)
	}
	log.Printf("[FILE-SERVICE] Files query success")
	defer rows.Close()

	var files []*models.File
	for rows.Next() {
		var file models.File
		err := rows.Scan(
			&file.ID, &file.OwnerID, &file.BlobHash, &file.OriginalFilename, &file.MimeType, &file.SizeBytes,
			&file.IsPublic, &file.DownloadCount, pq.Array(&file.Tags), &file.FolderID, &file.CreatedAt, &file.UpdatedAt, &file.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file row: %w", err)
		}
		files = append(files, &file)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating file rows: %w", err)
	}

	// Calculate pagination info
	totalPages := int((totalItems + int64(req.PageSize) - 1) / int64(req.PageSize))
	
	pagination := PaginationInfo{
		Page:        req.Page,
		PageSize:    req.PageSize,
		TotalItems:  totalItems,
		TotalPages:  totalPages,
		HasNext:     req.Page < totalPages,
		HasPrevious: req.Page > 1,
	}

	return &FileListResponse{
		Files:      files,
		Pagination: pagination,
	}, nil
}

// buildEnhancedWhereClause constructs WHERE clause with parameters for enhanced file filtering
func (s *FileService) buildEnhancedWhereClause(req FileListRequest) (string, []interface{}) {
	var conditions []string
	var args []interface{}
	argIndex := 1

	// Always exclude trashed files
	conditions = append(conditions, "f.deleted_at IS NULL")

	// Owner filter with table alias
	if req.OwnerID != nil {
		conditions = append(conditions, fmt.Sprintf("f.owner_id = $%d", argIndex))
		args = append(args, *req.OwnerID)
		argIndex++
	}

	// Folder filter with table alias
	if req.FolderID != nil {
		conditions = append(conditions, fmt.Sprintf("f.folder_id = $%d", argIndex))
		args = append(args, *req.FolderID)
		argIndex++
	} else {
		// If FolderID is explicitly nil, we might want to filter for root level files
		// This behavior can be controlled by checking if the field was set intentionally
	}

	// Search in filename with table alias
	if req.Search != "" {
		conditions = append(conditions, fmt.Sprintf("f.original_filename ILIKE $%d", argIndex))
		args = append(args, "%"+req.Search+"%")
		argIndex++
	}

	// MIME type filter with table alias
	if len(req.MimeTypes) > 0 {
		conditions = append(conditions, fmt.Sprintf("f.mime_type = ANY($%d)", argIndex))
		args = append(args, pq.Array(req.MimeTypes))
		argIndex++
	}

	// Tags filter (file must have all specified tags) with table alias
	if len(req.Tags) > 0 {
		conditions = append(conditions, fmt.Sprintf("f.tags @> $%d", argIndex))
		args = append(args, pq.Array(req.Tags))
		argIndex++
	}

	// Public status filter with table alias
	if req.IsPublic != nil {
		conditions = append(conditions, fmt.Sprintf("f.is_public = $%d", argIndex))
		args = append(args, *req.IsPublic)
		argIndex++
	}

	// Size filters with table alias
	if req.MinSizeBytes != nil {
		conditions = append(conditions, fmt.Sprintf("f.size_bytes >= $%d", argIndex))
		args = append(args, *req.MinSizeBytes)
		argIndex++
	}
	if req.MaxSizeBytes != nil {
		conditions = append(conditions, fmt.Sprintf("f.size_bytes <= $%d", argIndex))
		args = append(args, *req.MaxSizeBytes)
		argIndex++
	}

	// Date range filters with table alias
	if req.CreatedAfter != nil {
		conditions = append(conditions, fmt.Sprintf("f.created_at > $%d", argIndex))
		args = append(args, *req.CreatedAfter)
		argIndex++
	}
	if req.CreatedBefore != nil {
		conditions = append(conditions, fmt.Sprintf("f.created_at < $%d", argIndex))
		args = append(args, *req.CreatedBefore)
		argIndex++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	return whereClause, args
}

// buildHardcodedWhereClause creates WHERE clause with hardcoded values to avoid prepared statement issues
func (s *FileService) buildHardcodedWhereClause(req FileListRequest) string {
	var conditions []string

	// Always exclude trashed files
	conditions = append(conditions, "f.deleted_at IS NULL")

	// Owner filter with hardcoded UUID
	if req.OwnerID != nil {
		conditions = append(conditions, fmt.Sprintf("f.owner_id = '%s'", req.OwnerID.String()))
	}

	// Folder filter with hardcoded UUID
	if req.FolderID != nil {
		conditions = append(conditions, fmt.Sprintf("f.folder_id = '%s'", req.FolderID.String()))
	}

	// Search in filename with escaped string
	if req.Search != "" {
		// Escape single quotes for SQL safety
		escapedSearch := strings.ReplaceAll(req.Search, "'", "''")
		conditions = append(conditions, fmt.Sprintf("f.original_filename ILIKE '%%%s%%'", escapedSearch))
	}

	// MIME type filter - for simplicity, just handle single type
	if len(req.MimeTypes) > 0 {
		// Escape single quotes and take first mime type
		escapedMime := strings.ReplaceAll(req.MimeTypes[0], "'", "''")
		conditions = append(conditions, fmt.Sprintf("f.mime_type = '%s'", escapedMime))
	}

	// Public status filter
	if req.IsPublic != nil {
		conditions = append(conditions, fmt.Sprintf("f.is_public = %t", *req.IsPublic))
	}

	// Size filters
	if req.MinSizeBytes != nil {
		conditions = append(conditions, fmt.Sprintf("f.size_bytes >= %d", *req.MinSizeBytes))
	}
	if req.MaxSizeBytes != nil {
		conditions = append(conditions, fmt.Sprintf("f.size_bytes <= %d", *req.MaxSizeBytes))
	}

	// Date range filters - using RFC3339 format
	if req.CreatedAfter != nil {
		conditions = append(conditions, fmt.Sprintf("f.created_at > '%s'", req.CreatedAfter.Format(time.RFC3339)))
	}
	if req.CreatedBefore != nil {
		conditions = append(conditions, fmt.Sprintf("f.created_at < '%s'", req.CreatedBefore.Format(time.RFC3339)))
	}

	// Tags filter - check for overlap with PostgreSQL array operator
	if len(req.Tags) > 0 {
		// Build array literal for PostgreSQL using ARRAY syntax
		tagLiterals := make([]string, len(req.Tags))
		for i, tag := range req.Tags {
			// Escape single quotes in tags for SQL safety
			escapedTag := strings.ReplaceAll(tag, "'", "''")
			tagLiterals[i] = fmt.Sprintf("'%s'", escapedTag)
		}
		tagsArray := "ARRAY[" + strings.Join(tagLiterals, ",") + "]"
		conditions = append(conditions, fmt.Sprintf("f.tags && %s", tagsArray))
	}

	// Build final WHERE clause
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	return whereClause
}

// buildOrderByClause constructs ORDER BY clause with validation
func (s *FileService) buildOrderByClause(sortBy, sortOrder string) string {
	// Validate sortBy
	validSortFields := map[string]bool{
		"created_at":        true,
		"updated_at":        true,
		"size_bytes":        true,
		"original_filename": true,
		"download_count":    true,
	}
	if !validSortFields[sortBy] {
		sortBy = "created_at" // Default fallback
	}

	// Validate sortOrder
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc" // Default fallback
	}

	return fmt.Sprintf("ORDER BY %s %s", sortBy, strings.ToUpper(sortOrder))
}

// CheckFileAccess verifies if a user can access a file (ownership or public)
func (s *FileService) CheckFileAccess(fileID uuid.UUID, userID *uuid.UUID) (bool, error) {
	if fileID == uuid.Nil {
		return false, ErrInvalidFileID
	}

	query := `
		SELECT owner_id, is_public 
		FROM files 
		WHERE id = $1 AND deleted_at IS NULL`

	var ownerID uuid.UUID
	var isPublic bool
	err := s.db.QueryRow(query, fileID).Scan(&ownerID, &isPublic)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, ErrFileNotFound
		}
		return false, fmt.Errorf("failed to check file access: %w", err)
	}

	// Check if public or user owns it
	if isPublic {
		return true, nil
	}

	if userID != nil && *userID == ownerID {
		return true, nil
	}

	return false, nil
}

// GetFileForDownload retrieves file for download with access checks
func (s *FileService) GetFileForDownload(fileID uuid.UUID, userID *uuid.UUID) (*models.File, error) {
	file, err := s.GetFileByID(fileID)
	if err != nil {
		return nil, err
	}

	// Check access permissions
	if !file.IsPublic {
		// File is private, check if user owns it
		if userID == nil || *userID != file.OwnerID {
			return nil, errors.New("access denied: file is private")
		}
	}

	return file, nil
}

// AddTagsToFile adds tags to a file with ownership check
func (s *FileService) AddTagsToFile(fileID, userID uuid.UUID, tags []string) (*models.File, error) {
	// Get file and check ownership
	file, err := s.GetFileByID(fileID)
	if err != nil {
		return nil, err
	}

	if !file.IsOwnedBy(userID) {
		return nil, ErrUnauthorizedAccess
	}

	// Add tags using PostgreSQL array concatenation with deduplication
	query := `
		UPDATE files 
		SET tags = array(SELECT DISTINCT unnest(tags || $1)), updated_at = $2
		WHERE id = $3 AND owner_id = $4 AND deleted_at IS NULL
		RETURNING id, owner_id, blob_hash, original_filename, mime_type, size_bytes,
		         is_public, download_count, tags, folder_id, created_at, updated_at, deleted_at`

	var updatedFile models.File
	err = s.db.QueryRow(query, pq.Array(tags), time.Now().UTC(), fileID, userID).Scan(
		&updatedFile.ID, &updatedFile.OwnerID, &updatedFile.BlobHash, &updatedFile.OriginalFilename, 
		&updatedFile.MimeType, &updatedFile.SizeBytes, &updatedFile.IsPublic, &updatedFile.DownloadCount, 
		pq.Array(&updatedFile.Tags), &updatedFile.FolderID, &updatedFile.CreatedAt, &updatedFile.UpdatedAt, &updatedFile.DeletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrFileNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to add tags: %w", err)
	}

	return &updatedFile, nil
}

// RemoveTagsFromFile removes tags from a file with ownership check
func (s *FileService) RemoveTagsFromFile(fileID, userID uuid.UUID, tags []string) (*models.File, error) {
	// Get file and check ownership
	file, err := s.GetFileByID(fileID)
	if err != nil {
		return nil, err
	}

	if !file.IsOwnedBy(userID) {
		return nil, ErrUnauthorizedAccess
	}

	// Remove tags using PostgreSQL array subtraction
	query := `
		UPDATE files 
		SET tags = array(SELECT unnest(tags) EXCEPT SELECT unnest($1::text[])), updated_at = $2
		WHERE id = $3 AND owner_id = $4 AND deleted_at IS NULL
		RETURNING id, owner_id, blob_hash, original_filename, mime_type, size_bytes,
		         is_public, download_count, tags, folder_id, created_at, updated_at, deleted_at`

	var updatedFile models.File
	err = s.db.QueryRow(query, pq.Array(tags), time.Now().UTC(), fileID, userID).Scan(
		&updatedFile.ID, &updatedFile.OwnerID, &updatedFile.BlobHash, &updatedFile.OriginalFilename, 
		&updatedFile.MimeType, &updatedFile.SizeBytes, &updatedFile.IsPublic, &updatedFile.DownloadCount, 
		pq.Array(&updatedFile.Tags), &updatedFile.FolderID, &updatedFile.CreatedAt, &updatedFile.UpdatedAt, &updatedFile.DeletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrFileNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to remove tags: %w", err)
	}

	return &updatedFile, nil
}

// CreateShareLink creates a new share link for a file
func (s *FileService) CreateShareLink(fileID uuid.UUID, userID uuid.UUID) (*models.ShareLink, error) {
	// Check if user owns the file
	file, err := s.GetFileByID(fileID)
	if err != nil {
		return nil, err
	}

	if !file.IsOwnedBy(userID) {
		return nil, ErrUnauthorizedAccess
	}

	// Check if a sharelink already exists for this file
	existingShareLink, err := s.GetShareLinkByFileID(fileID)
	if err == nil && existingShareLink != nil {
		// Sharelink already exists, return it
		return existingShareLink, nil
	}

	// Create new sharelink
	shareLink, err := models.NewShareLink(fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to create share link: %w", err)
	}

	// Insert into database
	query := `
		INSERT INTO sharelinks (id, file_id, token, expires_at, is_active, download_count, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	
	_, err = s.db.Exec(query, shareLink.ID, shareLink.FileID, shareLink.Token, shareLink.ExpiresAt, shareLink.IsActive, shareLink.DownloadCount, shareLink.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create share link in database: %w", err)
	}

	return shareLink, nil
}

// GetShareLinkByFileID retrieves a share link for a file
func (s *FileService) GetShareLinkByFileID(fileID uuid.UUID) (*models.ShareLink, error) {
	log.Printf("[FILE-SERVICE] GetShareLinkByFileID called - fileID: %s", fileID)
	
	// Use hardcoded query to avoid prepared statement issues
	query := fmt.Sprintf(`
		SELECT sharelink_file.id, sharelink_file.file_id, sharelink_file.token, sharelink_file.expires_at, sharelink_file.is_active, sharelink_file.download_count, sharelink_file.created_at
		FROM sharelinks sharelink_file
		WHERE sharelink_file.file_id = '%s'
	`, fileID.String())
	
	log.Printf("[FILE-SERVICE] GetShareLinkByFileID query: %s", query)

	var shareLink models.ShareLink
	err := s.db.QueryRow(query).Scan(
		&shareLink.ID,
		&shareLink.FileID,
		&shareLink.Token,
		&shareLink.ExpiresAt,
		&shareLink.IsActive,
		&shareLink.DownloadCount,
		&shareLink.CreatedAt,
	)

	if err == sql.ErrNoRows {
		log.Printf("[FILE-SERVICE] GetShareLinkByFileID: no share link found for file %s", fileID)
		return nil, ErrShareLinkNotFound
	}
	if err != nil {
		log.Printf("[FILE-SERVICE] GetShareLinkByFileID query failed: %v", err)
		return nil, fmt.Errorf("failed to get share link: %w", err)
	}

	log.Printf("[FILE-SERVICE] GetShareLinkByFileID success - token: %s", shareLink.Token)
	return &shareLink, nil
}

// EnableShareLink enables a share link for a file
func (s *FileService) EnableShareLink(fileID uuid.UUID, userID uuid.UUID) (*models.ShareLink, error) {
	// Check ownership
	file, err := s.GetFileByID(fileID)
	if err != nil {
		return nil, err
	}

	if !file.IsOwnedBy(userID) {
		return nil, ErrUnauthorizedAccess
	}

	// Get or create sharelink
	shareLink, err := s.GetShareLinkByFileID(fileID)
	if err == ErrShareLinkNotFound {
		// Create new sharelink if it doesn't exist
		return s.CreateShareLink(fileID, userID)
	}
	if err != nil {
		return nil, err
	}

	// Enable the sharelink
	query := `
		UPDATE sharelinks 
		SET is_active = true
		WHERE file_id = $1
	`
	
	_, err = s.db.Exec(query, fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to enable share link: %w", err)
	}

	shareLink.IsActive = true

	return shareLink, nil
}

// DeleteShareLink permanently deletes a share link for a file
func (s *FileService) DeleteShareLink(fileID uuid.UUID, userID uuid.UUID) error {
	// Check ownership
	file, err := s.GetFileByID(fileID)
	if err != nil {
		return err
	}

	if !file.IsOwnedBy(userID) {
		return ErrUnauthorizedAccess
	}

	// Delete the sharelink permanently (not just disable)
	query := `
		DELETE FROM sharelinks 
		WHERE file_id = $1
	`
	
	result, err := s.db.Exec(query, fileID)
	if err != nil {
		return fmt.Errorf("failed to delete share link: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrShareLinkNotFound
	}

	return nil
}

// GetFileByShareToken retrieves a file by its share link token
func (s *FileService) GetFileByShareToken(token string) (*models.File, error) {
	// First validate token format
	if !models.IsValidShareToken(token) {
		return nil, ErrInvalidShareToken
	}

	// Query to get file and sharelink info together
	query := `
		SELECT f.id, f.owner_id, f.blob_hash, f.original_filename, f.mime_type, 
		       f.size_bytes, f.is_public, f.download_count, f.tags, f.folder_id, f.created_at, f.updated_at, f.deleted_at,
		       sl.expires_at, sl.is_active
		FROM files f
		JOIN sharelinks sl ON f.id = sl.file_id
		WHERE sl.token = $1 AND f.deleted_at IS NULL`

	var file models.File
	var expiresAt *time.Time
	var isActive bool
	
	err := s.db.QueryRow(query, token).Scan(
		&file.ID,
		&file.OwnerID,
		&file.BlobHash,
		&file.OriginalFilename,
		&file.MimeType,
		&file.SizeBytes,
		&file.IsPublic,
		&file.DownloadCount,
		pq.Array(&file.Tags),
		&file.FolderID,
		&file.CreatedAt,
		&file.UpdatedAt,
		&file.DeletedAt,
		&expiresAt,
		&isActive,
	)

	if err == sql.ErrNoRows {
		return nil, ErrShareLinkNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get file by share token: %w", err)
	}

	// Check if sharelink is active
	if !isActive {
		return nil, ErrShareLinkInactive
	}

	// Check if sharelink is expired
	if expiresAt != nil && time.Now().UTC().After(*expiresAt) {
		return nil, ErrShareLinkExpired
	}

	return &file, nil
}

// GetPublicFilesByOwnerID retrieves all public files for a specific owner (no authentication required)
func (s *FileService) GetPublicFilesByOwnerID(ownerID uuid.UUID, page, pageSize int) ([]*models.File, int, error) {
	if ownerID == uuid.Nil {
		return nil, 0, fmt.Errorf("owner ID cannot be nil")
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	// Get total count
	countQuery := `SELECT COUNT(*) FROM files WHERE owner_id = $1 AND is_public = true AND deleted_at IS NULL`
	var total int
	err := s.db.QueryRow(countQuery, ownerID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count public files: %w", err)
	}

	// Get paginated files
	query := `
		SELECT id, owner_id, blob_hash, original_filename, mime_type, size_bytes,
		       is_public, download_count, tags, folder_id, created_at, updated_at, deleted_at
		FROM files 
		WHERE owner_id = $1 AND is_public = true AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.db.Query(query, ownerID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get public files by owner: %w", err)
	}
	defer rows.Close()

	var files []*models.File
	for rows.Next() {
		var file models.File
		err := rows.Scan(
			&file.ID, &file.OwnerID, &file.BlobHash, &file.OriginalFilename, &file.MimeType,
			&file.SizeBytes, &file.IsPublic, &file.DownloadCount, pq.Array(&file.Tags), &file.FolderID,
			&file.CreatedAt, &file.UpdatedAt, &file.DeletedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan file row: %w", err)
		}
		files = append(files, &file)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating file rows: %w", err)
	}

	return files, total, nil
}

// GetPublicFileByID retrieves a file by ID only if it's public (no authentication required)
func (s *FileService) GetPublicFileByID(fileID uuid.UUID) (*models.File, error) {
	if fileID == uuid.Nil {
		return nil, ErrInvalidFileID
	}

	query := `
		SELECT f.id, f.owner_id, f.blob_hash, f.original_filename, f.mime_type, 
		       f.size_bytes, f.is_public, f.download_count, f.tags, f.folder_id, 
		       f.created_at, f.updated_at, f.deleted_at
		FROM files f
		WHERE f.id = $1 AND f.is_public = true AND f.deleted_at IS NULL`

	var file models.File
	err := s.db.QueryRow(query, fileID).Scan(
		&file.ID, &file.OwnerID, &file.BlobHash, &file.OriginalFilename, &file.MimeType,
		&file.SizeBytes, &file.IsPublic, &file.DownloadCount, pq.Array(&file.Tags), &file.FolderID,
		&file.CreatedAt, &file.UpdatedAt, &file.DeletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("failed to get public file by ID: %w", err)
	}

	return &file, nil
}

// DeleteFileAsAdmin deletes a file from the database as an admin (no ownership check)
func (s *FileService) DeleteFileAsAdmin(fileID uuid.UUID) error {
	if fileID == uuid.Nil {
		return ErrInvalidFileID
	}

	// Get file info for blob cleanup
	file, err := s.GetFileByID(fileID)
	if err != nil {
		return err
	}

	// Start a transaction for atomic deletion
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete the file record (admin can delete any file)
	query := `DELETE FROM files WHERE id = $1`

	result, err := tx.Exec(query, fileID)
	if err != nil {
		return fmt.Errorf("failed to delete file as admin: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrFileNotFound
	}

	// Decrement blob reference count
	blobQuery := `
		UPDATE blobs 
		SET ref_count = ref_count - 1
		WHERE hash = $1 AND ref_count > 0
	`

	_, err = tx.Exec(blobQuery, file.BlobHash)
	if err != nil {
		return fmt.Errorf("failed to decrement blob reference count: %w", err)
	}

	// Check if blob should be deleted (ref_count = 0)
	var refCount int
	checkQuery := `SELECT ref_count FROM blobs WHERE hash = $1`
	err = tx.QueryRow(checkQuery, file.BlobHash).Scan(&refCount)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check blob reference count: %w", err)
	}

	// Commit the transaction first
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// If blob has no more references, delete it from storage
	if refCount == 0 {
		err = s.storageService.DeleteBlob(file.BlobHash)
		if err != nil {
			// Log the error but don't fail the entire operation
			// The file record is already deleted from the database
			fmt.Printf("Warning: failed to delete blob from storage: %v\n", err)
		}
	}

	return nil
}

// CreateFolderShareLink creates a new share link for a folder
func (s *FileService) CreateFolderShareLink(folderID uuid.UUID, userID uuid.UUID) (*models.ShareLink, error) {
	// Check if user owns the folder
	var folderOwnerID uuid.UUID
	err := s.db.QueryRow(
		"SELECT owner_id FROM folders WHERE id = $1",
		folderID,
	).Scan(&folderOwnerID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("folder not found")
		}
		return nil, fmt.Errorf("failed to validate folder: %w", err)
	}

	if folderOwnerID != userID {
		return nil, ErrUnauthorizedAccess
	}

	// Check if a sharelink already exists for this folder
	existingShareLink, err := s.GetShareLinkByFolderID(folderID)
	if err == nil && existingShareLink != nil {
		// Sharelink already exists, return it
		return existingShareLink, nil
	}

	// Create new folder sharelink
	shareLink, err := models.NewFolderShareLink(folderID)
	if err != nil {
		return nil, fmt.Errorf("failed to create folder share link: %w", err)
	}

	// Insert into database
	query := `
		INSERT INTO sharelinks (id, folder_id, token, expires_at, is_active, download_count, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	
	_, err = s.db.Exec(query, shareLink.ID, shareLink.FolderID, shareLink.Token, shareLink.ExpiresAt, shareLink.IsActive, shareLink.DownloadCount, shareLink.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create folder share link in database: %w", err)
	}

	return shareLink, nil
}

// GetShareLinkByFolderID retrieves a share link for a folder
func (s *FileService) GetShareLinkByFolderID(folderID uuid.UUID) (*models.ShareLink, error) {
	query := `
		SELECT id, folder_id, token, expires_at, is_active, download_count, created_at
		FROM sharelinks
		WHERE folder_id = $1
	`

	var shareLink models.ShareLink
	err := s.db.QueryRow(query, folderID).Scan(
		&shareLink.ID,
		&shareLink.FolderID,
		&shareLink.Token,
		&shareLink.ExpiresAt,
		&shareLink.IsActive,
		&shareLink.DownloadCount,
		&shareLink.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrShareLinkNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get folder share link: %w", err)
	}

	return &shareLink, nil
}

// EnableFolderShareLink enables a share link for a folder
func (s *FileService) EnableFolderShareLink(folderID uuid.UUID, userID uuid.UUID) (*models.ShareLink, error) {
	// Check ownership
	var folderOwnerID uuid.UUID
	err := s.db.QueryRow(
		"SELECT owner_id FROM folders WHERE id = $1",
		folderID,
	).Scan(&folderOwnerID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("folder not found")
		}
		return nil, fmt.Errorf("failed to validate folder: %w", err)
	}

	if folderOwnerID != userID {
		return nil, ErrUnauthorizedAccess
	}

	// Get or create sharelink
	shareLink, err := s.GetShareLinkByFolderID(folderID)
	if err == ErrShareLinkNotFound {
		// Create new sharelink if it doesn't exist
		return s.CreateFolderShareLink(folderID, userID)
	}
	if err != nil {
		return nil, err
	}

	// Enable the sharelink
	query := `
		UPDATE sharelinks 
		SET is_active = true
		WHERE folder_id = $1
	`
	
	_, err = s.db.Exec(query, folderID)
	if err != nil {
		return nil, fmt.Errorf("failed to enable folder share link: %w", err)
	}

	shareLink.IsActive = true

	return shareLink, nil
}

// DisableFolderShareLink disables a share link for a folder
func (s *FileService) DisableFolderShareLink(folderID uuid.UUID, userID uuid.UUID) error {
	// Check ownership
	var folderOwnerID uuid.UUID
	err := s.db.QueryRow(
		"SELECT owner_id FROM folders WHERE id = $1",
		folderID,
	).Scan(&folderOwnerID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("folder not found")
		}
		return fmt.Errorf("failed to validate folder: %w", err)
	}

	if folderOwnerID != userID {
		return ErrUnauthorizedAccess
	}

	// Disable the sharelink
	query := `
		UPDATE sharelinks 
		SET is_active = false
		WHERE folder_id = $1
	`
	
	result, err := s.db.Exec(query, folderID)
	if err != nil {
		return fmt.Errorf("failed to disable folder share link: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrShareLinkNotFound
	}

	return nil
}

// GetFolderByShareToken retrieves a folder by its share link token
func (s *FileService) GetFolderByShareToken(token string) (*models.Folder, error) {
	// First validate token format
	if !models.IsValidShareToken(token) {
		return nil, ErrInvalidShareToken
	}

	// Query to get folder and sharelink info together
	query := `
		SELECT f.id, f.owner_id, f.name, f.parent_id, f.created_at, f.updated_at, f.deleted_at,
		       sl.expires_at, sl.is_active
		FROM folders f
		JOIN sharelinks sl ON f.id = sl.folder_id
		WHERE sl.token = $1 AND f.deleted_at IS NULL`

	var folder models.Folder
	var expiresAt *time.Time
	var isActive bool
	
	err := s.db.QueryRow(query, token).Scan(
		&folder.ID,
		&folder.OwnerID,
		&folder.Name,
		&folder.ParentID,
		&folder.CreatedAt,
		&folder.UpdatedAt,
		&folder.DeletedAt,
		&expiresAt,
		&isActive,
	)

	if err == sql.ErrNoRows {
		return nil, ErrShareLinkNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get folder by share token: %w", err)
	}

	// Check if sharelink is active
	if !isActive {
		return nil, ErrShareLinkInactive
	}

	// Check if sharelink is expired
	if expiresAt != nil && time.Now().UTC().After(*expiresAt) {
		return nil, ErrShareLinkExpired
	}

	return &folder, nil
}

// TrashFile soft-deletes a file by setting deleted_at
func (s *FileService) TrashFile(fileID, ownerID uuid.UUID) error {
	if fileID == uuid.Nil {
		return ErrInvalidFileID
	}

	query := `UPDATE files SET deleted_at = NOW() WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`

	result, err := s.db.Exec(query, fileID, ownerID)
	if err != nil {
		return fmt.Errorf("failed to trash file: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrFileNotFound
	}

	return nil
}

// RestoreFile restores a soft-deleted file
func (s *FileService) RestoreFile(fileID, ownerID uuid.UUID) (*models.File, error) {
	if fileID == uuid.Nil {
		return nil, ErrInvalidFileID
	}

	query := `
		UPDATE files SET deleted_at = NULL
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NOT NULL
		RETURNING id, owner_id, blob_hash, original_filename, mime_type, size_bytes,
		          is_public, download_count, tags, folder_id, created_at, updated_at, deleted_at
	`

	var file models.File
	err := s.db.QueryRow(query, fileID, ownerID).Scan(
		&file.ID, &file.OwnerID, &file.BlobHash, &file.OriginalFilename,
		&file.MimeType, &file.SizeBytes, &file.IsPublic, &file.DownloadCount,
		pq.Array(&file.Tags), &file.FolderID, &file.CreatedAt, &file.UpdatedAt, &file.DeletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("failed to restore file: %w", err)
	}

	return &file, nil
}

// PermanentDeleteFile permanently deletes a trashed file (with blob cleanup)
func (s *FileService) PermanentDeleteFile(fileID, ownerID uuid.UUID) error {
	if fileID == uuid.Nil {
		return ErrInvalidFileID
	}

	// Get the trashed file info for blob cleanup
	var blobHash string
	err := s.db.QueryRow(
		`SELECT blob_hash FROM files WHERE id = $1 AND owner_id = $2 AND deleted_at IS NOT NULL`,
		fileID, ownerID,
	).Scan(&blobHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrFileNotFound
		}
		return fmt.Errorf("failed to get trashed file: %w", err)
	}

	// Start transaction for atomic deletion
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete the file record
	result, err := tx.Exec(`DELETE FROM files WHERE id = $1 AND owner_id = $2 AND deleted_at IS NOT NULL`, fileID, ownerID)
	if err != nil {
		return fmt.Errorf("failed to permanently delete file: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrFileNotFound
	}

	// Decrement blob reference count
	_, err = tx.Exec(`UPDATE blobs SET ref_count = ref_count - 1 WHERE hash = $1 AND ref_count > 0`, blobHash)
	if err != nil {
		return fmt.Errorf("failed to decrement blob reference count: %w", err)
	}

	// Check if blob should be deleted
	var refCount int
	err = tx.QueryRow(`SELECT ref_count FROM blobs WHERE hash = $1`, blobHash).Scan(&refCount)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check blob reference count: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// If blob has no more references, delete from storage
	if refCount == 0 {
		err = s.storageService.DeleteBlob(blobHash)
		if err != nil {
			fmt.Printf("Warning: failed to delete blob from storage: %v\n", err)
		}
	}

	return nil
}

// GetTrashedFiles returns all trashed files for a user with pagination
func (s *FileService) GetTrashedFiles(ownerID uuid.UUID, page, pageSize int) ([]*models.File, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	// Get total count
	var total int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM files WHERE owner_id = $1 AND deleted_at IS NOT NULL`,
		ownerID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count trashed files: %w", err)
	}

	// Get paginated trashed files
	query := `
		SELECT id, owner_id, blob_hash, original_filename, mime_type, size_bytes,
		       is_public, download_count, tags, folder_id, created_at, updated_at, deleted_at
		FROM files
		WHERE owner_id = $1 AND deleted_at IS NOT NULL
		ORDER BY deleted_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.db.Query(query, ownerID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get trashed files: %w", err)
	}
	defer rows.Close()

	var files []*models.File
	for rows.Next() {
		var file models.File
		err := rows.Scan(
			&file.ID, &file.OwnerID, &file.BlobHash, &file.OriginalFilename,
			&file.MimeType, &file.SizeBytes, &file.IsPublic, &file.DownloadCount,
			pq.Array(&file.Tags), &file.FolderID, &file.CreatedAt, &file.UpdatedAt, &file.DeletedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan trashed file: %w", err)
		}
		files = append(files, &file)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating trashed files: %w", err)
	}

	return files, total, nil
}

// EmptyTrashFiles permanently deletes all trashed files for a user
func (s *FileService) EmptyTrashFiles(ownerID uuid.UUID) error {
	// Get all trashed file info for blob cleanup
	rows, err := s.db.Query(
		`SELECT id, blob_hash FROM files WHERE owner_id = $1 AND deleted_at IS NOT NULL`,
		ownerID,
	)
	if err != nil {
		return fmt.Errorf("failed to get trashed files: %w", err)
	}
	defer rows.Close()

	type fileBlob struct {
		id       uuid.UUID
		blobHash string
	}
	var fileBlobs []fileBlob
	for rows.Next() {
		var fb fileBlob
		if err := rows.Scan(&fb.id, &fb.blobHash); err != nil {
			return fmt.Errorf("failed to scan file blob: %w", err)
		}
		fileBlobs = append(fileBlobs, fb)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating file blobs: %w", err)
	}

	if len(fileBlobs) == 0 {
		return nil // Nothing to delete
	}

	// Start transaction
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete all trashed files
	_, err = tx.Exec(`DELETE FROM files WHERE owner_id = $1 AND deleted_at IS NOT NULL`, ownerID)
	if err != nil {
		return fmt.Errorf("failed to delete trashed files: %w", err)
	}

	// Decrement blob references and track for cleanup
	var blobsToDelete []string
	for _, fb := range fileBlobs {
		_, err = tx.Exec(`UPDATE blobs SET ref_count = ref_count - 1 WHERE hash = $1 AND ref_count > 0`, fb.blobHash)
		if err != nil {
			return fmt.Errorf("failed to decrement blob ref_count: %w", err)
		}

		var refCount int
		err = tx.QueryRow(`SELECT ref_count FROM blobs WHERE hash = $1`, fb.blobHash).Scan(&refCount)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("failed to check blob ref_count: %w", err)
		}
		if refCount == 0 {
			blobsToDelete = append(blobsToDelete, fb.blobHash)
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Clean up storage for unreferenced blobs
	for _, hash := range blobsToDelete {
		if err := s.storageService.DeleteBlob(hash); err != nil {
			fmt.Printf("Warning: failed to delete blob %s from storage: %v\n", hash, err)
		}
	}

	return nil
}

// GetTrashedFileByID retrieves a trashed file by ID
func (s *FileService) GetTrashedFileByID(fileID uuid.UUID) (*models.File, error) {
	if fileID == uuid.Nil {
		return nil, ErrInvalidFileID
	}

	var file models.File
	query := `
		SELECT id, owner_id, blob_hash, original_filename, mime_type, size_bytes,
		       is_public, download_count, tags, folder_id, created_at, updated_at, deleted_at
		FROM files 
		WHERE id = $1 AND deleted_at IS NOT NULL
	`

	err := s.db.QueryRow(query, fileID).Scan(
		&file.ID, &file.OwnerID, &file.BlobHash, &file.OriginalFilename,
		&file.MimeType, &file.SizeBytes, &file.IsPublic, &file.DownloadCount,
		pq.Array(&file.Tags), &file.FolderID, &file.CreatedAt, &file.UpdatedAt, &file.DeletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("failed to get trashed file by ID: %w", err)
	}

	return &file, nil
}

// MoveFile moves a file to a different folder (or root if folderID is nil)
func (s *FileService) MoveFile(fileID, ownerID uuid.UUID, folderID *uuid.UUID) error {
	if fileID == uuid.Nil {
		return ErrInvalidFileID
	}
	if ownerID == uuid.Nil {
		return errors.New("owner ID cannot be nil")
	}

	// Check if file exists and is owned by user
	file, err := s.GetFileByID(fileID)
	if err != nil {
		return err
	}

	if !file.IsOwnedBy(ownerID) {
		return ErrUnauthorizedAccess
	}

	// If moving to a folder, verify it exists and belongs to the same owner
	if folderID != nil {
		var folderOwnerID uuid.UUID
		err := s.db.QueryRow(
			"SELECT owner_id FROM folders WHERE id = $1",
			*folderID,
		).Scan(&folderOwnerID)
		if err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("target folder not found")
			}
			return fmt.Errorf("failed to validate target folder: %w", err)
		}
		
		if folderOwnerID != ownerID {
			return fmt.Errorf("target folder does not belong to the same owner")
		}
	}

	// Update file's folder_id
	query := `
		UPDATE files 
		SET folder_id = $1, updated_at = NOW()
		WHERE id = $2 AND owner_id = $3
	`

	result, err := s.db.Exec(query, folderID, fileID, ownerID)
	if err != nil {
		return fmt.Errorf("failed to move file: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrFileNotFound
	}

	return nil
}

