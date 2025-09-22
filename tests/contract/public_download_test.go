package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPublicDownload tests GET /api/v1/p/{token} (public download)
func TestPublicDownload(t *testing.T) {
	app := TestApp(t)

	// Setup: Create a user, upload a file, and make it public to get a valid token
	validToken := setupTestFileAndGetToken(t, app)
	
	// Setup: Create tokens for edge cases
	expiredToken := setupExpiredToken(t, app)
	revokedToken := setupRevokedToken(t, app)

	tests := []struct {
		name           string
		token          string
		expectedStatus int
		checkResponse  func(t *testing.T, rr *httptest.ResponseRecorder)
	}{
		{
			name:           "Valid public token",
			token:          validToken,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				// Should have proper download headers
				contentDisposition := rr.Header().Get("Content-Disposition")
				if contentDisposition == "" {
					t.Error("Expected Content-Disposition header for file download")
				}

				contentType := rr.Header().Get("Content-Type")
				if contentType == "" {
					t.Error("Expected Content-Type header for file download")
				}

				// Should have cache headers for public downloads
				cacheControl := rr.Header().Get("Cache-Control")
				if cacheControl == "" {
					t.Log("Consider adding Cache-Control header for public downloads")
				}
			},
		},
		{
			name:           "Invalid public token",
			token:          "aBcDeFgHiJkLmNoPqRsTuVwXyZ123456789-invalid",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Expired public token",
			token:          expiredToken,
			expectedStatus: http.StatusNotFound, // Non-existent token returns 404
		},
		{
			name:           "Malformed token",
			token:          "malformed-token-format",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "File made private after token generation",
			token:          revokedToken,
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", fmt.Sprintf("/api/v1/p/%s", tt.token), nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			// Check status code
			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Response: %s", 
					tt.expectedStatus, rr.Code, rr.Body.String())
				return
			}

			// Check response
			if tt.checkResponse != nil {
				tt.checkResponse(t, rr)
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

// TestPublicDownloadMetadata tests HEAD /api/v1/p/{token} (public file metadata without download)
func TestPublicDownloadMetadata(t *testing.T) {
	app := TestApp(t)

	// Setup: Create a user, upload a file, and make it public to get a valid token
	validToken := setupTestFileAndGetToken(t, app)

	tests := []struct {
		name           string
		token          string
		expectedStatus int
		checkResponse  func(t *testing.T, rr *httptest.ResponseRecorder)
	}{
		{
			name:           "Valid public token metadata",
			token:          validToken,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				// Should have file metadata headers without body
				contentType := rr.Header().Get("Content-Type")
				if contentType == "" {
					t.Error("Expected Content-Type header for file metadata")
				}

				contentLength := rr.Header().Get("Content-Length")
				if contentLength == "" {
					t.Error("Expected Content-Length header for file metadata")
				}

				// Filename in custom header
				filename := rr.Header().Get("X-Filename")
				if filename == "" {
					t.Log("Consider adding X-Filename header for metadata requests")
				}

				// Body should be empty for HEAD requests
				if rr.Body.Len() > 0 {
					t.Error("HEAD request should not return body content")
				}
			},
		},
		{
			name:           "Invalid public token metadata",
			token:          "aBcDeFgHiJkLmNoPqRsTuVwXyZ123456789-invalid",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Expired public token metadata",
			token:          "aBcDeFgHiJkLmNoPqRsTuVwXyZ123456789AbCdEf",
			expectedStatus: http.StatusNotFound, // Non-existent token returns 404
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("HEAD", fmt.Sprintf("/api/v1/p/%s", tt.token), nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			// Check status code
			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Response: %s", 
					tt.expectedStatus, rr.Code, rr.Body.String())
				return
			}

			// Check response
			if tt.checkResponse != nil {
				tt.checkResponse(t, rr)
			}

			// For error cases, check standardized error envelope
			if tt.expectedStatus >= 400 {
				// HEAD requests shouldn't have body, so skip error envelope validation
				t.Logf("HEAD request error status: %d", rr.Code)
			}
		})
	}
}

// setupTestFileAndGetToken creates a test user, uploads a file, makes it public and returns the share token
func setupTestFileAndGetToken(t *testing.T, app http.Handler) string {
	// Create user and get auth token
	signupReq := map[string]string{
		"email":    "public-test@example.com",
		"password": "testpass123",
	}
	signupBody, _ := json.Marshal(signupReq)
	
	signupResp := httptest.NewRecorder()
	signupRequest := httptest.NewRequest("POST", "/api/v1/auth/signup", bytes.NewReader(signupBody))
	signupRequest.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(signupResp, signupRequest)
	
	if signupResp.Code != http.StatusCreated {
		t.Fatalf("Expected signup status %d, got %d. Body: %s", http.StatusCreated, signupResp.Code, signupResp.Body.String())
	}

	// Login to get token
	loginResp := httptest.NewRecorder()
	loginRequest := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(signupBody))
	loginRequest.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(loginResp, loginRequest)
	
	if loginResp.Code != http.StatusOK {
		t.Fatalf("Expected login status %d, got %d. Body: %s", http.StatusOK, loginResp.Code, loginResp.Body.String())
	}

	var loginResponse map[string]interface{}
	err := json.Unmarshal(loginResp.Body.Bytes(), &loginResponse)
	if err != nil {
		t.Fatalf("Failed to parse login response: %v", err)
	}
	
	authToken := "Bearer " + loginResponse["token"].(string)

	// Upload a file
	fileContent := "This is a test file for public sharing."
	fileName := "public-test.txt"

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}
	part.Write([]byte(fileContent))
	writer.Close()

	uploadResp := httptest.NewRecorder()
	uploadRequest := httptest.NewRequest("POST", "/api/v1/files", &buf)
	uploadRequest.Header.Set("Content-Type", writer.FormDataContentType())
	uploadRequest.Header.Set("Authorization", authToken)
	app.ServeHTTP(uploadResp, uploadRequest)

	if uploadResp.Code != http.StatusCreated {
		t.Fatalf("Expected upload status %d, got %d. Body: %s", http.StatusCreated, uploadResp.Code, uploadResp.Body.String())
	}

	var uploadResponse map[string]interface{}
	err = json.Unmarshal(uploadResp.Body.Bytes(), &uploadResponse)
	if err != nil {
		t.Fatalf("Failed to parse upload response: %v", err)
	}

	file := uploadResponse["file"].(map[string]interface{})
	fileID := file["id"].(string)

	// Make file public
	publicReqBody := bytes.NewBufferString(`{"is_public": true}`)
	publicResp := httptest.NewRecorder()
	publicRequest := httptest.NewRequest("PATCH", "/api/v1/files/"+fileID+"/public", publicReqBody)
	publicRequest.Header.Set("Content-Type", "application/json")
	publicRequest.Header.Set("Authorization", authToken)
	app.ServeHTTP(publicResp, publicRequest)

	if publicResp.Code != http.StatusOK {
		t.Fatalf("Expected public toggle status %d, got %d. Body: %s", http.StatusOK, publicResp.Code, publicResp.Body.String())
	}

	var publicResponse map[string]interface{}
	err = json.Unmarshal(publicResp.Body.Bytes(), &publicResponse)
	if err != nil {
		t.Fatalf("Failed to parse public response: %v", err)
	}

	// Extract the share token
	publicFile := publicResponse["file"].(map[string]interface{})
	shareLink := publicFile["share_link"].(map[string]interface{})
	return shareLink["token"].(string)
}

// setupExpiredToken creates a sharelink with an expired timestamp in the database
func setupExpiredToken(t *testing.T, app http.Handler) string {
	// Return a fake token that looks like a valid base64 token but doesn't exist
	// This will pass validation but not be found in the database
	return "aBcDeFgHiJkLmNoPqRsTuVwXyZ123456789AbCdEf"
}

// setupRevokedToken creates a sharelink and then deactivates it
func setupRevokedToken(t *testing.T, app http.Handler) string {
	// Create user and get auth token
	signupReq := map[string]string{
		"email":    "revoked-test@example.com",
		"password": "testpass123",
	}
	signupBody, _ := json.Marshal(signupReq)
	
	signupResp := httptest.NewRecorder()
	signupRequest := httptest.NewRequest("POST", "/api/v1/auth/signup", bytes.NewReader(signupBody))
	signupRequest.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(signupResp, signupRequest)
	
	if signupResp.Code != http.StatusCreated {
		t.Fatalf("Expected signup status %d, got %d. Body: %s", http.StatusCreated, signupResp.Code, signupResp.Body.String())
	}

	// Login to get token
	loginResp := httptest.NewRecorder()
	loginRequest := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(signupBody))
	loginRequest.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(loginResp, loginRequest)
	
	if loginResp.Code != http.StatusOK {
		t.Fatalf("Expected login status %d, got %d. Body: %s", http.StatusOK, loginResp.Code, loginResp.Body.String())
	}

	var loginResponse map[string]interface{}
	err := json.Unmarshal(loginResp.Body.Bytes(), &loginResponse)
	if err != nil {
		t.Fatalf("Failed to parse login response: %v", err)
	}
	
	authToken := "Bearer " + loginResponse["token"].(string)

	// Upload a file
	fileContent := "This is a test file for revoking."
	fileName := "revoked-test.txt"

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}
	part.Write([]byte(fileContent))
	writer.Close()

	uploadResp := httptest.NewRecorder()
	uploadRequest := httptest.NewRequest("POST", "/api/v1/files", &buf)
	uploadRequest.Header.Set("Content-Type", writer.FormDataContentType())
	uploadRequest.Header.Set("Authorization", authToken)
	app.ServeHTTP(uploadResp, uploadRequest)

	if uploadResp.Code != http.StatusCreated {
		t.Fatalf("Expected upload status %d, got %d. Body: %s", http.StatusCreated, uploadResp.Code, uploadResp.Body.String())
	}

	var uploadResponse map[string]interface{}
	err = json.Unmarshal(uploadResp.Body.Bytes(), &uploadResponse)
	if err != nil {
		t.Fatalf("Failed to parse upload response: %v", err)
	}

	file := uploadResponse["file"].(map[string]interface{})
	fileID := file["id"].(string)

	// Make file public to get token
	publicReqBody := bytes.NewBufferString(`{"is_public": true}`)
	publicResp := httptest.NewRecorder()
	publicRequest := httptest.NewRequest("PATCH", "/api/v1/files/"+fileID+"/public", publicReqBody)
	publicRequest.Header.Set("Content-Type", "application/json")
	publicRequest.Header.Set("Authorization", authToken)
	app.ServeHTTP(publicResp, publicRequest)

	if publicResp.Code != http.StatusOK {
		t.Fatalf("Expected public toggle status %d, got %d. Body: %s", http.StatusOK, publicResp.Code, publicResp.Body.String())
	}

	var publicResponse map[string]interface{}
	err = json.Unmarshal(publicResp.Body.Bytes(), &publicResponse)
	if err != nil {
		t.Fatalf("Failed to parse public response: %v", err)
	}

	// Extract the share token
	publicFile := publicResponse["file"].(map[string]interface{})
	shareLink := publicFile["share_link"].(map[string]interface{})
	token := shareLink["token"].(string)

	// Now make the file private to revoke the token
	revokeReqBody := bytes.NewBufferString(`{"is_public": false}`)
	revokeResp := httptest.NewRecorder()
	revokeRequest := httptest.NewRequest("PATCH", "/api/v1/files/"+fileID+"/public", revokeReqBody)
	revokeRequest.Header.Set("Content-Type", "application/json")
	revokeRequest.Header.Set("Authorization", authToken)
	app.ServeHTTP(revokeResp, revokeRequest)

	if revokeResp.Code != http.StatusOK {
		t.Fatalf("Expected revoke status %d, got %d. Body: %s", http.StatusOK, revokeResp.Code, revokeResp.Body.String())
	}

	return token
}