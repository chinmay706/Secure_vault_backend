package contract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAuthSignup tests the POST /api/v1/auth/signup endpoint contract
func TestAuthSignup(t *testing.T) {
	app := TestApp(t)

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name: "successful signup with valid data",
			requestBody: map[string]interface{}{
				"email":    "test@example.com",
				"password": "securepassword123",
				"name":     "Test User",
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				if err != nil {
					t.Errorf("Failed to parse JSON response: %v", err)
					return
				}

				// Check for JWT token in response
				if token, exists := response["token"]; !exists || token == "" {
					t.Error("Response should contain a non-empty JWT token")
				}

				// Check for user data
				if user, exists := response["user"]; !exists {
					t.Error("Response should contain user data")
				} else if userMap, ok := user.(map[string]interface{}); ok {
					if email, exists := userMap["email"]; !exists || email != "test@example.com" {
						t.Error("User data should contain correct email")
					}
					if name, exists := userMap["name"]; !exists || name != "Test User" {
						t.Error("User data should contain correct name")
					}
					// Password should NOT be in response
					if _, exists := userMap["password"]; exists {
						t.Error("User data should not contain password")
					}
				}
			},
		},
		{
			name: "signup with missing email - should use standardized error envelope",
			requestBody: map[string]interface{}{
				"password": "securepassword123",
				"name":     "Test User",
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				if err != nil {
					t.Errorf("Failed to parse JSON response: %v", err)
					return
				}

				// Check standardized error envelope
				if errorData, exists := response["error"]; !exists {
					t.Error("Response should contain error field")
				} else if errorMap, ok := errorData.(map[string]interface{}); ok {
					if _, exists := errorMap["code"]; !exists {
						t.Error("Error should contain code field")
					}
					if _, exists := errorMap["message"]; !exists {
						t.Error("Error should contain message field")
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare request body
			requestBodyBytes, err := json.Marshal(tt.requestBody)
			if err != nil {
				t.Fatalf("Failed to marshal request body: %v", err)
			}

			// Create request
			req, err := http.NewRequest("POST", "/api/v1/auth/signup", bytes.NewBuffer(requestBodyBytes))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			// Create response recorder
			rr := httptest.NewRecorder()

			// Execute request
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
		})
	}
}

// TestAuthLogin tests the POST /api/v1/auth/login endpoint contract
func TestAuthLogin(t *testing.T) {
	app := TestApp(t)

	// First, create a user to login with
	signupBody := map[string]interface{}{
		"email":    "test@example.com",
		"password": "securepassword123",
		"name":     "Test User",
	}
	signupBytes, _ := json.Marshal(signupBody)
	signupReq, _ := http.NewRequest("POST", "/api/v1/auth/signup", bytes.NewBuffer(signupBytes))
	signupReq.Header.Set("Content-Type", "application/json")
	signupRr := httptest.NewRecorder()
	app.ServeHTTP(signupRr, signupReq)
	
	if signupRr.Code != http.StatusCreated {
		t.Fatalf("Failed to create test user: %d - %s", signupRr.Code, signupRr.Body.String())
	}

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
	}{
		{
			name: "successful login with valid credentials",
			requestBody: map[string]interface{}{
				"email":    "test@example.com",
				"password": "securepassword123",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "login with invalid credentials should use standardized error envelope",
			requestBody: map[string]interface{}{
				"email":    "test@example.com",
				"password": "wrongpassword",
			},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare request body
			requestBodyBytes, err := json.Marshal(tt.requestBody)
			if err != nil {
				t.Fatalf("Failed to marshal request body: %v", err)
			}

			// Create request
			req, err := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(requestBodyBytes))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			// Create response recorder
			rr := httptest.NewRecorder()

			// Execute request
			app.ServeHTTP(rr, req)

			// Check status code
			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Response: %s", 
					tt.expectedStatus, rr.Code, rr.Body.String())
			}
		})
	}
}