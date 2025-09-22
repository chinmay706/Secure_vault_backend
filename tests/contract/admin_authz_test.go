package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAdminAuthorizationErrors tests T010a - admin endpoints reject non-admin users (403) with standardized error envelope
func TestAdminAuthorizationErrors(t *testing.T) {
	app := TestApp(t)

	// Create a valid regular user token for testing authorization
	userToken := createTestUserAndGetToken(t, app, "regularuser@example.com", "userpass123")

	tests := []struct {
		name           string
		method         string
		url            string
		authHeader     string
		expectedStatus int
		expectedCode   string
		description    string
	}{
		{
			name:           "Regular user accessing admin files",
			method:         "GET",
			url:            "/api/v1/admin/files",
			authHeader:     "Bearer " + userToken,
			expectedStatus: http.StatusForbidden,
			expectedCode:   "ADMIN_REQUIRED",
			description:    "Non-admin users should be forbidden from accessing admin file list",
		},
		{
			name:           "Regular user accessing admin stats",
			method:         "GET",
			url:            "/api/v1/admin/stats",
			authHeader:     "Bearer " + userToken, 
			expectedStatus: http.StatusForbidden,
			expectedCode:   "ADMIN_REQUIRED",
			description:    "Non-admin users should be forbidden from accessing admin stats",
		},
		{
			name:           "Regular user trying to delete files as admin",
			method:         "DELETE",
			url:            "/api/v1/admin/files/some-file-id",
			authHeader:     "Bearer " + userToken,
			expectedStatus: http.StatusForbidden,
			expectedCode:   "ADMIN_REQUIRED",
			description:    "Non-admin users should be forbidden from admin file deletion",
		},
		{
			name:           "Regular user trying to update user quotas",
			method:         "PATCH",
			url:            "/api/v1/admin/users/12345/quota",
			authHeader:     "Bearer " + userToken,
			expectedStatus: http.StatusForbidden,
			expectedCode:   "ADMIN_REQUIRED",
			description:    "Non-admin users should be forbidden from modifying user quotas",
		},
		{
			name:           "Regular user trying to suspend users",
			method:         "POST", 
			url:            "/api/v1/admin/users/12345/suspend",
			authHeader:     "Bearer " + userToken,
			expectedStatus: http.StatusForbidden,
			expectedCode:   "ADMIN_REQUIRED",
			description:    "Non-admin users should be forbidden from suspending users",
		},
		{
			name:           "Invalid admin token",
			method:         "GET",
			url:            "/api/v1/admin/files",
			authHeader:     "Bearer invalid-admin-token",
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "UNAUTHORIZED",
			description:    "Invalid admin tokens should result in unauthorized error",
		},
		{
			name:           "Expired admin token", 
			method:         "GET",
			url:            "/api/v1/admin/stats",
			authHeader:     "Bearer expired-admin-token",
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "UNAUTHORIZED",
			description:    "Expired admin tokens should result in unauthorized error",
		},
		{
			name:           "Missing authorization for admin endpoint",
			method:         "GET",
			url:            "/api/v1/admin/files",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "UNAUTHORIZED",
			description:    "Missing authorization should result in unauthorized error",
		},
		{
			name:           "Malformed admin authorization header",
			method:         "GET",
			url:            "/api/v1/admin/stats",
			authHeader:     "InvalidFormat admin-token",
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "UNAUTHORIZED",
			description:    "Malformed authorization headers should result in unauthorized error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.url, nil)
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

			// Verify standardized error envelope with specific error codes
			if tt.expectedStatus >= 400 {
				var response map[string]interface{}
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				if err != nil {
					t.Errorf("Failed to parse JSON response: %v", err)
					return
				}

				// Verify error envelope structure
				if errorData, exists := response["error"]; !exists {
					t.Error("Response should contain error field")
				} else if errorMap, ok := errorData.(map[string]interface{}); !ok {
					t.Error("Error should be an object")
				} else {
					// Check required error envelope fields
					if code, exists := errorMap["code"]; !exists {
						t.Error("Error should contain code field")
					} else if codeStr, ok := code.(string); !ok {
						t.Error("Error code should be a string")
					} else if codeStr != tt.expectedCode {
						t.Errorf("Expected error code %s, got %s. %s", tt.expectedCode, codeStr, tt.description)
					}

					if message, exists := errorMap["message"]; !exists {
						t.Error("Error should contain message field")
					} else if messageStr, ok := message.(string); !ok {
						t.Error("Error message should be a string")
					} else if messageStr == "" {
						t.Error("Error message should not be empty")
					}

					// Details field is optional but should be object if present
					if details, exists := errorMap["details"]; exists {
						if _, ok := details.(map[string]interface{}); !ok {
							t.Error("Error details should be an object if present")
						}
					}
				}

				// Additional validation for forbidden errors (403)
				if tt.expectedStatus == http.StatusForbidden {
					// Should clearly indicate admin privileges required
					if errorData, exists := response["error"]; exists {
						if errorMap, ok := errorData.(map[string]interface{}); ok {
							if message, exists := errorMap["message"]; exists {
								if messageStr, ok := message.(string); ok {
									// Message should clearly indicate admin access required
									adminKeywords := []string{"admin", "privilege", "permission", "forbidden"}
									hasAdminKeyword := false
									for _, keyword := range adminKeywords {
										if len(messageStr) > 0 && contains(messageStr, keyword) {
											hasAdminKeyword = true
											break
										}
									}
									if !hasAdminKeyword {
										t.Log("Consider including admin-specific language in 403 error messages")
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

// Helper function to check if string contains substring (case-insensitive)
func contains(str, substr string) bool {
	// Simple case-insensitive contains check
	str = toLower(str)
	substr = toLower(substr)
	return len(str) >= len(substr) && hasSubstring(str, substr)
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

func hasSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// TestAdminRoleValidation tests various admin role validation scenarios
func TestAdminRoleValidation(t *testing.T) {
	app := TestApp(t)

	t.Run("Different user roles accessing admin endpoints", func(t *testing.T) {
		roleTests := []struct {
			name           string
			authHeader     string
			expectedStatus int
			roleType       string
		}{
			{
				name:           "Super admin access",
				authHeader:     "Bearer super-admin-jwt-token",
				expectedStatus: http.StatusOK,
				roleType:       "super_admin",
			},
			{
				name:           "Regular admin access", 
				authHeader:     "Bearer admin-jwt-token",
				expectedStatus: http.StatusOK,
				roleType:       "admin",
			},
			{
				name:           "Moderator access (should be forbidden)",
				authHeader:     "Bearer moderator-jwt-token",
				expectedStatus: http.StatusForbidden,
				roleType:       "moderator",
			},
			{
				name:           "Premium user access (should be forbidden)",
				authHeader:     "Bearer premium-user-jwt-token",
				expectedStatus: http.StatusForbidden,
				roleType:       "premium_user",
			},
			{
				name:           "Regular user access (should be forbidden)",
				authHeader:     "Bearer user-jwt-token",
				expectedStatus: http.StatusForbidden,
				roleType:       "user",
			},
		}

		for _, roleTest := range roleTests {
			t.Run(roleTest.name, func(t *testing.T) {
				req, err := http.NewRequest("GET", "/api/v1/admin/files", nil)
				if err != nil {
					t.Fatal(err)
				}

				req.Header.Set("Authorization", roleTest.authHeader)

				rr := httptest.NewRecorder()
				app.ServeHTTP(rr, req)

				// Check status code
				if rr.Code != roleTest.expectedStatus {
					t.Errorf("Role %s: Expected status %d, got %d", 
						roleTest.roleType, roleTest.expectedStatus, rr.Code)
					return
				}

				// For forbidden roles, check error envelope
				if roleTest.expectedStatus == http.StatusForbidden {
					var response map[string]interface{}
					err := json.Unmarshal(rr.Body.Bytes(), &response)
					if err == nil {
						if errorData, exists := response["error"]; exists {
							if errorMap, ok := errorData.(map[string]interface{}); ok {
								if code, exists := errorMap["code"]; exists {
									if codeStr, ok := code.(string); ok && codeStr == "ADMIN_REQUIRED" {
										t.Logf("Role %s correctly rejected with ADMIN_REQUIRED", roleTest.roleType)
									}
								}
							}
						}
					}
				}
			})
		}
	})
}