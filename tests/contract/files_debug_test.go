package contract

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestFileUploadDebug tests file upload with detailed error reporting
func TestFileUploadDebug(t *testing.T) {
	router := TestApp(t)

	// First, create a user and login to get a token
	signupReq := map[string]string{
		"email":    "debug@example.com",
		"password": "testpass123",
	}
	signupBody, _ := json.Marshal(signupReq)
	
	signupResp := httptest.NewRecorder()
	signupRequest := httptest.NewRequest("POST", "/api/v1/auth/signup", bytes.NewReader(signupBody))
	signupRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(signupResp, signupRequest)
	
	if signupResp.Code != http.StatusCreated {
		t.Fatalf("Expected signup status %d, got %d. Response: %s", http.StatusCreated, signupResp.Code, signupResp.Body.String())
	}

	// Login to get token
	loginResp := httptest.NewRecorder()
	loginRequest := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(signupBody))
	loginRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(loginResp, loginRequest)
	
	if loginResp.Code != http.StatusOK {
		t.Fatalf("Expected login status %d, got %d. Response: %s", http.StatusOK, loginResp.Code, loginResp.Body.String())
	}

	var loginResponse map[string]interface{}
	err := json.Unmarshal(loginResp.Body.Bytes(), &loginResponse)
	if err != nil {
		t.Fatalf("Failed to parse login response: %v", err)
	}
	
	token := loginResponse["token"].(string)
	if token == "" {
		t.Fatal("Token should not be empty")
	}

	// Test file upload with different scenarios
	t.Run("Upload Text File", func(t *testing.T) {
		// Create a multipart form
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		// Add file content
		fileContent := "This is a test file content"
		part, err := writer.CreateFormFile("file", "test.txt")
		if err != nil {
			t.Fatalf("Failed to create form file: %v", err)
		}
		
		_, err = part.Write([]byte(fileContent))
		if err != nil {
			t.Fatalf("Failed to write file content: %v", err)
		}

		err = writer.Close()
		if err != nil {
			t.Fatalf("Failed to close writer: %v", err)
		}

		// Create request
		req := httptest.NewRequest("POST", "/api/v1/files", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)

		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		t.Logf("Upload response status: %d", resp.Code)
		t.Logf("Upload response body: %s", resp.Body.String())

		if resp.Code != http.StatusCreated {
			// Try to parse the error response to get more details
			var errorResponse map[string]interface{}
			if err := json.Unmarshal(resp.Body.Bytes(), &errorResponse); err == nil {
				if errorDetail, ok := errorResponse["error"].(map[string]interface{}); ok {
					t.Fatalf("Upload failed with code %s: %s", errorDetail["code"], errorDetail["message"])
				}
			}
			t.Fatalf("Expected upload status %d, got %d. Response: %s", http.StatusCreated, resp.Code, resp.Body.String())
		}

		var uploadResponse map[string]interface{}
		err = json.Unmarshal(resp.Body.Bytes(), &uploadResponse)
		if err != nil {
			t.Fatalf("Failed to parse upload response: %v", err)
		}

		file := uploadResponse["file"].(map[string]interface{})
		t.Logf("File uploaded successfully: %s", file["id"])
		t.Logf("File details: %+v", file)
	})

	// Test simple GET request to see if auth works
	t.Run("Test Auth With List", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/files", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		t.Logf("List response status: %d", resp.Code)
		t.Logf("List response body: %s", resp.Body.String())

		if resp.Code != http.StatusOK {
			t.Errorf("Expected list status %d, got %d", http.StatusOK, resp.Code)
		}
	})
}