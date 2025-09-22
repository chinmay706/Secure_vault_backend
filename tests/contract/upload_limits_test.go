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
	"strings"
	"testing"
)

// TestUploadLimits tests the upload size limits and returns 413 with standardized error envelope
func TestUploadLimits(t *testing.T) {
	app := TestApp(t)

	// Setup authentication
	var authToken string

	// Sign up first
	{
		signupData := map[string]string{
			"email":    "upload-test@example.com",
			"password": "testpass123",
		}
		signupJSON, _ := json.Marshal(signupData)

		req, _ := http.NewRequest("POST", "/api/v1/auth/signup", bytes.NewBuffer(signupJSON))
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		var signupResponse map[string]interface{}
		json.Unmarshal(rr.Body.Bytes(), &signupResponse)
		authToken = signupResponse["token"].(string)
	}

	t.Run("upload file exceeding 10MB limit returns 413", func(t *testing.T) {
		// Create a large file content (simulate >11MB using actual test file + padding)
		sampleFilePath := filepath.Join("..", "..", "test-files", "sample.txt")
		sampleContent, err := os.ReadFile(sampleFilePath)
		if err != nil {
			t.Fatalf("Failed to read sample file: %v", err)
		}

		// Pad the sample content to exceed 10MB
		largeContent := string(sampleContent) + strings.Repeat("A", 11*1024*1024) // Add 11MB of padding

		var b bytes.Buffer
		writer := multipart.NewWriter(&b)

		part, err := writer.CreateFormFile("file", "large_file.txt")
		if err != nil {
			t.Fatalf("Failed to create form file: %v", err)
		}

		_, err = io.WriteString(part, largeContent)
		if err != nil {
			t.Fatalf("Failed to write file content: %v", err)
		}

		err = writer.Close()
		if err != nil {
			t.Fatalf("Failed to close multipart writer: %v", err)
		}

		req, err := http.NewRequest("POST", "/api/v1/files", &b)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Check status code - expect 413
		expectedStatus := http.StatusRequestEntityTooLarge
		if rr.Code != expectedStatus {
			t.Errorf("Expected status %d (413 Payload Too Large), got %d. Response: %s", 
				expectedStatus, rr.Code, rr.Body.String())
			return
		}

		// Verify standardized error envelope
		{
			var response map[string]interface{}
			err := json.Unmarshal(rr.Body.Bytes(), &response)
			if err != nil {
				t.Errorf("Failed to parse JSON response: %v", err)
				return
			}

			// Check standardized error envelope
			if errorData, exists := response["error"]; !exists {
				t.Error("Response should contain error field")
			} else if errorMap, ok := errorData.(map[string]interface{}); ok {
				if code, exists := errorMap["code"]; !exists {
					t.Error("Error should contain code field")
				} else if codeStr, ok := code.(string); ok && codeStr != "FILE_SIZE_EXCEEDED" {
					t.Logf("Expected error code 'FILE_SIZE_EXCEEDED', got '%s'", codeStr)
				}

				if message, exists := errorMap["message"]; !exists {
					t.Error("Error should contain message field")
				} else if messageStr, ok := message.(string); ok {
					if !strings.Contains(strings.ToLower(messageStr), "10") || !strings.Contains(strings.ToLower(messageStr), "mb") {
						t.Logf("Error message should mention 10MB limit, got: %s", messageStr)
					}
				}

				// Optional: check for details field
				if details, exists := errorMap["details"]; exists {
					if detailsMap, ok := details.(map[string]interface{}); ok {
						if maxSize, exists := detailsMap["maxSizeBytes"]; !exists {
							t.Log("Error details could include maxSizeBytes field")
						} else {
							t.Logf("Max size bytes: %v", maxSize)
						}
					}
				}
			}
		}
	})

	t.Run("upload file under 10MB limit should succeed", func(t *testing.T) {
		// Use the actual sample.txt file which should be well under 10MB
		sampleFilePath := filepath.Join("..", "..", "test-files", "sample.txt")
		sampleContent, err := os.ReadFile(sampleFilePath)
		if err != nil {
			t.Fatalf("Failed to read sample file: %v", err)
		}

		var b bytes.Buffer
		writer := multipart.NewWriter(&b)

		part, err := writer.CreateFormFile("file", "sample.txt")
		if err != nil {
			t.Fatalf("Failed to create form file: %v", err)
		}

		_, err = part.Write(sampleContent)
		if err != nil {
			t.Fatalf("Failed to write file content: %v", err)
		}

		err = writer.Close()
		if err != nil {
			t.Fatalf("Failed to close multipart writer: %v", err)
		}

		req, err := http.NewRequest("POST", "/api/v1/files", &b)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Check status code - expect 201 (should succeed)
		expectedStatus := http.StatusCreated
		if rr.Code != expectedStatus {
			t.Errorf("Expected status %d (should succeed for small file), got %d. Response: %s", 
				expectedStatus, rr.Code, rr.Body.String())
		}
	})

	t.Run("multiple files where one exceeds limit returns 413", func(t *testing.T) {
		var b bytes.Buffer
		writer := multipart.NewWriter(&b)

		// Add normal file (actual sample.txt)
		sampleFilePath := filepath.Join("..", "..", "test-files", "sample.txt")
		sampleContent, err := os.ReadFile(sampleFilePath)
		if err != nil {
			t.Fatalf("Failed to read sample file: %v", err)
		}

		part1, err := writer.CreateFormFile("file", "sample.txt")
		if err != nil {
			t.Fatalf("Failed to create form file 1: %v", err)
		}
		part1.Write(sampleContent)

		// Add large file that exceeds limit
		part2, err := writer.CreateFormFile("file", "large_file.txt")
		if err != nil {
			t.Fatalf("Failed to create form file 2: %v", err)
		}
		largeContent := strings.Repeat("C", 11*1024*1024) // 11MB
		io.WriteString(part2, largeContent)

		err = writer.Close()
		if err != nil {
			t.Fatalf("Failed to close multipart writer: %v", err)
		}

		req, err := http.NewRequest("POST", "/api/v1/files", &b)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Check status code - expect 413
		expectedStatus := http.StatusRequestEntityTooLarge
		if rr.Code != expectedStatus {
			t.Errorf("Expected status %d (should reject when any file exceeds limit), got %d. Response: %s", 
				expectedStatus, rr.Code, rr.Body.String())
			return
		}

		// Verify error envelope mentions which file(s) exceeded limit
		{
			var response map[string]interface{}
			err := json.Unmarshal(rr.Body.Bytes(), &response)
			if err == nil {
				if errorData, exists := response["error"]; exists {
					if errorMap, ok := errorData.(map[string]interface{}); ok {
						if details, exists := errorMap["details"]; exists {
							t.Logf("Error details: %v", details)
						}
					}
				}
			}
		}
	})

	t.Run("Content-Length header exceeding limit returns 413", func(t *testing.T) {
		// Test case where Content-Length itself indicates the request is too large
		// Use actual large content to trigger the middleware

		// Create large content that exceeds 10MB
		largeContent := strings.Repeat("X", 12*1024*1024) // 12MB

		var b bytes.Buffer
		writer := multipart.NewWriter(&b)
		
		part, err := writer.CreateFormFile("file", "large_content_length.txt")
		if err != nil {
			t.Fatalf("Failed to create form file: %v", err)
		}
		
		// Write the large content
		_, err = io.WriteString(part, largeContent)
		if err != nil {
			t.Fatalf("Failed to write large content: %v", err)
		}
		
		err = writer.Close()
		if err != nil {
			t.Fatalf("Failed to close multipart writer: %v", err)
		}

		req, err := http.NewRequest("POST", "/api/v1/files", &b)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Check status code - expect 413
		expectedStatus := http.StatusRequestEntityTooLarge
		if rr.Code != expectedStatus {
			t.Errorf("Expected status %d (should reject based on Content-Length), got %d. Response: %s", 
				expectedStatus, rr.Code, rr.Body.String())
		}
	})
}