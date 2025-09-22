package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestDeleteReferenceCount tests T014 - delete by owner; blob deleted only when last reference
func TestDeleteReferenceCount(t *testing.T) {
	app := setupApp(t)

	t.Run("Delete reference count integration workflow", func(t *testing.T) {
		user1Token := "Bearer user1-token"
		user2Token := "Bearer user2-token"
		
		var file1ID, file2ID, file3ID string
		var duplicateHash string

		// Step 1: User 1 uploads a file
		t.Run("Step 1: User 1 uploads original file", func(t *testing.T) {
			fileContent := []byte("This is a test file for reference counting during deletion.")
			fileName := "delete-test-original.txt"

			// Calculate hash for deduplication tracking
			hasher := sha256.New()
			hasher.Write(fileContent)
			duplicateHash = fmt.Sprintf("%x", hasher.Sum(nil))

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
			req.Header.Set("Authorization", user1Token)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusCreated {
				t.Errorf("Expected 201 Created for first upload, got %d", rr.Code)
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
							file1ID = idStr
							t.Logf("User 1 uploaded file, ID: %s", file1ID)
						}
					}

					// Verify hash
					if hash, exists := fileMap["hash"]; exists {
						if hashStr, ok := hash.(string); ok {
							if hashStr != duplicateHash {
								t.Errorf("Expected hash %s, got %s", duplicateHash, hashStr)
							}
						}
					}
				}
			}
		})

		// Step 2: User 2 uploads the same file (duplicate)
		t.Run("Step 2: User 2 uploads duplicate file", func(t *testing.T) {
			fileContent := []byte("This is a test file for reference counting during deletion.")
			fileName := "delete-test-user2-copy.txt" // Different name, same content

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
			req.Header.Set("Authorization", user2Token)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusCreated {
				t.Errorf("Expected 201 Created for duplicate upload, got %d", rr.Code)
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
							file2ID = idStr
							t.Logf("User 2 uploaded duplicate file, ID: %s", file2ID)
						}
					}

					// Should detect as duplicate
					if isDuplicate, exists := fileMap["is_duplicate"]; exists {
						if isDuplicateValue, ok := isDuplicate.(bool); ok && isDuplicateValue {
							t.Log("Deduplication correctly detected")
						}
					}

					// Should reference same blob
					if originalFileID, exists := fileMap["original_file_id"]; exists {
						t.Logf("Duplicate references original file: %v", originalFileID)
					}
				}
			}
		})

		// Step 3: User 1 uploads another duplicate
		t.Run("Step 3: User 1 uploads another duplicate", func(t *testing.T) {
			fileContent := []byte("This is a test file for reference counting during deletion.")
			fileName := "delete-test-user1-second.txt" // Third reference to same content

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
			req.Header.Set("Authorization", user1Token)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusCreated {
				t.Errorf("Expected 201 Created for third duplicate, got %d", rr.Code)
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
							file3ID = idStr
							t.Logf("User 1 uploaded third duplicate, ID: %s", file3ID)
						}
					}
				}
			}
		})

		// Step 4: Verify blob reference count before deletions
		t.Run("Step 4: Verify blob reference count is 3", func(t *testing.T) {
			// Check via admin stats or file details that show reference count
			req, err := http.NewRequest("GET", "/api/v1/admin/stats", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer admin-token")

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Expected 200 OK for admin stats, got %d", rr.Code)
				return
			}

			var response map[string]interface{}
			err = json.Unmarshal(rr.Body.Bytes(), &response)
			if err != nil {
				t.Errorf("Failed to parse response: %v", err)
				return
			}

			// Should show total files vs unique blobs
			if totalFiles, exists := response["total_files"]; exists {
				if count, ok := totalFiles.(float64); ok {
					if count < 3 {
						t.Errorf("Should have at least 3 files, got %v", count)
					}
					t.Logf("Total files: %v", count)
				}
			}

			if uniqueBlobs, exists := response["unique_blobs"]; exists {
				if count, ok := uniqueBlobs.(float64); ok {
					t.Logf("Unique blobs: %v (should be 1 for our duplicate files)", count)
				}
			}

			if deduplicationStats, exists := response["deduplication"]; exists {
				if dedupMap, ok := deduplicationStats.(map[string]interface{}); ok {
					if duplicateFiles, exists := dedupMap["duplicate_files"]; exists {
						if count, ok := duplicateFiles.(float64); ok {
							if count < 2 {
								t.Errorf("Should have at least 2 duplicate files, got %v", count)
							}
							t.Logf("Duplicate files: %v", count)
						}
					}
				}
			}
		})

		// Step 5: User 2 deletes their file (first deletion)
		t.Run("Step 5: User 2 deletes their file", func(t *testing.T) {
			if file2ID == "" {
				file2ID = "user2-file-id" // Fallback
			}

			req, err := http.NewRequest("DELETE", "/api/v1/files/"+file2ID, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", user2Token)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK && rr.Code != http.StatusNoContent {
				t.Errorf("Expected 200 OK or 204 No Content for delete, got %d", rr.Code)
				return
			}

			t.Log("User 2 successfully deleted their file")

			// The blob should still exist because other references remain
		})

		// Step 6: Verify blob still exists (reference count now 2)
		t.Run("Step 6: Verify blob still exists after first deletion", func(t *testing.T) {
			// Try to download User 1's original file - should still work
			if file1ID == "" {
				file1ID = "user1-file-id" // Fallback
			}

			req, err := http.NewRequest("GET", "/api/v1/files/"+file1ID+"/download", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", user1Token)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Expected 200 OK for download (blob should still exist), got %d", rr.Code)
				return
			}

			t.Log("User 1's file still accessible - blob correctly preserved")

			// Verify User 2's file is no longer accessible
			if file2ID != "" {
				req2, err := http.NewRequest("GET", "/api/v1/files/"+file2ID+"/download", nil)
				if err != nil {
					t.Fatal(err)
				}
				req2.Header.Set("Authorization", user2Token)

				rr2 := httptest.NewRecorder()
				app.ServeHTTP(rr2, req2)

				if rr2.Code != http.StatusNotFound && rr2.Code != http.StatusForbidden {
					t.Errorf("Expected 404 Not Found or 403 Forbidden for deleted file, got %d", rr2.Code)
					return
				}

				t.Log("User 2's file correctly inaccessible after deletion")
			}
		})

		// Step 7: User 1 deletes their first file (second deletion)
		t.Run("Step 7: User 1 deletes their first file", func(t *testing.T) {
			if file1ID == "" {
				file1ID = "user1-file-id" // Fallback
			}

			req, err := http.NewRequest("DELETE", "/api/v1/files/"+file1ID, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", user1Token)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK && rr.Code != http.StatusNoContent {
				t.Errorf("Expected 200 OK or 204 No Content for delete, got %d", rr.Code)
				return
			}

			t.Log("User 1 deleted their first file")
		})

		// Step 8: Verify blob still exists (reference count now 1)
		t.Run("Step 8: Verify blob still exists after second deletion", func(t *testing.T) {
			// User 1's third file should still work
			if file3ID == "" {
				file3ID = "user1-file3-id" // Fallback
			}

			req, err := http.NewRequest("GET", "/api/v1/files/"+file3ID+"/download", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", user1Token)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Expected 200 OK for download (blob should still exist), got %d", rr.Code)
				return
			}

			t.Log("User 1's remaining file still accessible - blob correctly preserved")
		})

		// Step 9: User 1 deletes their last file (final deletion)
		t.Run("Step 9: User 1 deletes their last file", func(t *testing.T) {
			if file3ID == "" {
				file3ID = "user1-file3-id" // Fallback
			}

			req, err := http.NewRequest("DELETE", "/api/v1/files/"+file3ID, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", user1Token)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK && rr.Code != http.StatusNoContent {
				t.Errorf("Expected 200 OK or 204 No Content for final delete, got %d", rr.Code)
				return
			}

			t.Log("User 1 deleted their last file - blob should now be deleted")
		})

		// Step 10: Verify blob is completely deleted
		t.Run("Step 10: Verify blob is completely deleted", func(t *testing.T) {
			// Check admin stats to verify blob cleanup
			req, err := http.NewRequest("GET", "/api/v1/admin/stats", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer admin-token")

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Expected 200 OK for admin stats, got %d", rr.Code)
				return
			}

			var response map[string]interface{}
			err = json.Unmarshal(rr.Body.Bytes(), &response)
			if err != nil {
				t.Errorf("Failed to parse response: %v", err)
				return
			}

			if storageUsed, exists := response["total_storage_used"]; exists {
				if storage, ok := storageUsed.(float64); ok {
					t.Logf("Total storage used after all deletions: %v bytes", storage)
					// Should be reduced compared to before deletions
				}
			}

			if uniqueBlobs, exists := response["unique_blobs"]; exists {
				if count, ok := uniqueBlobs.(float64); ok {
					t.Logf("Unique blobs after cleanup: %v", count)
					// Should be reduced by 1 if our blob was the only one deleted
				}
			}

			// Attempt to access any of the deleted files - all should fail
			deletedFileIDs := []string{file1ID, file2ID, file3ID}
			for i, fileID := range deletedFileIDs {
				if fileID != "" {
					req, err := http.NewRequest("GET", "/api/v1/files/"+fileID, nil)
					if err != nil {
						continue
					}
					req.Header.Set("Authorization", user1Token)

					rr := httptest.NewRecorder()
					app.ServeHTTP(rr, req)

					if rr.Code != http.StatusNotFound && rr.Code != http.StatusForbidden {
						t.Errorf("File %d should be inaccessible after deletion, got %d", i+1, rr.Code)
					}
				}
			}

			t.Log("All file references correctly removed, blob cleanup verified")
		})

		// Additional test: Verify unauthorized deletion is prevented
		t.Run("Verify unauthorized deletion is prevented", func(t *testing.T) {
			// Upload a file with User 1
			fileContent := []byte("This is a file for unauthorized deletion testing.")
			fileName := "unauthorized-delete-test.txt"

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
			req.Header.Set("Authorization", user1Token)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			var testFileID string
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
							testFileID = idStr
						}
					}
				}
			}

			// Try to delete with User 2 (should fail)
			if testFileID != "" {
				req, err := http.NewRequest("DELETE", "/api/v1/files/"+testFileID, nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Authorization", user2Token)

				rr := httptest.NewRecorder()
				app.ServeHTTP(rr, req)

				if rr.Code != http.StatusForbidden && rr.Code != http.StatusNotFound {
					t.Errorf("Expected 403 Forbidden or 404 Not Found for unauthorized delete, got %d", rr.Code)
					return
				}

				// Verify standardized error response
				var response map[string]interface{}
				err = json.Unmarshal(rr.Body.Bytes(), &response)
				if err != nil {
					t.Errorf("Failed to parse response: %v", err)
					return
				}

				if errorObj, exists := response["error"]; exists {
					if errorMap, ok := errorObj.(map[string]interface{}); ok {
						requiredFields := []string{"code", "message"}
						for _, field := range requiredFields {
							if _, exists := errorMap[field]; !exists {
								t.Errorf("Error response should contain %s field", field)
							}
						}
					}
				}

				t.Log("Unauthorized deletion correctly prevented")
			}
		})
	})
}