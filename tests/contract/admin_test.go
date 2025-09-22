package contract

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"securevault-backend/src/models"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// TestAdminFiles tests GET /api/v1/admin/files
func TestAdminFiles(t *testing.T) {
	app := TestApp(t)

	// Create admin and regular user tokens
	adminToken := createAdminUserAndGetToken(t, app, "admin@example.com", "adminpass123")
	userToken := createTestUserAndGetToken(t, app, "user@example.com", "userpass123")

	tests := []struct {
		name           string
		authHeader     string
		queryParams    string
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name:           "Valid admin files list",
			authHeader:     "Bearer " + adminToken,
			queryParams:    "",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				if err != nil {
					t.Errorf("Failed to parse JSON response: %v", err)
					return
				}

				// Check for files array with admin-specific fields
				if files, exists := response["files"]; !exists {
					t.Error("Response should contain files array")
				} else if filesArray, ok := files.([]interface{}); !ok {
					t.Error("Files should be an array")
				} else if len(filesArray) > 0 {
					// Check first file has admin fields
					if fileMap, ok := filesArray[0].(map[string]interface{}); ok {
						adminFields := []string{"id", "filename", "size", "mime_type", "upload_date", "user_email", "user_id", "is_public", "download_count"}
						for _, field := range adminFields {
							if _, exists := fileMap[field]; !exists {
								t.Errorf("Admin file entry should contain %s field", field)
							}
						}
					}
				}

				// Check for pagination metadata
				if _, exists := response["pagination"]; !exists {
					t.Error("Response should contain pagination metadata")
				}
			},
		},
		{
			name:           "Admin files with user filter",
			authHeader:     "Bearer " + adminToken,
			queryParams:    "?user_id=00000000-0000-0000-0000-000000000000", // Valid UUID format
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Admin files with email filter",
			authHeader:     "Bearer " + adminToken,
			queryParams:    "?user_email=user@example.com",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Admin files with date range",
			authHeader:     "Bearer " + adminToken,
			queryParams:    "?uploaded_after=2024-01-01&uploaded_before=2024-12-31",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Admin files sorted by size",
			authHeader:     "Bearer " + adminToken,
			queryParams:    "?sort=size&order=desc",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Non-admin user access",
			authHeader:     "Bearer " + userToken,
			queryParams:    "",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Unauthorized access",
			authHeader:     "",
			queryParams:    "",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/api/v1/admin/files"+tt.queryParams, nil)
			if err != nil {
				t.Fatal(err)
			}

			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			// Check status code
			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Response: %s", 
					tt.expectedStatus, rr.Code, rr.Body.String())
				return
			}

			// Check response structure
			if tt.checkResponse != nil {
				tt.checkResponse(t, rr.Body.Bytes())
			}

			// For error cases, check standardized error envelope
			if tt.expectedStatus >= 400 {
				var response map[string]interface{}
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				if err == nil {
					if errorData, exists := response["error"]; exists {
						if errorMap, ok := errorData.(map[string]interface{}); ok {
							if _, exists := errorMap["code"]; !exists {
								t.Error("Error should contain code field")
							}
							if _, exists := errorMap["message"]; !exists {
								t.Error("Error should contain message field")
							}
						}
					}
				}
			}
		})
	}
}

// TestAdminStats tests GET /api/v1/admin/stats
func TestAdminStats(t *testing.T) {
	app := TestApp(t)

	// Create admin and regular user tokens
	adminToken := createAdminUserAndGetToken(t, app, "admin2@example.com", "adminpass123")
	userToken := createTestUserAndGetToken(t, app, "user2@example.com", "userpass123")

	tests := []struct {
		name           string
		authHeader     string
		queryParams    string
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name:           "Valid admin stats request",
			authHeader:     "Bearer " + adminToken,
			queryParams:    "",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				if err != nil {
					t.Errorf("Failed to parse JSON response: %v", err)
					return
				}

				// Check for system-wide stats
				expectedFields := []string{
					"total_users", "total_files", "total_size_bytes", "total_quota_bytes", 
					"quota_utilization_percent", "files_by_type", "users_by_registration_date",
					"storage_by_user", "most_active_users",
				}
				for _, field := range expectedFields {
					if _, exists := response[field]; !exists {
						t.Errorf("Response should contain %s field", field)
					}
				}

				// Validate specific structure for storage_by_user
				if storageByUser, exists := response["storage_by_user"]; exists {
					if storageArray, ok := storageByUser.([]interface{}); ok {
						if len(storageArray) > 0 {
							if userEntry, ok := storageArray[0].(map[string]interface{}); ok {
								userFields := []string{"user_id", "user_email", "file_count", "total_size_bytes", "quota_bytes"}
								for _, field := range userFields {
									if _, exists := userEntry[field]; !exists {
										t.Errorf("User storage entry should contain %s field", field)
									}
								}
							}
						}
					} else {
						t.Error("storage_by_user should be an array")
					}
				}
			},
		},
		{
			name:           "Admin stats with time range",
			authHeader:     "Bearer " + adminToken,
			queryParams:    "?from=2024-01-01&to=2024-12-31",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Admin stats grouped by month",
			authHeader:     "Bearer " + adminToken,
			queryParams:    "?group_by=month",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Non-admin user access",
			authHeader:     "Bearer " + userToken,
			queryParams:    "",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Unauthorized access",
			authHeader:     "",
			queryParams:    "",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/api/v1/admin/stats"+tt.queryParams, nil)
			if err != nil {
				t.Fatal(err)
			}

			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			// Check status code
			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Response: %s", 
					tt.expectedStatus, rr.Code, rr.Body.String())
				return
			}

			// Check response structure
			if tt.checkResponse != nil {
				tt.checkResponse(t, rr.Body.Bytes())
			}

			// For error cases, check standardized error envelope
			if tt.expectedStatus >= 400 {
				var response map[string]interface{}
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				if err == nil {
					if errorData, exists := response["error"]; exists {
						if errorMap, ok := errorData.(map[string]interface{}); ok {
							if _, exists := errorMap["code"]; !exists {
								t.Error("Error should contain code field")
							}
							if _, exists := errorMap["message"]; !exists {
								t.Error("Error should contain message field")
							}
						}
					}
				}
			}
		})
	}
}

// TestAdminActions tests admin-specific actions like DELETE /api/v1/admin/files/{id}
func TestAdminActions(t *testing.T) {
	app := TestApp(t)

	// Create admin and regular user tokens
	adminToken := createAdminUserAndGetToken(t, app, "admin3@example.com", "adminpass123")
	userToken := createTestUserAndGetToken(t, app, "user3@example.com", "userpass123")
	tests := []struct {
		name           string
		method         string
		url            string
		authHeader     string
		requestBody    string
		expectedStatus int
	}{
		{
			name:           "Admin delete user file",
			method:         "DELETE",
			url:            "/api/v1/admin/files/user-file-id",
			authHeader:     "Bearer " + adminToken,
			expectedStatus: http.StatusNoContent,
		},
		{
			name:           "Admin update user quota",
			method:         "PATCH",
			url:            "/api/v1/admin/users/12345/quota",
			authHeader:     "Bearer " + adminToken,
			requestBody:    `{"quota_bytes": 52428800}`,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Admin suspend user",
			method:         "POST",
			url:            "/api/v1/admin/users/12345/suspend",
			authHeader:     "Bearer " + adminToken,
			requestBody:    `{"reason": "Terms violation"}`,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Non-admin user trying admin action",
			method:         "DELETE",
			url:            "/api/v1/admin/files/user-file-id",
			authHeader:     "Bearer " + userToken,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Unauthorized admin action",
			method:         "DELETE",
			url:            "/api/v1/admin/files/user-file-id",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			var err error

			if tt.requestBody != "" {
				req, err = http.NewRequest(tt.method, tt.url, strings.NewReader(tt.requestBody))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Content-Type", "application/json")
			} else {
				req, err = http.NewRequest(tt.method, tt.url, nil)
				if err != nil {
					t.Fatal(err)
				}
			}

			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			// Check status code
			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Response: %s", 
					tt.expectedStatus, rr.Code, rr.Body.String())
				return
			}

			// For error cases, check standardized error envelope
			if tt.expectedStatus >= 400 {
				var response map[string]interface{}
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				if err == nil {
					if errorData, exists := response["error"]; exists {
						if errorMap, ok := errorData.(map[string]interface{}); ok {
							// Check for specific admin error codes
							if code, exists := errorMap["code"]; exists {
								if codeStr, ok := code.(string); ok {
									expectedCodes := []string{"FORBIDDEN", "UNAUTHORIZED", "ADMIN_REQUIRED"}
									validCode := false
									for _, expectedCode := range expectedCodes {
										if codeStr == expectedCode {
											validCode = true
											break
										}
									}
									if !validCode && tt.expectedStatus == http.StatusForbidden {
										t.Logf("Admin error code: %s (consider using ADMIN_REQUIRED for non-admin users)", codeStr)
									}
								}
							}
						}
					}
				}
			}
		})
	}
}



// createAdminUserAndGetToken creates an admin user directly in the database and returns their JWT token
func createAdminUserAndGetToken(t *testing.T, app http.Handler, email, password string) string {
	t.Helper()

	// Get database connection from environment to create admin user directly
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		t.Fatal("DB_URL environment variable is required")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	// Create admin user directly in database
	adminID := uuid.New()
	now := time.Now().UTC()
	
	query := `
		INSERT INTO users (id, email, password_hash, role, rate_limit_rps, storage_quota_bytes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	
	_, err = db.Exec(query, adminID, email, string(hashedPassword), models.UserRoleAdmin, 10, 100*1024*1024, now, now)
	if err != nil {
		t.Fatalf("Failed to create admin user: %v", err)
	}

	// Now login to get token
	loginReq := map[string]string{
		"email":    email,
		"password": password,
	}
	loginBody, err := json.Marshal(loginReq)
	if err != nil {
		t.Fatalf("Failed to marshal login request: %v", err)
	}

	loginResp := httptest.NewRecorder()
	loginRequest := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(loginBody))
	loginRequest.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(loginResp, loginRequest)

	if loginResp.Code != http.StatusOK {
		t.Fatalf("Expected admin login status %d, got %d. Response: %s", http.StatusOK, loginResp.Code, loginResp.Body.String())
	}

	var loginResponse map[string]interface{}
	err = json.Unmarshal(loginResp.Body.Bytes(), &loginResponse)
	if err != nil {
		t.Fatalf("Failed to parse admin login response: %v", err)
	}

	token, exists := loginResponse["token"].(string)
	if !exists || token == "" {
		t.Fatal("Admin token should not be empty")
	}

	return token
}