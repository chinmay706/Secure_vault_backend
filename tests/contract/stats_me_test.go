package contract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestStatsMe tests GET /api/v1/stats/me
func TestStatsMe(t *testing.T) {
	app := TestApp(t)

	// Create a test user and get authentication token
	token := createTestUserAndGetToken(t, app, "stats@example.com", "testpass123")

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name:           "Valid user stats request",
			authHeader:     "Bearer " + token,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				if err != nil {
					t.Errorf("Failed to parse JSON response: %v", err)
					return
				}

				// Check for required stats fields
				expectedFields := []string{
					"total_files", "total_size_bytes", "quota_bytes", "quota_used_bytes",
					"quota_available_bytes", "files_by_type", "upload_history",
				}
				for _, field := range expectedFields {
					if _, exists := response[field]; !exists {
						t.Errorf("Response should contain %s field", field)
					}
				}

				// Validate data types
				if totalFiles, exists := response["total_files"]; exists {
					if _, ok := totalFiles.(float64); !ok {
						t.Error("total_files should be a number")
					}
				}

				if totalSize, exists := response["total_size_bytes"]; exists {
					if _, ok := totalSize.(float64); !ok {
						t.Error("total_size_bytes should be a number")
					}
				}

				if quotaBytes, exists := response["quota_bytes"]; exists {
					if _, ok := quotaBytes.(float64); !ok {
						t.Error("quota_bytes should be a number")
					}
				}

				// Validate files_by_type structure
				if filesByType, exists := response["files_by_type"]; exists {
					if filesByTypeMap, ok := filesByType.(map[string]interface{}); ok {
						// Should contain MIME type counts
						for mimeType, count := range filesByTypeMap {
							if mimeType == "" {
								t.Error("MIME type should be a non-empty string")
							}
							if _, ok := count.(float64); !ok {
								t.Errorf("File count for MIME type %s should be a number", mimeType)
							}
						}
					} else {
						t.Error("files_by_type should be an object")
					}
				}

				// Validate upload_history structure  
				if uploadHistory, exists := response["upload_history"]; exists {
					if historyArray, ok := uploadHistory.([]interface{}); ok {
						for i, entry := range historyArray {
							if entryMap, ok := entry.(map[string]interface{}); ok {
								expectedHistoryFields := []string{"date", "count", "total_size"}
								for _, field := range expectedHistoryFields {
									if _, exists := entryMap[field]; !exists {
										t.Errorf("Upload history entry %d should contain %s field", i, field)
									}
								}
							} else {
								t.Errorf("Upload history entry %d should be an object", i)
							}
						}
					} else {
						t.Error("upload_history should be an array")
					}
				}
			},
		},
		{
			name:           "Unauthorized access",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Invalid JWT token",
			authHeader:     "Bearer invalid-token",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Malformed Authorization header",
			authHeader:     "InvalidFormat token",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/api/v1/stats/me", nil)
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

// TestStatsValidation tests T009a - stats validation and error handling
func TestStatsValidation(t *testing.T) {
	app := TestApp(t)

	// Create a test user and get authentication token
	token := createTestUserAndGetToken(t, app, "statsvalidation@example.com", "testpass123")

	t.Run("Stats with query parameters", func(t *testing.T) {
		tests := []struct {
			name           string
			queryParams    string
			authHeader     string
			expectedStatus int
			expectedCode   string
		}{
			{
				name:           "Valid date range filter",
				queryParams:    "?from=2024-01-01&to=2024-12-31",
				authHeader:     "Bearer " + token,
				expectedStatus: http.StatusOK,
			},
			{
				name:           "Invalid from date format",
				queryParams:    "?from=invalid-date",
				authHeader:     "Bearer " + token,
				expectedStatus: http.StatusBadRequest,
				expectedCode:   "INVALID_DATE_FORMAT",
			},
			{
				name:           "Invalid to date format",
				queryParams:    "?to=not-a-date",
				authHeader:     "Bearer " + token,
				expectedStatus: http.StatusBadRequest,
				expectedCode:   "INVALID_DATE_FORMAT",
			},
			{
				name:           "From date after to date",
				queryParams:    "?from=2024-12-01&to=2024-01-01",
				authHeader:     "Bearer " + token,
				expectedStatus: http.StatusBadRequest,
				expectedCode:   "INVALID_DATE_RANGE",
			},
			{
				name:           "Date range too large",
				queryParams:    "?from=2020-01-01&to=2024-12-31",
				authHeader:     "Bearer " + token,
				expectedStatus: http.StatusBadRequest,
				expectedCode:   "DATE_RANGE_TOO_LARGE",
			},
			{
				name:           "Valid grouping parameter",
				queryParams:    "?group_by=month",
				authHeader:     "Bearer " + token,
				expectedStatus: http.StatusOK,
			},
			{
				name:           "Invalid grouping parameter",
				queryParams:    "?group_by=invalid",
				authHeader:     "Bearer " + token,
				expectedStatus: http.StatusBadRequest,
				expectedCode:   "INVALID_GROUPING",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req, err := http.NewRequest("GET", "/api/v1/stats/me"+tt.queryParams, nil)
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

				// Verify standardized error envelope for validation errors
				if tt.expectedStatus >= 400 {
					var response map[string]interface{}
					err := json.Unmarshal(rr.Body.Bytes(), &response)
					if err != nil {
						t.Errorf("Failed to parse JSON response: %v", err)
						return
					}

					if errorData, exists := response["error"]; !exists {
						t.Error("Response should contain error field")
					} else if errorMap, ok := errorData.(map[string]interface{}); !ok {
						t.Error("Error should be an object")
					} else {
						// Check error code if specified
						if tt.expectedCode != "" {
							if code, exists := errorMap["code"]; !exists {
								t.Error("Error should contain code field")
							} else if codeStr, ok := code.(string); !ok {
								t.Error("Error code should be a string")
							} else if codeStr != tt.expectedCode {
								t.Errorf("Expected error code %s, got %s", tt.expectedCode, codeStr)
							}
						}

						if message, exists := errorMap["message"]; !exists {
							t.Error("Error should contain message field")
						} else if _, ok := message.(string); !ok {
							t.Error("Error message should be a string")
						}
					}
				}
			})
		}
	})

	t.Run("Rate limiting for stats endpoint", func(t *testing.T) {
		// Test rate limiting (assuming stats might be expensive)
		authHeader := "Bearer " + token
		
		// Make multiple rapid requests
		for i := 0; i < 10; i++ {
			req, err := http.NewRequest("GET", "/api/v1/stats/me", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", authHeader)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			// Should eventually hit rate limit (429) if implemented
			if rr.Code == http.StatusTooManyRequests {
				// Check for rate limit error envelope
				var response map[string]interface{}
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				if err == nil {
					if errorData, exists := response["error"]; exists {
						if errorMap, ok := errorData.(map[string]interface{}); ok {
							if code, exists := errorMap["code"]; exists {
								if codeStr, ok := code.(string); ok && codeStr == "RATE_LIMIT_EXCEEDED" {
									t.Logf("Rate limiting working correctly on request %d", i+1)
									return
								}
							}
						}
					}
				}
			}
		}
		
		t.Log("Rate limiting test completed (may not be triggered in test environment)")
	})
}

// createTestUserAndGetToken creates a test user and returns their JWT token
func createTestUserAndGetToken(t *testing.T, app http.Handler, email, password string) string {
	t.Helper()

	// Create signup request
	signupReq := map[string]string{
		"email":    email,
		"password": password,
	}
	signupBody, err := json.Marshal(signupReq)
	if err != nil {
		t.Fatalf("Failed to marshal signup request: %v", err)
	}

	// Signup
	signupResp := httptest.NewRecorder()
	signupRequest := httptest.NewRequest("POST", "/api/v1/auth/signup", bytes.NewReader(signupBody))
	signupRequest.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(signupResp, signupRequest)

	if signupResp.Code != http.StatusCreated {
		t.Fatalf("Expected signup status %d, got %d. Response: %s", http.StatusCreated, signupResp.Code, signupResp.Body.String())
	}

	// Login to get token
	loginResp := httptest.NewRecorder()
	loginRequest := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(signupBody))
	loginRequest.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(loginResp, loginRequest)

	if loginResp.Code != http.StatusOK {
		t.Fatalf("Expected login status %d, got %d. Response: %s", http.StatusOK, loginResp.Code, loginResp.Body.String())
	}

	var loginResponse map[string]interface{}
	err = json.Unmarshal(loginResp.Body.Bytes(), &loginResponse)
	if err != nil {
		t.Fatalf("Failed to parse login response: %v", err)
	}

	token, exists := loginResponse["token"].(string)
	if !exists || token == "" {
		t.Fatal("Token should not be empty")
	}

	return token
}