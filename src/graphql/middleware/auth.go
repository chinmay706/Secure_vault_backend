package middleware

import (
	"context"
	"log"
	"net/http"
	"strings"

	"securevault-backend/src/services"

	"github.com/google/uuid"
)

type contextKey string

const (
	UserIDKey   contextKey = "user_id"
	UserRoleKey contextKey = "user_role"
)

// AuthMiddleware extracts JWT from Authorization header and puts user info in context
func AuthMiddleware(authService *services.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Extract Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				
				// Validate token
				claims, err := authService.ValidateToken(token)
				if err == nil {
					// Add user info to context
					ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
					ctx = context.WithValue(ctx, UserRoleKey, claims.Role)
				}
			}

			// Continue with request
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserID extracts user ID from context
func GetUserID(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value(UserIDKey).(uuid.UUID)
	return userID, ok
}

// GetUserRole extracts user role from context
func GetUserRole(ctx context.Context) (string, bool) {
	role, ok := ctx.Value(UserRoleKey).(string)
	return role, ok
}

// RequireAuth ensures user is authenticated
func RequireAuth(ctx context.Context) (uuid.UUID, error) {
	userID, ok := GetUserID(ctx)
	if !ok {
		log.Printf("[AUTH] RequireAuth failed - no user ID in context")
		return uuid.Nil, ErrUnauthorized
	}
	log.Printf("[AUTH] RequireAuth success - userID: %s", userID)
	return userID, nil
}

// RequireAdmin ensures user is authenticated and has admin role
func RequireAdmin(ctx context.Context) (uuid.UUID, error) {
	userID, err := RequireAuth(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	
	role, ok := GetUserRole(ctx)
	if !ok || role != "admin" {
		return uuid.Nil, ErrForbidden
	}
	
	return userID, nil
}

// Error definitions
var (
	ErrUnauthorized = &AuthError{Code: "UNAUTHORIZED", Message: "Authentication required"}
	ErrForbidden    = &AuthError{Code: "FORBIDDEN", Message: "Admin access required"}
)

type AuthError struct {
	Code    string
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}