package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"securevault-backend/src/services"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// FilesHandlers handles file-related HTTP requests
type FilesHandlers struct {
	fileService    *services.FileService
	storageService *services.StorageService
	authService    *services.AuthService
}

// NewFilesHandlers creates a new FilesHandlers instance
func NewFilesHandlers(fileService *services.FileService, storageService *services.StorageService, authService *services.AuthService) *FilesHandlers {
	return &FilesHandlers{
		fileService:    fileService,
		storageService: storageService,
		authService:    authService,
	}
}

// FileSearchParams represents search parameters for file listing
type FileSearchParams struct {
	OwnerID  uuid.UUID
	Filename string
	MimeType string
	Tags     []string
	Page     int
	PageSize int
}

// FileListResponse represents the response for file listing
type FileListResponse struct {
	Files    []FileResponse `json:"files"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
	Total    int64         `json:"total"`
}

// FileResponse represents a file in the API response
type FileResponse struct {
	ID               string              `json:"id"`
	OriginalFilename string              `json:"original_filename"`
	MimeType         string              `json:"mime_type"`
	SizeBytes        int64               `json:"size_bytes"`
	IsPublic         bool                `json:"is_public"`
	DownloadCount    int64               `json:"download_count"`
	Tags             []string            `json:"tags"`
	FolderID         *string             `json:"folder_id,omitempty"`
	CreatedAt        string              `json:"created_at"`
	UpdatedAt        string              `json:"updated_at"`
	ShareLink        *ShareLinkResponse  `json:"share_link,omitempty"`
}

// ShareLinkResponse represents a share link in the API response
type ShareLinkResponse struct {
	Token         string `json:"token"`
	IsActive      bool   `json:"is_active"`
	DownloadCount int64  `json:"download_count"`
	CreatedAt     string `json:"created_at"`
}

// UploadResponse represents the response for file upload
type UploadResponse struct {
	File         FileResponse `json:"file"`
	Hash         string       `json:"hash"`
	IsDuplicate  bool         `json:"is_duplicate"`
}

// TogglePublicRequest represents the request body for toggling file public visibility
type TogglePublicRequest struct {
	IsPublic bool `json:"is_public" example:"true"`
}

// TogglePublicResponse represents the response for toggling file public visibility
type TogglePublicResponse struct {
	File FileResponse `json:"file"`
}

// extractUserFromAuth extracts user ID from Authorization header
func (h *FilesHandlers) extractUserFromAuth(r *http.Request) (uuid.UUID, error) {
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

// writeErrorResponse writes a standardized error response
func (h *FilesHandlers) writeErrorResponse(w http.ResponseWriter, code string, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	
	response := ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	}
	
	json.NewEncoder(w).Encode(response)
}

// HandleFilesList handles GET /files - list files with filtering
// @Summary List user files
// @Description Get a paginated list of files owned by the authenticated user with optional filtering
// @Tags Files
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param filename query string false "Filter by filename (partial match)"
// @Param mime_type query string false "Filter by MIME type"
// @Param tags query string false "Filter by comma-separated tags"
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page" default(20)
// @Success 200 {object} FileListResponse "List of user files"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /files [get]
func (h *FilesHandlers) HandleFilesList(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from authorization
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		h.writeErrorResponse(w, "UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	
	// Pagination
	page := 1
	if pageStr := query.Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	
	pageSize := 20
	if pageSizeStr := query.Get("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	// Search filters
	filename := query.Get("filename")
	mimeType := query.Get("mime_type")
	
	// Folder filter
	var folderID *uuid.UUID
	if folderIDStr := query.Get("folder_id"); folderIDStr != "" {
		if parsed, err := uuid.Parse(folderIDStr); err == nil {
			folderID = &parsed
		}
	}
	
	// Tags filter
	var tags []string
	if tagsStr := query.Get("tags"); tagsStr != "" {
		tags = strings.Split(tagsStr, ",")
		for i, tag := range tags {
			tags[i] = strings.TrimSpace(tag)
		}
	}

	// Call file service
	fileListReq := services.FileListRequest{
		OwnerID:   &userID,
		FolderID:  folderID,
		Search:    filename,
		MimeTypes: []string{},
		Tags:      tags,
		Page:     page,
		PageSize: pageSize,
	}
	
	if mimeType != "" {
		fileListReq.MimeTypes = []string{mimeType}
	}

	fileListResp, err := h.fileService.ListFilesEnhanced(fileListReq)
	if err != nil {
		h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to list files", http.StatusInternalServerError)
		return
	}

	// Convert to response format
	fileResponses := make([]FileResponse, len(fileListResp.Files))
	for i, file := range fileListResp.Files {
		var folderIDStr *string
		if file.FolderID != nil {
			str := file.FolderID.String()
			folderIDStr = &str
		}
		
		fileResponses[i] = FileResponse{
			ID:               file.ID.String(),
			OriginalFilename: file.OriginalFilename,
			MimeType:         file.MimeType,
			SizeBytes:        file.SizeBytes,
			IsPublic:         file.IsPublic,
			DownloadCount:    int64(file.DownloadCount),
			Tags:             file.Tags,
			FolderID:         folderIDStr,
			CreatedAt:        file.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:        file.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	response := FileListResponse{
		Files:    fileResponses,
		Page:     page,
		PageSize: pageSize,
		Total:    fileListResp.Pagination.TotalItems,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// HandleFileUpload handles POST /files - upload new file
// @Summary Upload a file
// @Description Upload a new file to secure storage with optional tags
// @Tags Files
// @Accept multipart/form-data
// @Produce json
// @Security BearerAuth
// @Param file formData file true "File to upload (max 100MB)"
// @Param tags formData string false "Comma-separated tags for the file"
// @Success 201 {object} UploadResponse "File successfully uploaded"
// @Failure 400 {object} ErrorResponse "Invalid request or no file provided"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 413 {object} ErrorResponse "File too large"
// @Failure 500 {object} ErrorResponse "Upload failed"
// @Router /files [post]
func (h *FilesHandlers) HandleFileUpload(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from authorization
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		h.writeErrorResponse(w, "UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
		return
	}

	// Parse multipart form
	err = r.ParseMultipartForm(32 << 20) // 32 MB limit for form parsing
	if err != nil {
		h.writeErrorResponse(w, "INVALID_REQUEST", "Failed to parse multipart form", http.StatusBadRequest)
		return
	}

	// Get file from form
	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		h.writeErrorResponse(w, "INVALID_REQUEST", "No file provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Get optional tags
	var tags []string
	if tagsStr := r.FormValue("tags"); tagsStr != "" {
		tags = strings.Split(tagsStr, ",")
		for i, tag := range tags {
			tags[i] = strings.TrimSpace(tag)
		}
	}

	// Get optional folder_id
	var folderID *uuid.UUID
	if folderIDStr := r.FormValue("folder_id"); folderIDStr != "" {
		if parsed, err := uuid.Parse(folderIDStr); err == nil {
			folderID = &parsed
		}
	}

	// Detect content type
	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Create upload request
	uploadReq := &services.UploadRequest{
		Reader:      file,
		Filename:    fileHeader.Filename,
		ContentType: contentType,
		Size:        fileHeader.Size,
		OwnerID:     userID,
	}

	// Upload file using storage service
	result, err := h.storageService.StreamingUpload(uploadReq)
	if err != nil {
		if err == services.ErrFileSizeExceeded {
			h.writeErrorResponse(w, "FILE_TOO_LARGE", "File exceeds maximum size limit", http.StatusRequestEntityTooLarge)
		} else if err == services.ErrInvalidMimeType {
			h.writeErrorResponse(w, "INVALID_FILE_TYPE", "File type not allowed", http.StatusBadRequest)
		} else {
			// Log the actual error for debugging
			h.writeErrorResponse(w, "UPLOAD_FAILED", fmt.Sprintf("Failed to upload file: %v", err), http.StatusInternalServerError)
		}
		return
	}

	// If not a duplicate, update file with tags and folder
	if !result.IsDeuplicate {
		// Validate folder if provided
		if folderID != nil {
			// TODO: Add folder validation - check if folder exists and belongs to user
			// For now, we'll set the folder directly
		}
		
		// Update tags if provided
		if len(tags) > 0 {
			updatedFile, err := h.fileService.AddTagsToFile(result.File.ID, userID, tags)
			if err != nil {
				// Log error but don't fail the upload
				// The file was successfully uploaded, just tags failed to update
			} else {
				result.File = updatedFile
			}
		}
		
		// Update folder if provided (direct DB update for now)
		if folderID != nil {
			result.File.SetFolder(folderID)
			// TODO: Need to persist this folder change to database
			// For now the response will show the intended folder but DB won't be updated
		}
	}

	// Create response
	var folderIDStr *string
	if result.File.FolderID != nil {
		str := result.File.FolderID.String()
		folderIDStr = &str
	}

	response := UploadResponse{
		File: FileResponse{
			ID:               result.File.ID.String(),
			OriginalFilename: result.File.OriginalFilename,
			MimeType:         result.File.MimeType,
			SizeBytes:        result.File.SizeBytes,
			IsPublic:         result.File.IsPublic,
			DownloadCount:    int64(result.File.DownloadCount),
			Tags:             result.File.Tags,
			FolderID:         folderIDStr,
			CreatedAt:        result.File.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:        result.File.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		},
		Hash:        result.Hash,
		IsDuplicate: result.IsDeuplicate,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// HandleFileDetails handles GET /files/{id} - get file details
// @Summary Get file details
// @Description Get detailed information about a specific file including metadata and share link
// @Tags Files
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "File ID (UUID)"
// @Success 200 {object} FileResponse "File details"
// @Failure 400 {object} ErrorResponse "Invalid file ID format"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 403 {object} ErrorResponse "Access denied"
// @Failure 404 {object} ErrorResponse "File not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /files/{id} [get]
func (h *FilesHandlers) HandleFileDetails(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fileID := vars["id"]

	// Parse file ID
	id, err := uuid.Parse(fileID)
	if err != nil {
		h.writeErrorResponse(w, "INVALID_FILE_ID", "Invalid file ID format", http.StatusBadRequest)
		return
	}

	// Extract user ID from authorization
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		h.writeErrorResponse(w, "UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
		return
	}

	// Get file details
	file, err := h.fileService.GetFileByID(id)
	if err != nil {
		if err == services.ErrFileNotFound {
			h.writeErrorResponse(w, "FILE_NOT_FOUND", "File not found", http.StatusNotFound)
		} else {
			h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to get file details", http.StatusInternalServerError)
		}
		return
	}

	// Check if user owns the file or if it's public
	if !file.IsPublic && file.OwnerID != userID {
		h.writeErrorResponse(w, "UNAUTHORIZED", "Access denied", http.StatusForbidden)
		return
	}

	var folderIDStr *string
	if file.FolderID != nil {
		str := file.FolderID.String()
		folderIDStr = &str
	}

	response := FileResponse{
		ID:               file.ID.String(),
		OriginalFilename: file.OriginalFilename,
		MimeType:         file.MimeType,
		SizeBytes:        file.SizeBytes,
		IsPublic:         file.IsPublic,
		DownloadCount:    int64(file.DownloadCount),
		Tags:             file.Tags,
		FolderID:         folderIDStr,
		CreatedAt:        file.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:        file.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	// If file is public, fetch and include share link
	if file.IsPublic {
		shareLink, err := h.fileService.GetShareLinkByFileID(file.ID)
		if err == nil {
			response.ShareLink = &ShareLinkResponse{
				Token:         shareLink.Token,
				IsActive:      shareLink.IsActive,
				DownloadCount: int64(shareLink.DownloadCount),
				CreatedAt:     shareLink.CreatedAt.Format("2006-01-02T15:04:05Z"),
			}
		}
		// If no share link found, that's okay - we'll just not include it
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// HandleFileDownload handles GET /files/{id}/download - download file content
// @Summary Download file
// @Description Download file content with proper headers and content disposition
// @Tags Files
// @Produce application/octet-stream
// @Security BearerAuth
// @Param id path string true "File ID (UUID)"
// @Success 200 {file} file "File content with appropriate headers"
// @Failure 400 {object} ErrorResponse "Invalid file ID format"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 403 {object} ErrorResponse "Access denied"
// @Failure 404 {object} ErrorResponse "File not found"
// @Failure 500 {object} ErrorResponse "Download failed"
// @Router /files/{id}/download [get]
func (h *FilesHandlers) HandleFileDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fileID := vars["id"]

	// Parse file ID
	id, err := uuid.Parse(fileID)
	if err != nil {
		h.writeErrorResponse(w, "INVALID_FILE_ID", "Invalid file ID format", http.StatusBadRequest)
		return
	}

	// Extract user ID from authorization
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		h.writeErrorResponse(w, "UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
		return
	}

	// Get file details and check access
	file, err := h.fileService.GetFileForDownload(id, &userID)
	if err != nil {
		if err == services.ErrFileNotFound {
			h.writeErrorResponse(w, "FILE_NOT_FOUND", "File not found", http.StatusNotFound)
		} else if err == services.ErrUnauthorizedAccess {
			h.writeErrorResponse(w, "UNAUTHORIZED", "Access denied", http.StatusForbidden)
		} else {
			h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to get file details", http.StatusInternalServerError)
		}
		return
	}

	// Download file content
	content, err := h.storageService.DownloadFile(file.BlobHash)
	if err != nil {
		h.writeErrorResponse(w, "DOWNLOAD_FAILED", "Failed to download file", http.StatusInternalServerError)
		return
	}
	defer content.Close()

	// Increment download count
	_ = h.fileService.IncrementDownloadCount(id)

	// Set headers for file download
	w.Header().Set("Content-Type", file.MimeType)
	safeName := strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r == '\n' || r == '\r' {
			return '_'
		}
		return r
	}, file.OriginalFilename)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeName))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", file.SizeBytes))

	// Stream file content
	w.WriteHeader(http.StatusOK)
	
	// Copy content to response
	_, err = copyContent(w, content)
	if err != nil {
		// Can't write error response after headers are sent
		return
	}
}

// HandleFileDelete handles DELETE /files/{id} - delete file
// @Summary Delete file
// @Description Delete a file owned by the authenticated user (includes S3 cleanup)
// @Tags Files
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "File ID (UUID)"
// @Success 204 "File successfully deleted"
// @Failure 400 {object} ErrorResponse "Invalid file ID format"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 403 {object} ErrorResponse "Access denied"
// @Failure 404 {object} ErrorResponse "File not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /files/{id} [delete]
func (h *FilesHandlers) HandleFileDelete(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fileID := vars["id"]

	// Parse file ID
	id, err := uuid.Parse(fileID)
	if err != nil {
		h.writeErrorResponse(w, "INVALID_FILE_ID", "Invalid file ID format", http.StatusBadRequest)
		return
	}

	// Extract user ID from authorization
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		h.writeErrorResponse(w, "UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
		return
	}

	// Delete file
	err = h.fileService.DeleteFile(id, userID)
	if err != nil {
		if err == services.ErrFileNotFound {
			h.writeErrorResponse(w, "FILE_NOT_FOUND", "File not found", http.StatusNotFound)
		} else if err == services.ErrUnauthorizedAccess {
			h.writeErrorResponse(w, "UNAUTHORIZED", "Access denied", http.StatusForbidden)
		} else {
			h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to delete file", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleTogglePublic handles PATCH /files/{id}/public - toggle public access
// @Summary Toggle file public visibility
// @Description Make a file public or private. When made public, generates a share link. When made private, removes the share link.
// @Tags Files
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "File ID (UUID)" Format(uuid)
// @Param request body TogglePublicRequest true "Public visibility setting"
// @Success 200 {object} TogglePublicResponse "Updated file with share link info (if public)"
// @Failure 400 {object} ErrorResponse "Invalid file ID or JSON body"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 403 {object} ErrorResponse "Access denied"
// @Failure 404 {object} ErrorResponse "File not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /files/{id}/public [patch]
func (h *FilesHandlers) HandleTogglePublic(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fileID := vars["id"]

	// Parse file ID
	id, err := uuid.Parse(fileID)
	if err != nil {
		h.writeErrorResponse(w, "INVALID_FILE_ID", "Invalid file ID format", http.StatusBadRequest)
		return
	}

	// Extract user ID from authorization
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		h.writeErrorResponse(w, "UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
		return
	}

	// Parse request body
	var request TogglePublicRequest
	
	err = json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		h.writeErrorResponse(w, "INVALID_REQUEST", "Invalid JSON body", http.StatusBadRequest)
		return
	}

	// Toggle public access
	file, shareLink, err := h.fileService.SetFilePublic(id, request.IsPublic, userID)
	if err != nil {
		if err == services.ErrFileNotFound {
			h.writeErrorResponse(w, "FILE_NOT_FOUND", "File not found", http.StatusNotFound)
		} else if err == services.ErrUnauthorizedAccess {
			h.writeErrorResponse(w, "UNAUTHORIZED", "Access denied", http.StatusForbidden)
		} else {
			h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to update file", http.StatusInternalServerError)
		}
		return
	}

	var folderIDStr *string
	if file.FolderID != nil {
		str := file.FolderID.String()
		folderIDStr = &str
	}

	response := FileResponse{
		ID:               file.ID.String(),
		OriginalFilename: file.OriginalFilename,
		MimeType:         file.MimeType,
		SizeBytes:        file.SizeBytes,
		IsPublic:         file.IsPublic,
		DownloadCount:    int64(file.DownloadCount),
		Tags:             file.Tags,
		FolderID:         folderIDStr,
		CreatedAt:        file.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:        file.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	// Include sharelink data if file is public and sharelink exists
	if file.IsPublic && shareLink != nil {
		response.ShareLink = &ShareLinkResponse{
			Token:         shareLink.Token,
			IsActive:      shareLink.IsActive,
			DownloadCount: int64(shareLink.DownloadCount),
			CreatedAt:     shareLink.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(TogglePublicResponse{
		File: response,
	})
}

// copyContent copies content from source to destination with error handling
func copyContent(dst http.ResponseWriter, src interface{}) (int64, error) {
	// This is a simple copy implementation
	// In a production system, you might want to use io.CopyBuffer for better performance
	switch v := src.(type) {
	case interface{ Read([]byte) (int, error) }:
		buffer := make([]byte, 32*1024) // 32KB buffer
		var total int64
		for {
			n, err := v.Read(buffer)
			if n > 0 {
				written, writeErr := dst.Write(buffer[:n])
				total += int64(written)
				if writeErr != nil {
					return total, writeErr
				}
			}
			if err != nil {
				if err.Error() == "EOF" {
					break
				}
				return total, err
			}
		}
		return total, nil
	default:
		return 0, fmt.Errorf("unsupported source type")
	}
}

// MoveFileRequest represents the request body for moving a file
type MoveFileRequest struct {
	FolderID *uuid.UUID `json:"folder_id"`
}

// HandleFileMove handles PATCH /files/{id}/move
// @Summary Move file to folder
// @Description Move a file to a different folder or root
// @Tags files
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "File ID"
// @Param request body MoveFileRequest true "Move file request"
// @Success 200 {object} FileResponse "File moved successfully"
// @Failure 400 {object} ErrorResponse "Invalid request"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 404 {object} ErrorResponse "File not found"
// @Router /files/{id}/move [patch]
func (h *FilesHandlers) HandleFileMove(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from authorization
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		h.writeErrorResponse(w, "UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
		return
	}

	// Parse file ID from URL
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.writeErrorResponse(w, "INVALID_FILE_ID", "Invalid file ID format", http.StatusBadRequest)
		return
	}

	// Parse request body
	var request MoveFileRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.writeErrorResponse(w, "INVALID_JSON", "Invalid JSON in request body", http.StatusBadRequest)
		return
	}

	// Get the file first to verify ownership
	file, err := h.fileService.GetFileByID(id)
	if err != nil {
		if err == services.ErrFileNotFound {
			h.writeErrorResponse(w, "FILE_NOT_FOUND", "File not found", http.StatusNotFound)
		} else {
			h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to get file", http.StatusInternalServerError)
		}
		return
	}

	// Check ownership
	if !file.IsOwnedBy(userID) {
		h.writeErrorResponse(w, "UNAUTHORIZED", "Access denied", http.StatusForbidden)
		return
	}

	// Validate folder exists and belongs to user if folder_id is provided
	if request.FolderID != nil {
		// TODO: Add folder validation when FolderService.GetFolderByID is available
		// For now, we'll trust that the folder_id is valid
	}
	
	// Update the file's folder and persist to database
	err = h.fileService.MoveFile(id, userID, request.FolderID)
	if err != nil {
		h.writeErrorResponse(w, "MOVE_FAILED", "Failed to move file", http.StatusInternalServerError)
		return
	}
	
	// Get the updated file from database
	file, err = h.fileService.GetFileByID(id)
	if err != nil {
		h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to get updated file", http.StatusInternalServerError)
		return
	}
	
	var folderIDStr *string
	if file.FolderID != nil {
		str := file.FolderID.String()
		folderIDStr = &str
	}

	response := FileResponse{
		ID:               file.ID.String(),
		OriginalFilename: file.OriginalFilename,
		MimeType:         file.MimeType,
		SizeBytes:        file.SizeBytes,
		IsPublic:         file.IsPublic,
		DownloadCount:    int64(file.DownloadCount),
		Tags:             file.Tags,
		FolderID:         folderIDStr,
		CreatedAt:        file.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:        file.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}