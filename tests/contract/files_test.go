package contract

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestFileUploadAndList tests file upload and listing functionality
func TestFileUploadAndList(t *testing.T) {
	router := TestApp(t)

	// First, create a user and login to get a token
	signupReq := map[string]string{
		"email":    "filetest@example.com",
		"password": "testpass123",
	}
	signupBody, _ := json.Marshal(signupReq)
	
	signupResp := httptest.NewRecorder()
	signupRequest := httptest.NewRequest("POST", "/api/v1/auth/signup", bytes.NewReader(signupBody))
	signupRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(signupResp, signupRequest)
	
	if signupResp.Code != http.StatusCreated {
		t.Fatalf("Expected signup status %d, got %d", http.StatusCreated, signupResp.Code)
	}

	// Login to get token
	loginResp := httptest.NewRecorder()
	loginRequest := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(signupBody))
	loginRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(loginResp, loginRequest)
	
	if loginResp.Code != http.StatusOK {
		t.Fatalf("Expected login status %d, got %d", http.StatusOK, loginResp.Code)
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

	// Test file upload
	t.Run("Upload File", func(t *testing.T) {
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

		// Add tags
		writer.WriteField("tags", "test,sample")

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

		if resp.Code != http.StatusCreated {
			t.Errorf("Expected upload status %d, got %d. Response: %s", http.StatusCreated, resp.Code, resp.Body.String())
			return
		}

		var uploadResponse map[string]interface{}
		err = json.Unmarshal(resp.Body.Bytes(), &uploadResponse)
		if err != nil {
			t.Fatalf("Failed to parse upload response: %v", err)
		}

		file := uploadResponse["file"].(map[string]interface{})
		if file["original_filename"] != "test.txt" {
			t.Errorf("Expected filename 'test.txt', got %v", file["original_filename"])
		}
		
		t.Logf("File uploaded successfully: %s", file["id"])
	})

	// Test file listing
	t.Run("List Files", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/files", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Errorf("Expected list status %d, got %d. Response: %s", http.StatusOK, resp.Code, resp.Body.String())
			return
		}

		var listResponse map[string]interface{}
		err := json.Unmarshal(resp.Body.Bytes(), &listResponse)
		if err != nil {
			t.Fatalf("Failed to parse list response: %v", err)
		}

		files := listResponse["files"].([]interface{})
		if len(files) < 1 {
			t.Error("Expected at least 1 file in list")
		}

		t.Logf("Listed %d files successfully", len(files))
	})
}