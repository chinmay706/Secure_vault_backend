package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"securevault-backend/src/models"
	"securevault-backend/src/services"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

var targetMimeTypes = map[string]string{
	"pdf":  "application/pdf",
	"txt":  "text/plain",
	"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
}

type ConversionHandlers struct {
	conversionService *services.ConversionService
	authService       *services.AuthService
}

func NewConversionHandlers(conversionService *services.ConversionService, authService *services.AuthService) *ConversionHandlers {
	return &ConversionHandlers{
		conversionService: conversionService,
		authService:       authService,
	}
}

func (h *ConversionHandlers) extractUserFromAuth(r *http.Request) (uuid.UUID, error) {
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

func (h *ConversionHandlers) writeError(w http.ResponseWriter, code string, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

// HandleStartConversion handles POST /api/v1/files/{id}/convert
func (h *ConversionHandlers) HandleStartConversion(w http.ResponseWriter, r *http.Request) {
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		h.writeError(w, "UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	fileID, err := uuid.Parse(vars["id"])
	if err != nil {
		h.writeError(w, "INVALID_ID", "Invalid file ID format", http.StatusBadRequest)
		return
	}

	var req struct {
		TargetFormat string `json:"target_format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, "INVALID_REQUEST", "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.TargetFormat) == "" {
		h.writeError(w, "INVALID_REQUEST", "target_format is required", http.StatusBadRequest)
		return
	}

	job, err := h.conversionService.StartConversion(fileID, userID, req.TargetFormat)
	if err != nil {
		switch err {
		case services.ErrFileNotFound:
			h.writeError(w, "NOT_FOUND", "File not found", http.StatusNotFound)
		case services.ErrUnauthorizedAccess:
			h.writeError(w, "FORBIDDEN", "You do not own this file", http.StatusForbidden)
		case services.ErrConversionFileTooLarge:
			h.writeError(w, "FILE_TOO_LARGE", err.Error(), http.StatusBadRequest)
		case services.ErrConversionUnsupported:
			h.writeError(w, "UNSUPPORTED_CONVERSION", err.Error(), http.StatusBadRequest)
		case services.ErrConversionRateLimit:
			h.writeError(w, "RATE_LIMIT_EXCEEDED", err.Error(), http.StatusTooManyRequests)
		default:
			h.writeError(w, "INTERNAL_ERROR", "Failed to start conversion", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(job)
}

// HandleGetConversionJob handles GET /api/v1/conversions/{jobId}
func (h *ConversionHandlers) HandleGetConversionJob(w http.ResponseWriter, r *http.Request) {
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		h.writeError(w, "UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	jobID, err := uuid.Parse(vars["jobId"])
	if err != nil {
		h.writeError(w, "INVALID_ID", "Invalid job ID format", http.StatusBadRequest)
		return
	}

	job, err := h.conversionService.GetJob(jobID, userID)
	if err != nil {
		if err == services.ErrConversionNotFound {
			h.writeError(w, "NOT_FOUND", "Conversion job not found", http.StatusNotFound)
		} else {
			h.writeError(w, "INTERNAL_ERROR", "Failed to get conversion job", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(job)
}

// HandleDownloadConversion handles GET /api/v1/conversions/{jobId}/download
func (h *ConversionHandlers) HandleDownloadConversion(w http.ResponseWriter, r *http.Request) {
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		h.writeError(w, "UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	jobID, err := uuid.Parse(vars["jobId"])
	if err != nil {
		h.writeError(w, "INVALID_ID", "Invalid job ID format", http.StatusBadRequest)
		return
	}

	job, file, err := h.conversionService.GetResultFile(jobID, userID)
	if err != nil {
		switch err {
		case services.ErrConversionNotFound:
			h.writeError(w, "NOT_FOUND", "Conversion job not found", http.StatusNotFound)
		case services.ErrConversionNotCompleted:
			h.writeError(w, "NOT_READY", "Conversion has not completed yet", http.StatusBadRequest)
		default:
			if strings.Contains(err.Error(), "conversion failed:") {
				h.writeError(w, "CONVERSION_FAILED", err.Error(), http.StatusBadRequest)
			} else {
				h.writeError(w, "INTERNAL_ERROR", err.Error(), http.StatusInternalServerError)
			}
		}
		return
	}
	defer file.Close()

	baseName := strings.TrimSuffix(job.OriginalFilename, "."+job.SourceFormat)
	downloadName := baseName + "." + job.TargetFormat
	safeName := strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r == '\n' || r == '\r' {
			return '_'
		}
		return r
	}, downloadName)

	contentType := targetMimeTypes[job.TargetFormat]
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeName))
	io.Copy(w, file)
}

// HandleDeleteConversion handles DELETE /api/v1/conversions/{jobId}
func (h *ConversionHandlers) HandleDeleteConversion(w http.ResponseWriter, r *http.Request) {
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		h.writeError(w, "UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	jobID, err := uuid.Parse(vars["jobId"])
	if err != nil {
		h.writeError(w, "INVALID_ID", "Invalid job ID format", http.StatusBadRequest)
		return
	}

	err = h.conversionService.DeleteJob(jobID, userID)
	if err != nil {
		if err == services.ErrConversionNotFound {
			h.writeError(w, "NOT_FOUND", "Conversion not found", http.StatusNotFound)
		} else {
			h.writeError(w, "INTERNAL_ERROR", "Failed to delete conversion", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleConversionHistory handles GET /api/v1/conversions
func (h *ConversionHandlers) HandleConversionHistory(w http.ResponseWriter, r *http.Request) {
	userID, err := h.extractUserFromAuth(r)
	if err != nil {
		h.writeError(w, "UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
		return
	}

	jobs, err := h.conversionService.GetJobHistory(userID)
	if err != nil {
		h.writeError(w, "INTERNAL_ERROR", "Failed to get conversion history", http.StatusInternalServerError)
		return
	}

	if jobs == nil {
		jobs = []*models.ConversionJob{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jobs": jobs,
	})
}
