package contract

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestFileUploadWithRealFile tests file upload using an actual file
func TestFileUploadWithRealFile(t *testing.T) {
	router := TestApp(t)

	// First, create a user and login to get a token
	signupReq := map[string]string{
		"email":    "filereal@example.com",
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

	// Test file upload with real file
	t.Run("Upload Real File", func(t *testing.T) {
		// Get current working directory to find test file
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get working directory: %v", err)
		}
		
		// Path to test file (adjust based on test execution context)
		testFilePath := filepath.Join(wd, "test-files", "sample.txt")
		
		// Check if file exists, if not create it
		if _, err := os.Stat(testFilePath); os.IsNotExist(err) {
			// Create directory if it doesn't exist
			os.MkdirAll(filepath.Dir(testFilePath), 0755)
			
			// Create test file
			testContent := "This is a test file for upload testing\nIt has multiple lines\nAnd some content"
			err = os.WriteFile(testFilePath, []byte(testContent), 0644)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}
		}

		// Open the test file
		file, err := os.Open(testFilePath)
		if err != nil {
			t.Fatalf("Failed to open test file: %v", err)
		}
		defer file.Close()

		// Create multipart form
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		// Create form file part
		part, err := writer.CreateFormFile("file", "sample.txt")
		if err != nil {
			t.Fatalf("Failed to create form file: %v", err)
		}

		// Copy file content to form
		_, err = io.Copy(part, file)
		if err != nil {
			t.Fatalf("Failed to copy file content: %v", err)
		}

		// Add tags
		writer.WriteField("tags", "test,sample,real-file")

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
			// Try to parse error response
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

		fileData := uploadResponse["file"].(map[string]interface{})
		t.Logf("File uploaded successfully: %s", fileData["id"])
		t.Logf("File details: %+v", fileData)

		if fileData["original_filename"] != "sample.txt" {
			t.Errorf("Expected filename 'sample.txt', got %v", fileData["original_filename"])
		}
	})

	// Test file listing after upload
	t.Run("List Files After Upload", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/files", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		t.Logf("List response status: %d", resp.Code)
		t.Logf("List response body: %s", resp.Body.String())

		if resp.Code != http.StatusOK {
			t.Errorf("Expected list status %d, got %d", http.StatusOK, resp.Code)
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
		} else {
			t.Logf("Listed %d files successfully", len(files))
		}
	})
}