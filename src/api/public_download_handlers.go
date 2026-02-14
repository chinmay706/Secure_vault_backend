package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"securevault-backend/src/services"

	"github.com/gorilla/mux"
)

// PublicDownloadHandlers handles public file download requests
type PublicDownloadHandlers struct {
	fileService    *services.FileService
	storageService *services.StorageService
}

// NewPublicDownloadHandlers creates a new PublicDownloadHandlers instance
func NewPublicDownloadHandlers(fileService *services.FileService, storageService *services.StorageService) *PublicDownloadHandlers {
	return &PublicDownloadHandlers{
		fileService:    fileService,
		storageService: storageService,
	}
}

// HandlePublicDownload handles GET /p/{token} - public file download without authentication
// @Summary Download public file
// @Description Download a publicly shared file using share token (no authentication required)
// @Tags Public
// @Produce application/octet-stream
// @Param token path string true "Public share token"
// @Param If-None-Match header string false "ETag for cache validation"
// @Success 200 {file} file "File content"
// @Success 304 "Not modified (cached)"
// @Failure 400 {object} ErrorResponse "Invalid token"
// @Failure 403 {object} ErrorResponse "Link revoked"
// @Failure 404 {object} ErrorResponse "Link not found"
// @Failure 410 {object} ErrorResponse "Link expired"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /p/{token} [get]
// @Router /p/{token} [head]
func (h *PublicDownloadHandlers) HandlePublicDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	token := vars["token"]

	if token == "" {
		h.writeErrorResponse(w, "INVALID_TOKEN", "Invalid share token", http.StatusBadRequest)
		return
	}

	// Get file by public share token
	file, err := h.fileService.GetFileByShareToken(token)
	if err != nil {
		if err == services.ErrShareLinkNotFound {
			h.writeErrorResponse(w, "LINK_NOT_FOUND", "Public download link not found", http.StatusNotFound)
		} else if err == services.ErrShareLinkExpired {
			h.writeErrorResponse(w, "LINK_EXPIRED", "Public download link has expired", http.StatusGone)
		} else if err == services.ErrShareLinkInactive {
			h.writeErrorResponse(w, "LINK_REVOKED", "Public download link has been revoked", http.StatusForbidden)
		} else if err == services.ErrInvalidShareToken {
			h.writeErrorResponse(w, "INVALID_TOKEN", "Invalid share token format", http.StatusBadRequest)
		} else {
			h.writeErrorResponse(w, "INTERNAL_ERROR", "Failed to retrieve file", http.StatusInternalServerError)
		}
		return
	}

	// Verify the share link is active
	if !file.IsPublic {
		h.writeErrorResponse(w, "LINK_REVOKED", "Public download link has been revoked", http.StatusForbidden)
		return
	}

	// Increment download count for public downloads
	err = h.fileService.IncrementDownloadCount(file.ID)
	if err != nil {
		// Log error but don't fail the download
		// h.logger.Error("Failed to increment download count", "file_id", file.ID, "error", err)
	}

	// Get file content from storage
	reader, err := h.storageService.DownloadFile(file.BlobHash)
	if err != nil {
		h.writeErrorResponse(w, "FILE_NOT_FOUND", "File content not found in storage", http.StatusNotFound)
		return
	}
	defer reader.Close()

	// Set response headers for file download
	w.Header().Set("Content-Type", file.MimeType)
	safeName := strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r == '\n' || r == '\r' {
			return '_'
		}
		return r
	}, file.OriginalFilename)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeName))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", file.SizeBytes))
	w.Header().Set("X-Filename", file.OriginalFilename)
	
	// Set cache headers for public files
	w.Header().Set("Cache-Control", "public, max-age=3600") // Cache for 1 hour
	w.Header().Set("ETag", `"`+file.BlobHash+`"`)
	
	// Check if client has cached version
	if match := r.Header.Get("If-None-Match"); match == `"`+file.BlobHash+`"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// For HEAD requests, just return headers without body
	if r.Method == "HEAD" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Stream file content for GET requests
	w.WriteHeader(http.StatusOK)
	
	// Copy content from reader to response writer
	_, err = h.copyContent(w, reader)
	if err != nil {
		// Log error but can't change response at this point
		// h.logger.Error("Failed to stream file content", "file_id", file.ID, "error", err)
	}
}

// writeErrorResponse writes a standardized error response
func (h *PublicDownloadHandlers) writeErrorResponse(w http.ResponseWriter, code, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	})
}

// copyContent copies content from source to destination with error handling
func (h *PublicDownloadHandlers) copyContent(dst http.ResponseWriter, src interface{}) (int64, error) {
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