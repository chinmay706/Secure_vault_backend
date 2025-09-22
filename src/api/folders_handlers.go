package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"securevault-backend/src/models"
	"securevault-backend/src/services"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// FoldersHandlers handles folder-related HTTP requests
type FoldersHandlers struct {
	folderService *services.FolderService
	fileService   *services.FileService
	authService   *services.AuthService
}

// NewFoldersHandlers creates a new FoldersHandlers instance
func NewFoldersHandlers(folderService *services.FolderService, fileService *services.FileService, authService *services.AuthService) *FoldersHandlers {
	return &FoldersHandlers{
		folderService: folderService,
		fileService:   fileService,
		authService:   authService,
	}
}

// Response types for folder API
type FolderResponse struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	ParentID  *string `json:"parent_id,omitempty"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

type FolderChildrenResponse struct {
	Folders         []FolderResponse     `json:"folders"`
	Files           []FolderFileResponse `json:"files"`
	FilesPagination FolderPaginationInfo `json:"files_pagination"`
}

type FolderDetailsResponse struct {
	Folder      FolderResponse   `json:"folder"`
	Breadcrumbs []FolderResponse `json:"breadcrumbs"`
}

type FolderFileResponse struct {
	ID               string    `json:"id"`
	OriginalFilename string    `json:"original_filename"`
	MimeType         string    `json:"mime_type"`
	SizeBytes        int64     `json:"size_bytes"`
	IsPublic         bool      `json:"is_public"`
	DownloadCount    int       `json:"download_count"`
	Tags             []string  `json:"tags"`
	FolderID         *string   `json:"folder_id,omitempty"`
	CreatedAt        string    `json:"created_at"`
	UpdatedAt        string    `json:"updated_at"`
}

type CreateFolderRequest struct {
	Name     string     `json:"name"`
	ParentID *uuid.UUID `json:"parent_id,omitempty"`
}

type UpdateFolderRequest struct {
	Name     string     `json:"name"`
	ParentID *uuid.UUID `json:"parent_id,omitempty"`
}

type FolderPaginationInfo struct {
	Page         int   `json:"page"`
	PageSize     int   `json:"page_size"`
	TotalItems   int64 `json:"total_items"`
	TotalPages   int   `json:"total_pages"`
	HasNext      bool  `json:"has_next"`
	HasPrevious  bool  `json:"has_previous"`
	// Additional fields for GraphQL compatibility
	TotalFolders int  `json:"total_folders"`
	TotalFiles   int  `json:"total_files"`
	HasMore      bool `json:"has_more"`
}

type FolderShareLinkResponse struct {
	ID            string `json:"id"`
	Token         string `json:"token"`
	IsActive      bool   `json:"is_active"`
	DownloadCount int    `json:"download_count"`
	CreatedAt     string `json:"created_at"`
}

// FolderShareLinkStatusResponse represents the response for checking if a folder has sharelinks
type FolderShareLinkStatusResponse struct {
	HasShareLink bool    `json:"has_share_link"`
	Token        *string `json:"token,omitempty"`
}

// Helper functions
func convertFolderToResponse(folder *models.Folder) FolderResponse {
	var parentIDStr *string
	if folder.ParentID != nil {
		str := folder.ParentID.String()
		parentIDStr = &str
	}

	return FolderResponse{
		ID:        folder.ID.String(),
		Name:      folder.Name,
		ParentID:  parentIDStr,
		CreatedAt: folder.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: folder.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func convertFileToFolderResponse(file *models.File) FolderFileResponse {
	var folderIDStr *string
	if file.FolderID != nil {
		str := file.FolderID.String()
		folderIDStr = &str
	}

	return FolderFileResponse{
		ID:               file.ID.String(),
		OriginalFilename: file.OriginalFilename,
		MimeType:         file.MimeType,
		SizeBytes:        file.SizeBytes,
		IsPublic:         file.IsPublic,
		DownloadCount:    file.DownloadCount,
		Tags:             file.Tags,
		FolderID:         folderIDStr,
		CreatedAt:        file.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:        file.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func convertPaginationInfo(info services.FolderPaginationInfo) FolderPaginationInfo {
	return FolderPaginationInfo{
		Page:         info.Page,
		PageSize:     info.PageSize,
		TotalItems:   info.TotalItems,
		TotalPages:   info.TotalPages,
		HasNext:      info.HasNext,
		HasPrevious:  info.HasPrevious,
	}
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

// extractUserFromAuth extracts user ID from Authorization header (same as FilesHandlers)
func (h *FoldersHandlers) extractUserFromAuth(r *http.Request) (uuid.UUID, error) {
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

// HandleCreateFolder handles POST /folders
// @Summary Create a new folder
// @Description Create a new folder in the user's file system
// @Tags folders
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateFolderRequest true "Folder creation data"
// @Success 201 {object} FolderResponse "Folder successfully created"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 409 {object} ErrorResponse "Folder already exists"
// @Router /folders [post]
func (h *FoldersHandlers) HandleCreateFolder(w http.ResponseWriter, r *http.Request) {
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	var req CreateFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body")
		return
	}

	if req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "MISSING_NAME", "Folder name is required")
		return
	}

	folder, err := h.folderService.CreateFolder(userID, req.Name, req.ParentID)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeJSONError(w, http.StatusConflict, "FOLDER_EXISTS", err.Error())
			return
		}
		if strings.Contains(err.Error(), "maximum folder depth") {
			writeJSONError(w, http.StatusBadRequest, "MAX_DEPTH_EXCEEDED", err.Error())
			return
		}
		if strings.Contains(err.Error(), "not found") {
			writeJSONError(w, http.StatusNotFound, "PARENT_NOT_FOUND", err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "CREATE_FAILED", "Failed to create folder")
		return
	}

	response := convertFolderToResponse(folder)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// HandleListFolders handles GET /folders
// @Summary List folder children
// @Description Get folders and files within a specific folder (or root)
// @Tags folders
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param parent_id query string false "Parent folder ID (omit for root)"
// @Param page query int false "Page number for file pagination" default(1)
// @Param page_size query int false "Items per page for files" default(20)
// @Success 200 {object} FolderChildrenResponse "Folder contents retrieved successfully"
// @Failure 400 {object} ErrorResponse "Invalid request parameters"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 404 {object} ErrorResponse "Parent folder not found"
// @Router /folders [get]
func (h *FoldersHandlers) HandleListFolders(w http.ResponseWriter, r *http.Request) {
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	var parentID *uuid.UUID
	if parentIDStr := r.URL.Query().Get("parent_id"); parentIDStr != "" {
		if parsed, err := uuid.Parse(parentIDStr); err != nil {
			writeJSONError(w, http.StatusBadRequest, "INVALID_PARENT_ID", "Invalid parent folder ID format")
			return
		} else {
			parentID = &parsed
		}
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	childrenResponse, err := h.folderService.ListChildren(userID, parentID, page, pageSize)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSONError(w, http.StatusNotFound, "PARENT_NOT_FOUND", err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "LIST_FAILED", "Failed to list folder children")
		return
	}

	// Convert to response format
	folderResponses := make([]FolderResponse, len(childrenResponse.Folders))
	for i, folder := range childrenResponse.Folders {
		folderResponses[i] = convertFolderToResponse(folder)
	}

	fileResponses := make([]FolderFileResponse, len(childrenResponse.Files))
	for i, file := range childrenResponse.Files {
		fileResponses[i] = convertFileToFolderResponse(file)
	}

	// Add additional fields for GraphQL compatibility
	pagination := convertPaginationInfo(childrenResponse.FilesPagination)
	pagination.TotalFolders = len(folderResponses)
	pagination.TotalFiles = len(fileResponses)
	pagination.HasMore = pagination.HasNext

	response := FolderChildrenResponse{
		Folders:         folderResponses,
		Files:           fileResponses,
		FilesPagination: pagination,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleGetFolder handles GET /folders/{id}
// @Summary Get folder details
// @Description Get detailed information about a specific folder including breadcrumbs
// @Tags folders
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Folder ID"
// @Success 200 {object} FolderDetailsResponse "Folder details retrieved successfully"
// @Failure 400 {object} ErrorResponse "Invalid folder ID"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 404 {object} ErrorResponse "Folder not found"
// @Router /folders/{id} [get]
func (h *FoldersHandlers) HandleGetFolder(w http.ResponseWriter, r *http.Request) {
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	vars := mux.Vars(r)
	folderID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_FOLDER_ID", "Invalid folder ID format")
		return
	}

	folder, err := h.folderService.GetFolderByID(userID, folderID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSONError(w, http.StatusNotFound, "FOLDER_NOT_FOUND", err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "GET_FAILED", "Failed to get folder details")
		return
	}

	breadcrumbs, err := h.folderService.GetBreadcrumbs(userID, folderID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "BREADCRUMBS_FAILED", "Failed to get breadcrumbs")
		return
	}

	// Convert to response format
	folderResponse := convertFolderToResponse(folder)
	breadcrumbResponses := make([]FolderResponse, len(breadcrumbs))
	for i, breadcrumb := range breadcrumbs {
		breadcrumbResponses[i] = convertFolderToResponse(breadcrumb)
	}

	response := FolderDetailsResponse{
		Folder:      folderResponse,
		Breadcrumbs: breadcrumbResponses,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleUpdateFolder handles PATCH /folders/{id}
// @Summary Update folder
// @Description Update folder name or move to different parent
// @Tags folders
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Folder ID"
// @Param request body UpdateFolderRequest true "Folder update data"
// @Success 200 {object} FolderResponse "Folder updated successfully"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 404 {object} ErrorResponse "Folder not found"
// @Router /folders/{id} [patch]
func (h *FoldersHandlers) HandleUpdateFolder(w http.ResponseWriter, r *http.Request) {
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	vars := mux.Vars(r)
	folderID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_FOLDER_ID", "Invalid folder ID format")
		return
	}

	var req UpdateFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body")
		return
	}

	var folder *models.Folder
	
	// Handle name change vs move - use separate service methods
	if req.Name != "" && req.ParentID != nil {
		// Both name and parent change - need to handle separately
		// First get current folder to check current parent
		currentFolder, getErr := h.folderService.GetFolderByID(userID, folderID)
		if getErr != nil {
			if strings.Contains(getErr.Error(), "not found") {
				writeJSONError(w, http.StatusNotFound, "FOLDER_NOT_FOUND", getErr.Error())
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to get folder")
			return
		}
		
		// Check if parent is actually changing
		var currentParentChanged bool
		if currentFolder.ParentID == nil && req.ParentID != nil {
			currentParentChanged = true
		} else if currentFolder.ParentID != nil && req.ParentID == nil {
			currentParentChanged = true
		} else if currentFolder.ParentID != nil && req.ParentID != nil && *currentFolder.ParentID != *req.ParentID {
			currentParentChanged = true
		}
		
		if currentParentChanged {
			// Move first, then rename
			folder, err = h.folderService.MoveFolder(userID, folderID, req.ParentID)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					writeJSONError(w, http.StatusNotFound, "FOLDER_NOT_FOUND", err.Error())
					return
				}
				if strings.Contains(err.Error(), "cycle") || strings.Contains(err.Error(), "descendant") {
					writeJSONError(w, http.StatusBadRequest, "CIRCULAR_REFERENCE", err.Error())
					return
				}
				writeJSONError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to move folder")
				return
			}
		}
		
		// Then rename if name changed
		if currentFolder.Name != req.Name {
			folder, err = h.folderService.RenameFolder(userID, folderID, req.Name)
		}
	} else if req.Name != "" {
		// Only name change
		folder, err = h.folderService.RenameFolder(userID, folderID, req.Name)
	} else if req.ParentID != nil {
		// Only parent change
		folder, err = h.folderService.MoveFolder(userID, folderID, req.ParentID)
	} else {
		writeJSONError(w, http.StatusBadRequest, "NO_CHANGES", "No changes specified")
		return
	}
	
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSONError(w, http.StatusNotFound, "FOLDER_NOT_FOUND", err.Error())
			return
		}
		if strings.Contains(err.Error(), "cycle") || strings.Contains(err.Error(), "descendant") {
			writeJSONError(w, http.StatusBadRequest, "CIRCULAR_REFERENCE", err.Error())
			return
		}
		if strings.Contains(err.Error(), "already exists") {
			writeJSONError(w, http.StatusConflict, "FOLDER_EXISTS", err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to update folder")
		return
	}

	response := convertFolderToResponse(folder)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleDeleteFolder handles DELETE /folders/{id}
// @Summary Delete folder
// @Description Delete a folder and all its contents
// @Tags folders
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Folder ID"
// @Success 204 "Folder deleted successfully"
// @Failure 400 {object} ErrorResponse "Invalid folder ID"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 404 {object} ErrorResponse "Folder not found"
// @Router /folders/{id} [delete]
func (h *FoldersHandlers) HandleDeleteFolder(w http.ResponseWriter, r *http.Request) {
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	vars := mux.Vars(r)
	folderID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_FOLDER_ID", "Invalid folder ID format")
		return
	}

	// Default to recursive delete as per Task specification
	recursive := true
	if r.URL.Query().Get("recursive") == "false" {
		recursive = false
	}
	
	err = h.folderService.DeleteFolder(userID, folderID, recursive)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSONError(w, http.StatusNotFound, "FOLDER_NOT_FOUND", err.Error())
			return
		}
		if strings.Contains(err.Error(), "not empty") {
			writeJSONError(w, http.StatusConflict, "FOLDER_NOT_EMPTY", err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "DELETE_FAILED", "Failed to delete folder")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleCreateFolderShareLink handles POST /folders/{id}/share
// @Summary Create folder share link
// @Description Create a public share link for a folder
// @Tags folders
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Folder ID"
// @Success 200 {object} FolderShareLinkResponse "Share link created successfully"
// @Failure 400 {object} ErrorResponse "Invalid folder ID"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 404 {object} ErrorResponse "Folder not found"
// @Router /folders/{id}/share [post]
func (h *FoldersHandlers) HandleCreateFolderShareLink(w http.ResponseWriter, r *http.Request) {
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	vars := mux.Vars(r)
	folderID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_FOLDER_ID", "Invalid folder ID format")
		return
	}

	_, shareLink, err := h.folderService.SetFolderPublic(folderID, true, userID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSONError(w, http.StatusNotFound, "FOLDER_NOT_FOUND", "Folder not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "SHARE_LINK_FAILED", "Failed to create share link")
		return
	}

	response := FolderShareLinkResponse{
		ID:            shareLink.ID.String(),
		Token:         shareLink.Token,
		IsActive:      shareLink.IsActive,
		DownloadCount: shareLink.DownloadCount,
		CreatedAt:     shareLink.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleDeleteFolderShareLink handles DELETE /folders/{id}/share
// @Summary Delete folder share link
// @Description Remove the public share link for a folder
// @Tags folders
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Folder ID"
// @Success 204 "Share link deleted successfully"
// @Failure 400 {object} ErrorResponse "Invalid folder ID"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 404 {object} ErrorResponse "Folder or share link not found"
// @Router /folders/{id}/share [delete]
func (h *FoldersHandlers) HandleDeleteFolderShareLink(w http.ResponseWriter, r *http.Request) {
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	vars := mux.Vars(r)
	folderID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_FOLDER_ID", "Invalid folder ID format")
		return
	}

	_, _, err = h.folderService.SetFolderPublic(folderID, false, userID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSONError(w, http.StatusNotFound, "FOLDER_NOT_FOUND", "Folder not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "DELETE_SHARE_LINK_FAILED", "Failed to delete share link")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandlePublicFolderAccess handles GET /p/f/{token}
// @Summary Access public folder
// @Description Access a folder via public share token (no authentication required)
// @Tags public
// @Accept json
// @Produce json
// @Param token path string true "Share token"
// @Param page query int false "Page number for file pagination" default(1)
// @Param page_size query int false "Items per page for files" default(20)
// @Success 200 {object} FolderChildrenResponse "Folder contents retrieved successfully"
// @Failure 400 {object} ErrorResponse "Invalid token"
// @Failure 403 {object} ErrorResponse "Link revoked"
// @Failure 404 {object} ErrorResponse "Link not found"
// @Failure 410 {object} ErrorResponse "Link expired"
// @Router /p/f/{token} [get]
func (h *FoldersHandlers) HandlePublicFolderAccess(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	token := vars["token"]

	if token == "" {
		writeJSONError(w, http.StatusBadRequest, "INVALID_TOKEN", "Invalid share token")
		return
	}

	// Get folder by public share token
	folder, err := h.folderService.GetFolderByShareToken(token)
	if err != nil {
		// Import the error constants from services package
		if err.Error() == "share link not found" {
			writeJSONError(w, http.StatusNotFound, "LINK_NOT_FOUND", "Public folder link not found")
		} else if err.Error() == "share link has expired" {
			writeJSONError(w, http.StatusGone, "LINK_EXPIRED", "Public folder link has expired")
		} else if err.Error() == "invalid share token format" {
			writeJSONError(w, http.StatusBadRequest, "INVALID_TOKEN", "Invalid share token format")
		} else {
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve folder")
		}
		return
	}

	// Get pagination parameters
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Get folder contents using the folder's owner ID and folder ID
	folderID := &folder.ID
	childrenResponse, err := h.folderService.ListChildren(folder.OwnerID, folderID, page, pageSize)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "LIST_FAILED", "Failed to list folder contents")
		return
	}

	// Convert to response format (same as authenticated endpoint)
	folderResponses := make([]FolderResponse, len(childrenResponse.Folders))
	for i, childFolder := range childrenResponse.Folders {
		folderResponses[i] = convertFolderToResponse(childFolder)
	}

	fileResponses := make([]FolderFileResponse, len(childrenResponse.Files))
	for i, file := range childrenResponse.Files {
		fileResponses[i] = convertFileToFolderResponse(file)
	}

	// Add additional fields for compatibility
	pagination := convertPaginationInfo(childrenResponse.FilesPagination)
	pagination.TotalFolders = len(folderResponses)
	pagination.TotalFiles = len(fileResponses)
	pagination.HasMore = pagination.HasNext

	response := FolderChildrenResponse{
		Folders:         folderResponses,
		Files:           fileResponses,
		FilesPagination: pagination,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleCreateFolderShareLinkWithFilePublicity handles POST /folders/{id}/share with automatic file publicity management
// @Summary Create folder share link with file publicity
// @Description Create a public share link for a folder and automatically make all nested files public
// @Tags folders
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Folder ID"
// @Success 200 {object} FolderShareLinkResponse "Share link created successfully with files made public"
// @Failure 400 {object} ErrorResponse "Invalid folder ID"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 404 {object} ErrorResponse "Folder not found"
// @Router /folders/{id}/share [post]
func (h *FoldersHandlers) HandleCreateFolderShareLinkWithFilePublicity(w http.ResponseWriter, r *http.Request) {
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	vars := mux.Vars(r)
	folderID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_FOLDER_ID", "Invalid folder ID format")
		return
	}

	// First create the sharelink using existing logic
	_, shareLink, err := h.folderService.SetFolderPublic(folderID, true, userID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSONError(w, http.StatusNotFound, "FOLDER_NOT_FOUND", "Folder not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "SHARE_LINK_FAILED", "Failed to create share link")
		return
	}

	// Then make all files in the folder and subfolders public
	err = h.folderService.MakeAllFilesInFolderPublic(folderID, userID)
	if err != nil {
		// Log the error but don't fail the sharelink creation
		log.Printf("Warning: Failed to make files public for folder %s: %v", folderID, err)
		// Could consider reverting the sharelink creation here if desired
	}

	response := FolderShareLinkResponse{
		ID:            shareLink.ID.String(),
		Token:         shareLink.Token,
		IsActive:      shareLink.IsActive,
		DownloadCount: shareLink.DownloadCount,
		CreatedAt:     shareLink.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleDeleteFolderShareLinkWithFilePublicity handles DELETE /folders/{id}/share with automatic file publicity reversion
// @Summary Delete folder share link with file publicity reversion
// @Description Remove the public share link for a folder and revert files to original publicity state
// @Tags folders
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Folder ID"
// @Success 204 "Share link deleted successfully with files reverted"
// @Failure 400 {object} ErrorResponse "Invalid folder ID"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 404 {object} ErrorResponse "Folder or share link not found"
// @Router /folders/{id}/share [delete]
func (h *FoldersHandlers) HandleDeleteFolderShareLinkWithFilePublicity(w http.ResponseWriter, r *http.Request) {
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	vars := mux.Vars(r)
	folderID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_FOLDER_ID", "Invalid folder ID format")
		return
	}

	log.Printf("Deleting folder sharelink for folder %s, user %s", folderID, userID)

	// First revert files to their original publicity state
	err = h.folderService.RevertFilesInFolderToOriginalState(folderID, userID)
	if err != nil {
		log.Printf("ERROR: Failed to revert files publicity for folder %s: %v", folderID, err)
		// If reversion fails, we should not continue with sharelink deletion
		writeJSONError(w, http.StatusInternalServerError, "FILE_REVERT_FAILED", "Failed to revert files to original state")
		return
	}

	// Then delete the sharelink using existing logic
	_, _, err = h.folderService.SetFolderPublic(folderID, false, userID)
	if err != nil {
		log.Printf("ERROR: Failed to delete sharelink for folder %s: %v", folderID, err)
		if strings.Contains(err.Error(), "not found") {
			writeJSONError(w, http.StatusNotFound, "FOLDER_NOT_FOUND", "Folder not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "DELETE_SHARE_LINK_FAILED", "Failed to delete share link")
		return
	}

	log.Printf("Successfully deleted folder sharelink for folder %s", folderID)
	w.WriteHeader(http.StatusNoContent)
}

// HandleCheckFolderShareLinkStatus handles GET /folders/{id}/share/status
// @Summary Check if folder has sharelinks
// @Description Check whether a folder has any active sharelinks and return the token if exists
// @Tags folders
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Folder ID"
// @Success 200 {object} FolderShareLinkStatusResponse "Sharelink status retrieved successfully with token if exists"
// @Failure 400 {object} ErrorResponse "Invalid folder ID"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Failure 404 {object} ErrorResponse "Folder not found"
// @Router /folders/{id}/share/status [get]
func (h *FoldersHandlers) HandleCheckFolderShareLinkStatus(w http.ResponseWriter, r *http.Request) {
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	vars := mux.Vars(r)
	folderID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_FOLDER_ID", "Invalid folder ID format")
		return
	}

	log.Printf("Checking sharelink status for folder %s, user %s", folderID, userID)

	// Check if folder has sharelinks
	hasShareLink, token, err := h.folderService.HasShareLink(folderID, userID)
	if err != nil {
		log.Printf("ERROR: Failed to check sharelink status for folder %s: %v", folderID, err)
		if strings.Contains(err.Error(), "not found") {
			writeJSONError(w, http.StatusNotFound, "FOLDER_NOT_FOUND", "Folder not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "CHECK_SHARELINK_FAILED", "Failed to check sharelink status")
		return
	}

	response := FolderShareLinkStatusResponse{
		HasShareLink: hasShareLink,
		Token:        token,
	}

	log.Printf("Folder %s sharelink status: %t", folderID, hasShareLink)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}