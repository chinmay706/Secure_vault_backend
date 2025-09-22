package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

// Limiter represents rate limiting configuration
type Limiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// LimitsMiddleware handles rate limiting and quota enforcement
type LimitsMiddleware struct {
	// Rate limiting
	limiters map[uuid.UUID]*Limiter
	mu       sync.RWMutex
	rps      rate.Limit // Requests per second
	burst    int        // Burst size
	
	// Quota checking
	quotaService QuotaService
	
	// Cleanup
	cleanupInterval time.Duration
	maxAge          time.Duration
}

// QuotaService interface for checking user quotas
type QuotaService interface {
	GetUserQuotaUsage(userID uuid.UUID) (used, quota int64, err error)
	CheckUserQuota(userID uuid.UUID, additionalSize int64) (bool, error)
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

// RateLimitDetails contains rate limit information
type RateLimitDetails struct {
	Limit       float64 `json:"limit"`        // Requests per second
	Window      string  `json:"window"`       // Time window
	RetryAfter  int     `json:"retry_after"`  // Seconds to wait
}

// QuotaDetails contains quota information
type QuotaDetails struct {
	QuotaLimit    int64 `json:"quota_limit"`
	QuotaUsed     int64 `json:"quota_used"`
	QuotaRemaining int64 `json:"quota_remaining"`
	FileSize      int64 `json:"file_size,omitempty"`
}

// NewLimitsMiddleware creates a new limits middleware
func NewLimitsMiddleware(rps float64, quotaService QuotaService) *LimitsMiddleware {
	lm := &LimitsMiddleware{
		limiters:        make(map[uuid.UUID]*Limiter),
		rps:            rate.Limit(rps),
		burst:          int(rps * 2), // Allow burst of 2x the rate
		quotaService:   quotaService,
		cleanupInterval: 5 * time.Minute,
		maxAge:         10 * time.Minute,
	}
	
	// Start cleanup goroutine
	go lm.cleanupRoutine()
	
	return lm
}

// RateLimitMiddleware creates HTTP middleware for rate limiting
func (lm *LimitsMiddleware) RateLimitMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract user ID from context (set by auth middleware)
			userID, ok := r.Context().Value("user_id").(uuid.UUID)
			if !ok || userID == uuid.Nil {
				// No user context, skip rate limiting for now
				// This could be tightened to require authentication
				next.ServeHTTP(w, r)
				return
			}

			// Check rate limit
			if !lm.checkRateLimit(userID) {
				lm.sendRateLimitError(w, userID)
				return
			}

			// Set rate limit headers
			lm.setRateLimitHeaders(w, userID)

			next.ServeHTTP(w, r)
		})
	}
}

// QuotaMiddleware creates HTTP middleware for quota checking on uploads
func (lm *LimitsMiddleware) QuotaMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only check quota for upload requests (POST to /files, multipart content)
			if r.Method != "POST" || !lm.isUploadRequest(r) {
				next.ServeHTTP(w, r)
				return
			}

			// Extract user ID from context
			userID, ok := r.Context().Value("user_id").(uuid.UUID)
			if !ok || userID == uuid.Nil {
				// No user context, let auth middleware handle this
				next.ServeHTTP(w, r)
				return
			}

			// Get content length
			contentLength := r.ContentLength
			if contentLength <= 0 {
				// Try to parse from header
				if lengthStr := r.Header.Get("Content-Length"); lengthStr != "" {
					if parsed, err := strconv.ParseInt(lengthStr, 10, 64); err == nil {
						contentLength = parsed
					}
				}
			}

			// Check if content length exceeds maximum file size (10MB default)
			maxFileSize := int64(10 * 1024 * 1024) // 10MB
			if contentLength > maxFileSize {
				lm.sendFileSizeError(w, contentLength, maxFileSize)
				return
			}

			// Check user quota
			if contentLength > 0 {
				hasQuota, err := lm.quotaService.CheckUserQuota(userID, contentLength)
				if err != nil {
					lm.sendInternalError(w, "Failed to check quota")
					return
				}

				if !hasQuota {
					lm.sendQuotaExceededError(w, userID, contentLength)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// checkRateLimit checks if user is within rate limits
func (lm *LimitsMiddleware) checkRateLimit(userID uuid.UUID) bool {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	limiter, exists := lm.limiters[userID]
	if !exists {
		limiter = &Limiter{
			limiter:  rate.NewLimiter(lm.rps, lm.burst),
			lastSeen: time.Now(),
		}
		lm.limiters[userID] = limiter
	}

	limiter.lastSeen = time.Now()
	return limiter.limiter.Allow()
}

// setRateLimitHeaders sets rate limit information headers
func (lm *LimitsMiddleware) setRateLimitHeaders(w http.ResponseWriter, userID uuid.UUID) {
	lm.mu.RLock()
	limiter, exists := lm.limiters[userID]
	lm.mu.RUnlock()

	if !exists {
		return
	}

	// Set standard rate limit headers
	w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%.0f", float64(lm.rps)))
	w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%.0f", limiter.limiter.Tokens()))
	w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(time.Second).Unix()))
}

// sendRateLimitError sends rate limit exceeded error
func (lm *LimitsMiddleware) sendRateLimitError(w http.ResponseWriter, userID uuid.UUID) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "1") // Suggest retry after 1 second
	w.WriteHeader(http.StatusTooManyRequests)

	errorResp := ErrorResponse{
		Error: ErrorDetail{
			Code:    "RATE_LIMIT_EXCEEDED",
			Message: "Rate limit exceeded. Please slow down your requests.",
			Details: RateLimitDetails{
				Limit:      float64(lm.rps),
				Window:     "1s",
				RetryAfter: 1,
			},
		},
	}

	json.NewEncoder(w).Encode(errorResp)
}

// sendQuotaExceededError sends quota exceeded error
func (lm *LimitsMiddleware) sendQuotaExceededError(w http.ResponseWriter, userID uuid.UUID, fileSize int64) {
	used, quota, err := lm.quotaService.GetUserQuotaUsage(userID)
	if err != nil {
		lm.sendInternalError(w, "Failed to get quota information")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPaymentRequired) // 402 for quota exceeded

	remaining := quota - used
	if remaining < 0 {
		remaining = 0
	}

	errorResp := ErrorResponse{
		Error: ErrorDetail{
			Code:    "QUOTA_EXCEEDED",
			Message: fmt.Sprintf("Storage quota exceeded. File size %d bytes would exceed your %d byte limit.", fileSize, quota),
			Details: QuotaDetails{
				QuotaLimit:     quota,
				QuotaUsed:      used,
				QuotaRemaining: remaining,
				FileSize:       fileSize,
			},
		},
	}

	json.NewEncoder(w).Encode(errorResp)
}

// sendFileSizeError sends file size exceeded error
func (lm *LimitsMiddleware) sendFileSizeError(w http.ResponseWriter, fileSize, maxSize int64) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusRequestEntityTooLarge) // 413

	errorResp := ErrorResponse{
		Error: ErrorDetail{
			Code:    "FILE_SIZE_EXCEEDED",
			Message: fmt.Sprintf("File size %d bytes exceeds maximum allowed size of %d bytes (%.1f MB).", 
				fileSize, maxSize, float64(maxSize)/(1024*1024)),
			Details: map[string]interface{}{
				"file_size":     fileSize,
				"max_file_size": maxSize,
				"max_size_mb":   float64(maxSize) / (1024 * 1024),
			},
		},
	}

	json.NewEncoder(w).Encode(errorResp)
}

// sendInternalError sends internal server error
func (lm *LimitsMiddleware) sendInternalError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)

	errorResp := ErrorResponse{
		Error: ErrorDetail{
			Code:    "INTERNAL_ERROR",
			Message: message,
		},
	}

	json.NewEncoder(w).Encode(errorResp)
}

// isUploadRequest determines if this is a file upload request
func (lm *LimitsMiddleware) isUploadRequest(r *http.Request) bool {
	contentType := r.Header.Get("Content-Type")
	
	// Check for multipart form data (file uploads)
	if contentType != "" {
		// Use strings.HasPrefix to safely check the prefix
		if contentType == "multipart/form-data" || strings.HasPrefix(contentType, "multipart/form-data") {
			return true
		}
	}
	
	// Check URL patterns that indicate file uploads
	if r.URL.Path == "/api/v1/files" || r.URL.Path == "/api/v1/files/upload" {
		return true
	}
	
	return false
}

// cleanupRoutine periodically removes old limiters to prevent memory leaks
func (lm *LimitsMiddleware) cleanupRoutine() {
	ticker := time.NewTicker(lm.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		lm.cleanup()
	}
}

// cleanup removes limiters that haven't been used recently
func (lm *LimitsMiddleware) cleanup() {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	now := time.Now()
	for userID, limiter := range lm.limiters {
		if now.Sub(limiter.lastSeen) > lm.maxAge {
			delete(lm.limiters, userID)
		}
	}
}

// GetLimiterStats returns statistics about active limiters
func (lm *LimitsMiddleware) GetLimiterStats() map[string]interface{} {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	stats := map[string]interface{}{
		"active_limiters": len(lm.limiters),
		"rate_limit":      float64(lm.rps),
		"burst_size":      lm.burst,
		"cleanup_interval": lm.cleanupInterval.String(),
		"max_age":         lm.maxAge.String(),
	}

	return stats
}

// SetRateLimit updates the rate limit configuration
func (lm *LimitsMiddleware) SetRateLimit(rps float64, burst int) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.rps = rate.Limit(rps)
	lm.burst = burst

	// Update existing limiters with new rate
	for _, limiter := range lm.limiters {
		limiter.limiter.SetLimit(lm.rps)
		limiter.limiter.SetBurst(lm.burst)
	}
}

// ResetUserLimit resets rate limit for a specific user
func (lm *LimitsMiddleware) ResetUserLimit(userID uuid.UUID) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	delete(lm.limiters, userID)
}

// GetUserLimitStatus returns rate limit status for a user
func (lm *LimitsMiddleware) GetUserLimitStatus(userID uuid.UUID) map[string]interface{} {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	limiter, exists := lm.limiters[userID]
	if !exists {
		return map[string]interface{}{
			"has_limiter": false,
			"limit":       float64(lm.rps),
			"burst":       lm.burst,
		}
	}

	return map[string]interface{}{
		"has_limiter":    true,
		"limit":          float64(lm.rps),
		"burst":          lm.burst,
		"tokens":         limiter.limiter.Tokens(),
		"last_seen":      limiter.lastSeen,
	}
}

// DefaultQuotaService provides a basic quota service implementation
type DefaultQuotaService struct {
	db interface{} // Database connection - would be *sql.DB in real implementation
}

// GetUserQuotaUsage returns user's quota usage (placeholder implementation)
func (dqs *DefaultQuotaService) GetUserQuotaUsage(userID uuid.UUID) (used, quota int64, err error) {
	// This would be implemented to query the database
	// For now, return defaults
	return 0, 10*1024*1024, nil // 10MB default quota
}

// CheckUserQuota checks if user has enough quota (placeholder implementation)
func (dqs *DefaultQuotaService) CheckUserQuota(userID uuid.UUID, additionalSize int64) (bool, error) {
	used, quota, err := dqs.GetUserQuotaUsage(userID)
	if err != nil {
		return false, err
	}
	
	return (used + additionalSize) <= quota, nil
}