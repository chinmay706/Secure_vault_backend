package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"securevault-backend/src/services"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type SummaryHandlers struct {
	aiSummaryService *services.AiSummaryService
	authService      *services.AuthService
}

func NewSummaryHandlers(aiSummaryService *services.AiSummaryService, authService *services.AuthService) *SummaryHandlers {
	return &SummaryHandlers{
		aiSummaryService: aiSummaryService,
		authService:      authService,
	}
}

func (h *SummaryHandlers) extractUserFromAuth(r *http.Request) (uuid.UUID, error) {
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

func (h *SummaryHandlers) writeError(w http.ResponseWriter, code string, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

// HandleGenerateAiSummary handles POST /files/{id}/ai-summary
func (h *SummaryHandlers) HandleGenerateAiSummary(w http.ResponseWriter, r *http.Request) {
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

	if !h.aiSummaryService.IsEnabled() {
		h.writeError(w, "UNAVAILABLE", "AI summary service not configured", http.StatusServiceUnavailable)
		return
	}

	// Check if a completed summary exists (return it unless ?force=true)
	force := r.URL.Query().Get("force") == "true"
	if !force && h.aiSummaryService.HasExistingCompleted(fileID, userID) {
		existing, err := h.aiSummaryService.GetSummary(fileID, userID)
		if err == nil && existing != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(existing)
			return
		}
	}

	// Rate limit check
	if err := h.aiSummaryService.CheckRateLimit(userID, 1.0); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	// Fire async generation
	go h.aiSummaryService.GenerateSummary(fileID, userID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"file_id": fileID.String(),
		"status":  "processing",
		"message": "AI summary generation started",
	})
}

// HandleGetAiSummary handles GET /files/{id}/ai-summary
func (h *SummaryHandlers) HandleGetAiSummary(w http.ResponseWriter, r *http.Request) {
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

	summary, err := h.aiSummaryService.GetSummary(fileID, userID)
	if err != nil {
		h.writeError(w, "ERROR", err.Error(), http.StatusInternalServerError)
		return
	}
	if summary == nil {
		h.writeError(w, "NOT_FOUND", "No summary exists for this file", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if summary.Status == "pending" || summary.Status == "processing" {
		w.WriteHeader(http.StatusAccepted)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	json.NewEncoder(w).Encode(summary)
}

// HandleRefineAiSummary handles POST /files/{id}/ai-summary/refine
func (h *SummaryHandlers) HandleRefineAiSummary(w http.ResponseWriter, r *http.Request) {
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
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, "INVALID_REQUEST", "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Command) == "" {
		h.writeError(w, "INVALID_REQUEST", "command field is required", http.StatusBadRequest)
		return
	}

	if !h.aiSummaryService.IsEnabled() {
		h.writeError(w, "UNAVAILABLE", "AI summary service not configured", http.StatusServiceUnavailable)
		return
	}

	// Rate limit check (refinement = 0.5 cost)
	if err := h.aiSummaryService.CheckRateLimit(userID, services.SummaryRefinementCostExported); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	updated, err := h.aiSummaryService.RefineSummary(fileID, userID, req.Command)
	if err != nil {
		if strings.Contains(err.Error(), "no summary found") {
			h.writeError(w, "NOT_FOUND", err.Error(), http.StatusNotFound)
		} else if strings.Contains(err.Error(), "not in completed state") {
			h.writeError(w, "CONFLICT", err.Error(), http.StatusConflict)
		} else {
			h.writeError(w, "ERROR", err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(updated)
}
