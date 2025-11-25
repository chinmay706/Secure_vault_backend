package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"securevault-backend/src/services"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// PublicHandlers handles public API requests that don't require authentication
type PublicHandlers struct {
	fileService    *services.FileService
	folderService  *services.FolderService
	storageService *services.StorageService
}

// NewPublicHandlers creates a new PublicHandlers instance
func NewPublicHandlers(fileService *services.FileService, folderService *services.FolderService, storageService *services.StorageService) *PublicHandlers {
	return &PublicHandlers{
		fileService:    fileService,
		folderService:  folderService,
		storageService: storageService,
	}
}

// PublicFileResponse represents the public file details response
type PublicFileResponse struct {
	ID               uuid.UUID  `json:"id"`
	OriginalFilename string     `json:"original_filename"`
	MimeType         string     `json:"mime_type"`
	SizeBytes        int64      `json:"size_bytes"`
	Tags             []string   `json:"tags"`
	DownloadURL      string     `json:"download_url"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	FolderID         *uuid.UUID `json:"folder_id,omitempty"`
}

// PublicFolderResponse represents the public folder details response
type PublicFolderResponse struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	ParentID  *uuid.UUID `json:"parent_id,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// PublicFolderTreeResponse represents a folder tree with files and subfolders
type PublicFolderTreeResponse struct {
	Folder     PublicFolderInfo          `json:"folder"`
	Files      []PublicFileInfo          `json:"files"`
	Subfolders []PublicFolderTreeResponse `json:"subfolders"`
}

// PublicFolderInfo represents basic folder information in a tree
type PublicFolderInfo struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	ParentID  *uuid.UUID `json:"parent_id,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// PublicFileInfo represents basic file information in a tree
type PublicFileInfo struct {
	ID               uuid.UUID `json:"id"`
	OriginalFilename string    `json:"original_filename"`
	MimeType         string    `json:"mime_type"`
	SizeBytes        int64     `json:"size_bytes"`
	Tags             []string  `json:"tags"`
	DownloadURL      string    `json:"download_url"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Helper function to generate download URL
func (h *PublicHandlers) generateDownloadURL(r *http.Request, fileID uuid.UUID) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/api/v1/public/files/%s/download", scheme, r.Host, fileID.String())
}

// PublicFilesListResponse represents a paginated list of public files
type PublicFilesListResponse struct {
	Files      []PublicFileInfo `json:"files"`
	Pagination PaginationMeta   `json:"pagination"`
}

// PaginationMeta represents pagination metadata
type PaginationMeta struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// HandlePublicFilesByOwner handles GET /public/files/owner/{owner_id} - get all public files by owner ID
// @Summary Get all public files by owner ID
// @Description Get paginated list of all public files for a specific owner (no authentication required)
// @Tags Public
// @Accept json
// @Produce json
// @Param owner_id path string true "Owner ID (UUID)"
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page (max 100)" default(20)
// @Success 200 {object} PublicFilesListResponse "List of public files"
// @Failure 400 {object} ErrorResponse "Invalid owner ID"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /public/files/owner/{owner_id} [get]
func (h *PublicHandlers) HandlePublicFilesByOwner(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ownerIDStr := vars["owner_id"]

	// Parse owner ID
	ownerID, err := uuid.Parse(ownerIDStr)
	if err != nil {
		h.writeErrorResponse(w, "INVALID_OWNER_ID", "Invalid owner ID format", http.StatusBadRequest)
		return
	}

	// Parse pagination params
	page := 1
	pageSize := 20

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if pageSizeStr := r.URL.Query().Get("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	// Get public files by owner
	files, total, err := h.fileService.GetPublicFilesByOwnerID(ownerID, page, pageSize)
	if err != nil {
		h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to retrieve public files", http.StatusInternalServerError)
		return
	}

	// Convert to response format
	publicFiles := make([]PublicFileInfo, len(files))
	for i, file := range files {
		publicFiles[i] = PublicFileInfo{
			ID:               file.ID,
			OriginalFilename: file.OriginalFilename,
			MimeType:         file.MimeType,
			SizeBytes:        file.SizeBytes,
			Tags:             file.Tags,
			DownloadURL:      h.generateDownloadURL(r, file.ID),
			CreatedAt:        file.CreatedAt,
			UpdatedAt:        file.UpdatedAt,
		}
	}

	totalPages := (total + pageSize - 1) / pageSize

	response := PublicFilesListResponse{
		Files: publicFiles,
		Pagination: PaginationMeta{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// HandlePublicFileByID handles GET /public/files/{id} - get public file details by ID
// @Summary Get public file details by ID
// @Description Get file details for public files (no authentication required)
// @Tags Public
// @Accept json
// @Produce json
// @Param id path string true "File ID (UUID)"
// @Success 200 {object} PublicFileResponse "File details"
// @Failure 400 {object} ErrorResponse "Invalid file ID"
// @Failure 403 {object} ErrorResponse "File is private"
// @Failure 404 {object} ErrorResponse "File not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /public/files/{id} [get]
func (h *PublicHandlers) HandlePublicFileByID(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fileID := vars["id"]

	// Parse file ID
	id, err := uuid.Parse(fileID)
	if err != nil {
		h.writeErrorResponse(w, "INVALID_FILE_ID", "Invalid file ID format", http.StatusBadRequest)
		return
	}

	// Get public file
	file, err := h.fileService.GetPublicFileByID(id)
	if err != nil {
		if err == services.ErrFileNotFound {
			h.writeErrorResponse(w, "FILE_NOT_FOUND", "File not found or not public", http.StatusNotFound)
		} else {
			h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to retrieve file", http.StatusInternalServerError)
		}
		return
	}

	// Convert to public response format
	response := PublicFileResponse{
		ID:               file.ID,
		OriginalFilename: file.OriginalFilename,
		MimeType:         file.MimeType,
		SizeBytes:        file.SizeBytes,
		Tags:             file.Tags,
		DownloadURL:      h.generateDownloadURL(r, file.ID),
		CreatedAt:        file.CreatedAt,
		UpdatedAt:        file.UpdatedAt,
		FolderID:         file.FolderID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// HandlePublicFileByShareToken handles GET /public/files/share/{token} - get public file details by share token
// @Summary Get public file details by share token
// @Description Get file details using share token (no authentication required)
// @Tags Public
// @Accept json
// @Produce json
// @Param token path string true "File share token"
// @Success 200 {object} PublicFileResponse "File details"
// @Failure 400 {object} ErrorResponse "Invalid token"
// @Failure 403 {object} ErrorResponse "File is private"
// @Failure 404 {object} ErrorResponse "File not found"
// @Failure 410 {object} ErrorResponse "Share link expired"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /public/files/share/{token} [get]
func (h *PublicHandlers) HandlePublicFileByShareToken(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	token := vars["token"]

	if token == "" {
		h.writeErrorResponse(w, "INVALID_TOKEN", "Share token is required", http.StatusBadRequest)
		return
	}

	// Get file by share token
	file, err := h.fileService.GetFileByShareToken(token)
	if err != nil {
		if err == services.ErrShareLinkNotFound {
			h.writeErrorResponse(w, "NOT_FOUND", "Share link not found", http.StatusNotFound)
		} else if err == services.ErrShareLinkExpired {
			h.writeErrorResponse(w, "EXPIRED", "Share link has expired", http.StatusGone)
		} else if err == services.ErrShareLinkInactive {
			h.writeErrorResponse(w, "INACTIVE", "Share link has been deactivated", http.StatusForbidden)
		} else if err == services.ErrInvalidShareToken {
			h.writeErrorResponse(w, "INVALID_TOKEN", "Invalid share token format", http.StatusBadRequest)
		} else {
			h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to retrieve file", http.StatusInternalServerError)
		}
		return
	}

	// Double-check that file is public
	if !file.IsPublic {
		h.writeErrorResponse(w, "PRIVATE_FILE", "File is not publicly accessible", http.StatusForbidden)
		return
	}

	// Convert to public response format with download URL using token
	response := PublicFileResponse{
		ID:               file.ID,
		OriginalFilename: file.OriginalFilename,
		MimeType:         file.MimeType,
		SizeBytes:        file.SizeBytes,
		Tags:             file.Tags,
		DownloadURL:      h.generateTokenDownloadURL(r, token),
		CreatedAt:        file.CreatedAt,
		UpdatedAt:        file.UpdatedAt,
		FolderID:         file.FolderID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// Helper function to generate download URL using token
func (h *PublicHandlers) generateTokenDownloadURL(r *http.Request, token string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/api/v1/public/files/share/%s/download", scheme, r.Host, token)
}

// HandlePublicFolderByShareToken handles GET /public/folders/share/{token} - get public folder tree by share token
// @Summary Get public folder tree by share token
// @Description Get complete folder tree structure with all subfolders and files using share token (no authentication required)
// @Tags Public
// @Accept json
// @Produce json
// @Param token path string true "Folder share token"
// @Success 200 {object} PublicFolderTreeResponse "Folder tree with subfolders and files"
// @Failure 400 {object} ErrorResponse "Invalid token"
// @Failure 404 {object} ErrorResponse "Folder not found"
// @Failure 410 {object} ErrorResponse "Share link expired"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /public/folders/share/{token} [get]
func (h *PublicHandlers) HandlePublicFolderByShareToken(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	token := vars["token"]

	if token == "" {
		h.writeErrorResponse(w, "INVALID_TOKEN", "Share token is required", http.StatusBadRequest)
		return
	}

	// Get folder tree by share token
	folderTree, err := h.folderService.GetFolderTreeByShareToken(token)
	if err != nil {
		if err == services.ErrShareLinkNotFound {
			h.writeErrorResponse(w, "NOT_FOUND", "Share link not found", http.StatusNotFound)
		} else if err == services.ErrShareLinkExpired {
			h.writeErrorResponse(w, "EXPIRED", "Share link has expired", http.StatusGone)
		} else if err == services.ErrShareLinkInactive {
			h.writeErrorResponse(w, "INACTIVE", "Share link has been deactivated", http.StatusForbidden)
		} else if err == services.ErrInvalidShareToken {
			h.writeErrorResponse(w, "INVALID_TOKEN", "Invalid share token format", http.StatusBadRequest)
		} else {
			h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to retrieve folder", http.StatusInternalServerError)
		}
		return
	}

	// Convert tree structure to public response format
	response := h.convertToPublicFolderTree(folderTree, r)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// convertToPublicFolderTree converts service tree structure to public API response format
func (h *PublicHandlers) convertToPublicFolderTree(tree *services.FolderTreeItem, r *http.Request) PublicFolderTreeResponse {
	response := PublicFolderTreeResponse{
		Folder: PublicFolderInfo{
			ID:        tree.Folder.ID,
			Name:      tree.Folder.Name,
			ParentID:  tree.Folder.ParentID,
			CreatedAt: tree.Folder.CreatedAt,
			UpdatedAt: tree.Folder.UpdatedAt,
		},
		Files:      make([]PublicFileInfo, len(tree.Files)),
		Subfolders: make([]PublicFolderTreeResponse, len(tree.Subfolders)),
	}

	// Convert files
	for i, file := range tree.Files {
		response.Files[i] = PublicFileInfo{
			ID:               file.ID,
			OriginalFilename: file.OriginalFilename,
			MimeType:         file.MimeType,
			SizeBytes:        file.SizeBytes,
			Tags:             file.Tags,
			DownloadURL:      h.generateDownloadURL(r, file.ID),
			CreatedAt:        file.CreatedAt,
			UpdatedAt:        file.UpdatedAt,
		}
	}

	// Convert child folders recursively
	for i, child := range tree.Subfolders {
		response.Subfolders[i] = h.convertToPublicFolderTree(child, r)
	}

	return response
}

// HandlePublicFileDownload handles GET /public/files/{id}/download - download public file content
// @Summary Download public file
// @Description Download file content for public files (no authentication required)
// @Tags Public
// @Produce application/octet-stream
// @Param id path string true "File ID (UUID)"
// @Success 200 {file} file "File content with appropriate headers"
// @Failure 400 {object} ErrorResponse "Invalid file ID format"
// @Failure 403 {object} ErrorResponse "File not public"
// @Failure 404 {object} ErrorResponse "File not found"
// @Failure 500 {object} ErrorResponse "Download failed"
// @Router /public/files/{id}/download [get]
func (h *PublicHandlers) HandlePublicFileDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fileID := vars["id"]

	// Parse file ID
	id, err := uuid.Parse(fileID)
	if err != nil {
		h.writeErrorResponse(w, "INVALID_FILE_ID", "Invalid file ID format", http.StatusBadRequest)
		return
	}

	// Get public file (no user required)
	file, err := h.fileService.GetPublicFileByID(id)
	if err != nil {
		if err == services.ErrFileNotFound {
			h.writeErrorResponse(w, "FILE_NOT_FOUND", "File not found or not public", http.StatusNotFound)
		} else {
			h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to retrieve file", http.StatusInternalServerError)
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

	// Increment download count (optional for public files)
	_ = h.fileService.IncrementDownloadCount(id)

	// Set headers for file download
	w.Header().Set("Content-Type", file.MimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", file.OriginalFilename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", file.SizeBytes))

	// Stream file content
	w.WriteHeader(http.StatusOK)
	
	// Copy content to response
	_, err = h.copyContent(w, content)
	if err != nil {
		// Can't write error response after headers are sent
		return
	}
}

// HandlePublicFileDownloadByToken handles GET /public/files/share/{token}/download - download file by share token
// @Summary Download file by share token
// @Description Download file content using share token (no authentication required)
// @Tags Public
// @Produce application/octet-stream
// @Param token path string true "File share token"
// @Success 200 {file} file "File content with appropriate headers"
// @Failure 400 {object} ErrorResponse "Invalid token format"
// @Failure 403 {object} ErrorResponse "Share link expired or inactive"
// @Failure 404 {object} ErrorResponse "File not found"
// @Failure 500 {object} ErrorResponse "Download failed"
// @Router /public/files/share/{token}/download [get]
func (h *PublicHandlers) HandlePublicFileDownloadByToken(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	token := vars["token"]

	// Get file by share token
	file, err := h.fileService.GetFileByShareToken(token)
	if err != nil {
		if err == services.ErrShareLinkNotFound {
			h.writeErrorResponse(w, "NOT_FOUND", "Share link not found", http.StatusNotFound)
		} else if err == services.ErrShareLinkExpired {
			h.writeErrorResponse(w, "EXPIRED", "Share link has expired", http.StatusGone)
		} else if err == services.ErrShareLinkInactive {
			h.writeErrorResponse(w, "INACTIVE", "Share link has been deactivated", http.StatusForbidden)
		} else if err == services.ErrInvalidShareToken {
			h.writeErrorResponse(w, "INVALID_TOKEN", "Invalid share token format", http.StatusBadRequest)
		} else {
			h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to retrieve file", http.StatusInternalServerError)
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
	_ = h.fileService.IncrementDownloadCount(file.ID)

	// Set headers for file download
	w.Header().Set("Content-Type", file.MimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", file.OriginalFilename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", file.SizeBytes))

	// Stream file content
	w.WriteHeader(http.StatusOK)
	
	// Copy content to response
	_, err = h.copyContent(w, content)
	if err != nil {
		// Can't write error response after headers are sent
		return
	}
}

// copyContent copies content from source to destination with error handling
func (h *PublicHandlers) copyContent(dst http.ResponseWriter, src interface{}) (int64, error) {
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

// writeErrorResponse writes a standardized error response
func (h *PublicHandlers) writeErrorResponse(w http.ResponseWriter, code, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	errorResponse := ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	}
	
	json.NewEncoder(w).Encode(errorResponse)
}