package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"securevault-backend/src/models"
	"securevault-backend/src/services"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// AuthHandlers provides HTTP handlers for authentication endpoints
type AuthHandlers struct {
	authService *services.AuthService
}

// NewAuthHandlers creates a new AuthHandlers instance
func NewAuthHandlers(authService *services.AuthService) *AuthHandlers {
	return &AuthHandlers{
		authService: authService,
	}
}

// SignupRequest represents the request payload for user signup
type SignupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name,omitempty"`
}

// LoginRequest represents the request payload for user login
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse represents the response for successful authentication
type AuthResponse struct {
	Token string             `json:"token"`
	User  *UserResponseData  `json:"user"`
}

// UserResponseData represents user data in API responses (excludes sensitive fields)
type UserResponseData struct {
	ID                uuid.UUID          `json:"id"`
	Email             string             `json:"email"`
	Name              string             `json:"name,omitempty"`
	Role              models.UserRole    `json:"role"`
	RateLimitRps      int                `json:"rate_limit_rps"`
	StorageQuotaBytes int64              `json:"storage_quota_bytes"`
	CreatedAt         time.Time          `json:"created_at"`
}

// ErrorResponse represents standardized error response
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error information
type ErrorDetail struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// HandleSignup handles POST /api/v1/auth/signup
// @Summary Register a new user
// @Description Create a new user account with email and password
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body SignupRequest true "User registration data"
// @Success 201 {object} AuthResponse "User successfully registered"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 409 {object} ErrorResponse "Email already exists"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /auth/signup [post]
func (h *AuthHandlers) HandleSignup(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", 
			"Invalid JSON in request body", nil)
		return
	}

	// Validate required fields
	if req.Email == "" {
		h.sendErrorResponse(w, http.StatusBadRequest, "MISSING_EMAIL", 
			"Email is required", nil)
		return
	}

	if req.Password == "" {
		h.sendErrorResponse(w, http.StatusBadRequest, "MISSING_PASSWORD", 
			"Password is required", nil)
		return
	}

	// Validate email format
	if !h.isValidEmail(req.Email) {
		h.sendErrorResponse(w, http.StatusBadRequest, "INVALID_EMAIL", 
			"Invalid email format", nil)
		return
	}

	// Validate password strength
	if len(req.Password) < 8 {
		h.sendErrorResponse(w, http.StatusBadRequest, "WEAK_PASSWORD", 
			"Password must be at least 8 characters long", nil)
		return
	}

	// Normalize email (lowercase)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	// Try to sign up the user
	signupReq := &services.SignUpRequest{
		Email:    req.Email,
		Password: req.Password,
	}
	
	authResp, err := h.authService.SignUp(signupReq)
	if err != nil {
		log.Printf("Signup error: %v", err)
		
		// Check for duplicate email error
		if err == services.ErrUserExists || strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			h.sendErrorResponse(w, http.StatusConflict, "EMAIL_EXISTS", 
				"A user with this email already exists", nil)
			return
		}

		h.sendErrorResponse(w, http.StatusInternalServerError, "SIGNUP_FAILED", 
			"Failed to create user account", nil)
		return
	}

	// Prepare response
	response := AuthResponse{
		Token: authResp.Token,
		User: &UserResponseData{
			ID:                authResp.User.ID,
			Email:             authResp.User.Email,
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

// HandleLogin handles POST /api/v1/auth/login
// @Summary Authenticate user
// @Description Authenticate user with email and password to get JWT token
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body LoginRequest true "User login credentials"
// @Success 200 {object} AuthResponse "User successfully authenticated"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 401 {object} ErrorResponse "Invalid credentials"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /auth/login [post]
func (h *AuthHandlers) HandleLogin(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", 
			"Invalid JSON in request body", nil)
		return
	}

	// Validate required fields
	if req.Email == "" {
		h.sendErrorResponse(w, http.StatusBadRequest, "MISSING_EMAIL", 
			"Email is required", nil)
		return
	}

	if req.Password == "" {
		h.sendErrorResponse(w, http.StatusBadRequest, "MISSING_PASSWORD", 
			"Password is required", nil)
		return
	}

	// Normalize email
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	// Attempt login
	loginReq := &services.LoginRequest{
		Email:    req.Email,
		Password: req.Password,
	}
	
	authResp, err := h.authService.Login(loginReq)
	if err != nil {
		log.Printf("Login error: %v", err)
		
		// Check for authentication failure
		if err == services.ErrInvalidCredentials || 
		   strings.Contains(err.Error(), "invalid") ||
		   strings.Contains(err.Error(), "not found") {
			h.sendErrorResponse(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", 
				"Invalid email or password", nil)
			return
		}

		h.sendErrorResponse(w, http.StatusInternalServerError, "LOGIN_FAILED", 
			"Login failed due to server error", nil)
		return
	}

	// Prepare response
	response := AuthResponse{
		Token: authResp.Token,
		User: &UserResponseData{
			ID:                authResp.User.ID,
			Email:             authResp.User.Email,
			Role:              authResp.User.Role,
			RateLimitRps:      authResp.User.RateLimitRps,
			StorageQuotaBytes: authResp.User.StorageQuotaBytes,
			CreatedAt:         authResp.User.CreatedAt,
		},
	}

	// Send successful response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// sendErrorResponse sends a standardized error response
func (h *AuthHandlers) sendErrorResponse(w http.ResponseWriter, statusCode int, errorCode, message string, details interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResp := ErrorResponse{
		Error: ErrorDetail{
			Code:    errorCode,
			Message: message,
			Details: details,
		},
	}

	json.NewEncoder(w).Encode(errorResp)
}

// isValidEmail performs basic email format validation
func (h *AuthHandlers) isValidEmail(email string) bool {
	// Basic email validation - at minimum should contain @ and a dot
	if len(email) < 5 {
		return false
	}
	
	atIndex := strings.Index(email, "@")
	if atIndex <= 0 || atIndex >= len(email)-1 {
		return false
	}
	
	dotIndex := strings.LastIndex(email, ".")
	if dotIndex <= atIndex || dotIndex >= len(email)-1 {
		return false
	}
	
	return true
}

// HandleDeleteUser handles DELETE /api/v1/users/{id}
// @Summary Delete user account
// @Description Delete a user account (user can only delete their own account)
// @Tags Authentication
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID"
// @Success 204 "User successfully deleted"
// @Failure 400 {object} ErrorResponse "Invalid user ID"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Forbidden - can only delete own account"
// @Failure 404 {object} ErrorResponse "User not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /users/{id} [delete]
func (h *AuthHandlers) HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from JWT token (authenticated user)
	userIDFromToken, exists := r.Context().Value("user_id").(uuid.UUID)
	if !exists {
		h.sendErrorResponse(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
		return
	}

	// Get user ID from URL parameter
	vars := mux.Vars(r)
	userIDParam := vars["id"]
	if userIDParam == "" {
		h.sendErrorResponse(w, http.StatusBadRequest, "MISSING_USER_ID", "User ID is required", nil)
		return
	}

	// Parse user ID from URL
	userID, err := uuid.Parse(userIDParam)
	if err != nil {
		h.sendErrorResponse(w, http.StatusBadRequest, "INVALID_USER_ID", "Invalid user ID format", nil)
		return
	}

	// Check if user is trying to delete their own account
	if userID != userIDFromToken {
		h.sendErrorResponse(w, http.StatusForbidden, "FORBIDDEN", "You can only delete your own account", nil)
		return
	}

	// Delete user
	err = h.authService.DeleteUser(userID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.sendErrorResponse(w, http.StatusNotFound, "USER_NOT_FOUND", "User not found", nil)
			return
		}

		h.sendErrorResponse(w, http.StatusInternalServerError, "DELETE_FAILED", "Failed to delete user", nil)
		return
	}

	// Send successful response
	w.WriteHeader(http.StatusNoContent)
}

// UpdatePasswordRequest represents the request payload for password update
type UpdatePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// HandleUpdatePassword handles PATCH /api/v1/users/{id}/password
// @Summary Update user password
// @Description Update a user's password (user can only update their own password)
// @Tags Authentication
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID"
// @Param request body UpdatePasswordRequest true "Password update data"
// @Success 200 {object} map[string]string "Password successfully updated"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Forbidden - can only update own password"
// @Failure 404 {object} ErrorResponse "User not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /users/{id}/password [patch]
func (h *AuthHandlers) HandleUpdatePassword(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from JWT token (authenticated user)
	userIDFromToken, exists := r.Context().Value("user_id").(uuid.UUID)
	if !exists {
		h.sendErrorResponse(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
		return
	}

	// Get user ID from URL parameter
	vars := mux.Vars(r)
	userIDParam := vars["id"]
	if userIDParam == "" {
		h.sendErrorResponse(w, http.StatusBadRequest, "MISSING_USER_ID", "User ID is required", nil)
		return
	}

	// Parse user ID from URL
	userID, err := uuid.Parse(userIDParam)
	if err != nil {
		h.sendErrorResponse(w, http.StatusBadRequest, "INVALID_USER_ID", "Invalid user ID format", nil)
		return
	}

	// Check if user is trying to update their own password
	if userID != userIDFromToken {
		h.sendErrorResponse(w, http.StatusForbidden, "FORBIDDEN", "You can only update your own password", nil)
		return
	}

	// Parse request body
	var req UpdatePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", nil)
		return
	}

	// Validate required fields
	if req.CurrentPassword == "" {
		h.sendErrorResponse(w, http.StatusBadRequest, "MISSING_CURRENT_PASSWORD", "Current password is required", nil)
		return
	}

	if req.NewPassword == "" {
		h.sendErrorResponse(w, http.StatusBadRequest, "MISSING_NEW_PASSWORD", "New password is required", nil)
		return
	}

	// Validate password strength
	if len(req.NewPassword) < 8 {
		h.sendErrorResponse(w, http.StatusBadRequest, "WEAK_PASSWORD", "New password must be at least 8 characters long", nil)
		return
	}

	// Update password using auth service
	serviceReq := &services.UpdatePasswordRequest{
		CurrentPassword: req.CurrentPassword,
		NewPassword:     req.NewPassword,
	}

	err = h.authService.UpdatePassword(userID, serviceReq)
	if err != nil {
		if strings.Contains(err.Error(), "current password is incorrect") {
			h.sendErrorResponse(w, http.StatusBadRequest, "INVALID_CURRENT_PASSWORD", "Current password is incorrect", nil)
			return
		}

		if strings.Contains(err.Error(), "not found") {
			h.sendErrorResponse(w, http.StatusNotFound, "USER_NOT_FOUND", "User not found", nil)
			return
		}

		h.sendErrorResponse(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to update password", nil)
		return
	}

	// Send successful response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Password updated successfully",
	})
}
