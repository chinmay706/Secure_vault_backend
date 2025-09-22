package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPublicSharing tests T013 - toggle public on/off; public link download increments counter; revoke stops
func TestPublicSharing(t *testing.T) {
	app := setupApp(t)

	t.Run("Public sharing integration workflow", func(t *testing.T) {
		authToken := "Bearer integration-test-token"
		var fileID string
		var publicToken string

		// Step 1: Upload a file
		t.Run("Step 1: Upload file for public sharing", func(t *testing.T) {
			fileContent := []byte("This is a test file for public sharing.")
			fileName := "public-share-test.txt"

			// Create multipart form request
			var buf bytes.Buffer
			writer := multipart.NewWriter(&buf)

			part, err := writer.CreateFormFile("file", fileName)
			if err != nil {
				t.Fatal(err)
			}
			_, err = io.Copy(part, bytes.NewReader(fileContent))
			if err != nil {
				t.Fatal(err)
			}

			writer.Close()

			req, err := http.NewRequest("POST", "/api/v1/files/upload", &buf)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", writer.FormDataContentType())
			req.Header.Set("Authorization", authToken)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusCreated {
				t.Errorf("Expected 201 Created for file upload, got %d", rr.Code)
				return
			}

			var response map[string]interface{}
			err = json.Unmarshal(rr.Body.Bytes(), &response)
			if err != nil {
				t.Errorf("Failed to parse response: %v", err)
				return
			}

			if file, exists := response["file"]; exists {
				if fileMap, ok := file.(map[string]interface{}); ok {
					if id, exists := fileMap["id"]; exists {
						if idStr, ok := id.(string); ok {
							fileID = idStr
							t.Logf("File uploaded successfully, ID: %s", fileID)
						}
					}

					// Verify file is initially private
					if isPublic, exists := fileMap["is_public"]; exists {
						if isPublicValue, ok := isPublic.(bool); ok && isPublicValue {
							t.Error("File should be private by default")
						}
					}
				}
			}
		})

		// Step 2: Toggle file to public
		t.Run("Step 2: Toggle file to public", func(t *testing.T) {
			if fileID == "" {
				fileID = "test-file-id" // Fallback for TDD
			}

			reqBody := bytes.NewBufferString(`{"is_public": true}`)
			req, err := http.NewRequest("PATCH", "/api/v1/files/"+fileID+"/public", reqBody)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", authToken)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Expected 200 OK for toggling public, got %d", rr.Code)
				return
			}

			var response map[string]interface{}
			err = json.Unmarshal(rr.Body.Bytes(), &response)
			if err != nil {
				t.Errorf("Failed to parse response: %v", err)
				return
			}

			if file, exists := response["file"]; exists {
				if fileMap, ok := file.(map[string]interface{}); ok {
					// Verify file is now public
					if isPublic, exists := fileMap["is_public"]; exists {
						if isPublicValue, ok := isPublic.(bool); ok {
							if !isPublicValue {
								t.Error("File should be public after toggle")
							}
						}
					}

					// Get public token
					if shareLink, exists := fileMap["share_link"]; exists {
						if shareLinkMap, ok := shareLink.(map[string]interface{}); ok {
							if token, exists := shareLinkMap["token"]; exists {
								if tokenStr, ok := token.(string); ok {
									publicToken = tokenStr
									t.Logf("Public token generated: %s", publicToken)
								}
							}

							// Verify initial download count is 0
							if downloadCount, exists := shareLinkMap["download_count"]; exists {
								if count, ok := downloadCount.(float64); ok {
									if count != 0 {
										t.Error("Initial download count should be 0")
									}
								}
							}
						}
					}
				}
			}
		})

		// Step 3: Download via public link (first time)
		t.Run("Step 3: Download via public link (first download)", func(t *testing.T) {
			if publicToken == "" {
				publicToken = "test-public-token" // Fallback for TDD
			}

			req, err := http.NewRequest("GET", "/api/v1/p/"+publicToken, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Expected 200 OK for public download, got %d", rr.Code)
				return
			}

			// Verify file content is returned
			if rr.Header().Get("Content-Type") == "" {
				t.Error("Content-Type header should be set")
			}

			if rr.Header().Get("Content-Disposition") == "" {
				t.Error("Content-Disposition header should be set for download")
			}

			t.Logf("First public download successful, Content-Length: %s", rr.Header().Get("Content-Length"))
		})

		// Step 4: Verify download counter incremented
		t.Run("Step 4: Verify download counter incremented", func(t *testing.T) {
			if fileID == "" {
				fileID = "test-file-id" // Fallback for TDD
			}

			req, err := http.NewRequest("GET", "/api/v1/files/"+fileID, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", authToken)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Expected 200 OK for file details, got %d", rr.Code)
				return
			}

			var response map[string]interface{}
			err = json.Unmarshal(rr.Body.Bytes(), &response)
			if err != nil {
				t.Errorf("Failed to parse response: %v", err)
				return
			}

			if file, exists := response["file"]; exists {
				if fileMap, ok := file.(map[string]interface{}); ok {
					if shareLink, exists := fileMap["share_link"]; exists {
						if shareLinkMap, ok := shareLink.(map[string]interface{}); ok {
							if downloadCount, exists := shareLinkMap["download_count"]; exists {
								if count, ok := downloadCount.(float64); ok {
									if count != 1 {
										t.Errorf("Download count should be 1 after first download, got %v", count)
									} else {
										t.Logf("Download count correctly incremented to: %v", count)
									}
								}
							}
						}
					}
				}
			}
		})

		// Step 5: Download via public link (second time)
		t.Run("Step 5: Download via public link (second download)", func(t *testing.T) {
			if publicToken == "" {
				publicToken = "test-public-token" // Fallback for TDD
			}

			req, err := http.NewRequest("GET", "/api/v1/p/"+publicToken, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Expected 200 OK for second public download, got %d", rr.Code)
				return
			}

			t.Log("Second public download successful")
		})

		// Step 6: Verify download counter incremented again
		t.Run("Step 6: Verify download counter incremented to 2", func(t *testing.T) {
			if fileID == "" {
				fileID = "test-file-id" // Fallback for TDD
			}

			req, err := http.NewRequest("GET", "/api/v1/files/"+fileID, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", authToken)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Expected 200 OK for file details, got %d", rr.Code)
				return
			}

			var response map[string]interface{}
			err = json.Unmarshal(rr.Body.Bytes(), &response)
			if err != nil {
				t.Errorf("Failed to parse response: %v", err)
				return
			}

			if file, exists := response["file"]; exists {
				if fileMap, ok := file.(map[string]interface{}); ok {
					if shareLink, exists := fileMap["share_link"]; exists {
						if shareLinkMap, ok := shareLink.(map[string]interface{}); ok {
							if downloadCount, exists := shareLinkMap["download_count"]; exists {
								if count, ok := downloadCount.(float64); ok {
									if count != 2 {
										t.Errorf("Download count should be 2 after second download, got %v", count)
									} else {
										t.Logf("Download count correctly incremented to: %v", count)
									}
								}
							}
						}
					}
				}
			}
		})

		// Step 7: Toggle file back to private (revoke public access)
		t.Run("Step 7: Revoke public access", func(t *testing.T) {
			if fileID == "" {
				fileID = "test-file-id" // Fallback for TDD
			}

			reqBody := bytes.NewBufferString(`{"is_public": false}`)
			req, err := http.NewRequest("PATCH", "/api/v1/files/"+fileID+"/public", reqBody)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", authToken)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Expected 200 OK for revoking public access, got %d", rr.Code)
				return
			}

			var response map[string]interface{}
			err = json.Unmarshal(rr.Body.Bytes(), &response)
			if err != nil {
				t.Errorf("Failed to parse response: %v", err)
				return
			}

			if file, exists := response["file"]; exists {
				if fileMap, ok := file.(map[string]interface{}); ok {
					// Verify file is now private
					if isPublic, exists := fileMap["is_public"]; exists {
						if isPublicValue, ok := isPublic.(bool); ok {
							if isPublicValue {
								t.Error("File should be private after revoke")
							}
						}
					}

					// Verify share link is disabled/removed
					if shareLink, exists := fileMap["share_link"]; exists {
						if shareLinkMap, ok := shareLink.(map[string]interface{}); ok {
							if isActive, exists := shareLinkMap["is_active"]; exists {
								if isActiveValue, ok := isActive.(bool); ok && isActiveValue {
									t.Error("Share link should be inactive after revoke")
								}
							}
						}
					}

					t.Log("Public access successfully revoked")
				}
			}
		})

		// Step 8: Attempt to download via revoked public link
		t.Run("Step 8: Attempt download via revoked public link", func(t *testing.T) {
			if publicToken == "" {
				publicToken = "test-public-token" // Fallback for TDD
			}

			req, err := http.NewRequest("GET", "/api/v1/p/"+publicToken, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			// Should return 404 or 403 for revoked link
			if rr.Code != http.StatusNotFound && rr.Code != http.StatusForbidden {
				t.Errorf("Expected 404 Not Found or 403 Forbidden for revoked link, got %d", rr.Code)
				return
			}

			// Verify error response uses standardized envelope
			var response map[string]interface{}
			err = json.Unmarshal(rr.Body.Bytes(), &response)
			if err != nil {
				t.Errorf("Failed to parse response: %v", err)
				return
			}

			if errorObj, exists := response["error"]; exists {
				if errorMap, ok := errorObj.(map[string]interface{}); ok {
					// Should have standardized error structure
					requiredFields := []string{"code", "message"}
					for _, field := range requiredFields {
						if _, exists := errorMap[field]; !exists {
							t.Errorf("Error response should contain %s field", field)
						}
					}

					if code, exists := errorMap["code"]; exists {
						if codeStr, ok := code.(string); ok {
							expectedCodes := []string{"LINK_NOT_FOUND", "LINK_REVOKED", "ACCESS_DENIED"}
							codeFound := false
							for _, expectedCode := range expectedCodes {
								if codeStr == expectedCode {
									codeFound = true
									break
								}
							}
							if !codeFound {
								t.Logf("Error code: %s (should be one of: %v)", codeStr, expectedCodes)
							}
						}
					}
				}
			}

			t.Log("Revoked public link correctly denied access")
		})

		// Additional test: Verify download count persists after revoke
		t.Run("Verify download count persists after revoke", func(t *testing.T) {
			if fileID == "" {
				fileID = "test-file-id" // Fallback for TDD
			}

			req, err := http.NewRequest("GET", "/api/v1/files/"+fileID, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", authToken)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Expected 200 OK for file details, got %d", rr.Code)
				return
			}

			var response map[string]interface{}
			err = json.Unmarshal(rr.Body.Bytes(), &response)
			if err != nil {
				t.Errorf("Failed to parse response: %v", err)
				return
			}

			if file, exists := response["file"]; exists {
				if fileMap, ok := file.(map[string]interface{}); ok {
					if shareLink, exists := fileMap["share_link"]; exists {
						if shareLinkMap, ok := shareLink.(map[string]interface{}); ok {
							if downloadCount, exists := shareLinkMap["download_count"]; exists {
								if count, ok := downloadCount.(float64); ok {
									if count != 2 {
										t.Errorf("Download count should persist at 2 after revoke, got %v", count)
									} else {
										t.Logf("Download count correctly persisted: %v", count)
									}
								}
							}
						}
					}
				}
			}
		})

		// Additional test: Toggle public again after revoke
		t.Run("Toggle public again after revoke", func(t *testing.T) {
			if fileID == "" {
				fileID = "test-file-id" // Fallback for TDD
			}

			reqBody := bytes.NewBufferString(`{"is_public": true}`)
			req, err := http.NewRequest("PATCH", "/api/v1/files/"+fileID+"/public", reqBody)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", authToken)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Expected 200 OK for re-enabling public, got %d", rr.Code)
				return
			}

			var response map[string]interface{}
			err = json.Unmarshal(rr.Body.Bytes(), &response)
			if err != nil {
				t.Errorf("Failed to parse response: %v", err)
				return
			}

			if file, exists := response["file"]; exists {
				if fileMap, ok := file.(map[string]interface{}); ok {
					// Should generate new token or reactivate existing
					if shareLink, exists := fileMap["share_link"]; exists {
						if shareLinkMap, ok := shareLink.(map[string]interface{}); ok {
							if newToken, exists := shareLinkMap["token"]; exists {
								if tokenStr, ok := newToken.(string); ok {
									t.Logf("Public sharing re-enabled with token: %s", tokenStr)
									// Note: Could be same token reactivated or new token generated
								}
							}

							if isActive, exists := shareLinkMap["is_active"]; exists {
								if isActiveValue, ok := isActive.(bool); ok && !isActiveValue {
									t.Error("Share link should be active after re-enabling public")
								}
							}
						}
					}
				}
			}
		})
	})
}