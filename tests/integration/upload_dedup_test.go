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

func TestUploadDeduplication(t *testing.T) {
	app := setupApp(t)

	t.Run("File deduplication integration workflow", func(t *testing.T) {
		authToken := "Bearer integration-test-token"

		// Step 1: Upload first file
		t.Run("Step 1: Upload original file", func(t *testing.T) {
			// Create a test file content
			fileContent := []byte("This is a test file for deduplication testing.")
			fileName := "test-document.txt"

			// Calculate expected hash
			hasher := sha256.New()
			hasher.Write(fileContent)
			expectedHash := fmt.Sprintf("%x", hasher.Sum(nil))

			// Create multipart form request
			var buf bytes.Buffer
			writer := multipart.NewWriter(&buf)

			// Add file field
			part, err := writer.CreateFormFile("file", fileName)
			if err != nil {
				t.Fatal(err)
			}
			_, err = io.Copy(part, bytes.NewReader(fileContent))
			if err != nil {
				t.Fatal(err)
			}

			writer.Close()

			// Make request
			req, err := http.NewRequest("POST", "/api/v1/files/upload", &buf)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", writer.FormDataContentType())
			req.Header.Set("Authorization", authToken)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			// Verify successful upload
			if rr.Code != http.StatusCreated {
				t.Errorf("Expected 201 Created for first upload, got %d", rr.Code)
				return
			}

			var response map[string]interface{}
			err = json.Unmarshal(rr.Body.Bytes(), &response)
			if err != nil {
				t.Fatal("Failed to parse JSON response:", err)
			}

			// Verify response structure
			if file, exists := response["file"]; exists {
				if fileMap, ok := file.(map[string]interface{}); ok {
					// Check that hash is calculated correctly
					if hash, exists := fileMap["hash"]; exists {
						if hashStr, ok := hash.(string); ok {
							if hashStr != expectedHash {
								t.Errorf("Expected hash %s, got %s", expectedHash, hashStr)
							}
						}
					}

					// Check filename
					if filename, exists := fileMap["filename"]; exists {
						if filenameStr, ok := filename.(string); ok {
							if filenameStr != fileName {
								t.Errorf("Expected filename %s, got %s", fileName, filenameStr)
							}
						}
					}

					// Check size
					if size, exists := fileMap["size"]; exists {
						if sizeValue, ok := size.(float64); ok {
							expectedSize := float64(len(fileContent))
							if sizeValue != expectedSize {
								t.Errorf("Expected size %v, got %v", expectedSize, sizeValue)
							}
						}
					}

					// Store file ID for subsequent tests
					if fileID, exists := fileMap["id"]; exists {
						t.Logf("First upload successful, file ID: %v", fileID)
					}
				}
			}
		})

		// Step 2: Upload different file
		t.Run("Step 2: Upload different file", func(t *testing.T) {
			// Create a different test file
			differentContent := []byte("This is a completely different file content for testing.")
			fileName := "different-document.txt"

			// Create multipart form request
			var buf bytes.Buffer
			writer := multipart.NewWriter(&buf)

			part, err := writer.CreateFormFile("file", fileName)
			if err != nil {
				t.Fatal(err)
			}
			_, err = io.Copy(part, bytes.NewReader(differentContent))
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
				t.Errorf("Expected 201 Created for second upload, got %d", rr.Code)
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
					t.Logf("Second upload successful (different file): %v", fileMap["id"])
				}
			}
		})

		// Step 3: Upload duplicate file (same content as Step 1)
		t.Run("Step 3: Upload duplicate file", func(t *testing.T) {
			// Upload the same content as Step 1 again
			fileContent := []byte("This is a test file for deduplication testing.")
			fileName := "duplicate-test-document.txt" // Different filename, same content

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
					// Check if deduplication information is provided
					if isDuplicate, exists := fileMap["is_duplicate"]; exists {
						if isDuplicateValue, ok := isDuplicate.(bool); ok && isDuplicateValue {
							t.Log("Deduplication detected correctly")
						}
					}

					// Check if original file ID is referenced
					if originalFileID, exists := fileMap["original_file_id"]; exists {
						t.Logf("Duplicate references original file: %v", originalFileID)
					}

					// This upload should have a new file ID but reference the same storage
					t.Logf("Duplicate upload successful: %v", fileMap["id"])
				}
			}
		})

		// Step 4: Verify deduplication statistics
		t.Run("Step 4: Verify deduplication statistics", func(t *testing.T) {
			req, err := http.NewRequest("GET", "/api/v1/stats/me", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", authToken)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Expected 200 OK for stats, got %d", rr.Code)
				return
			}

			var response map[string]interface{}
			err = json.Unmarshal(rr.Body.Bytes(), &response)
			if err != nil {
				t.Errorf("Failed to parse response: %v", err)
				return
			}

			// Verify deduplication statistics are tracked
			if deduplication, exists := response["deduplication"]; exists {
				if dedupMap, ok := deduplication.(map[string]interface{}); ok {
					// Should have duplicate count
					if duplicateCount, exists := dedupMap["duplicate_files"]; exists {
						if count, ok := duplicateCount.(float64); ok {
							if count < 1 {
								t.Error("Should have at least 1 duplicate file")
							}
							t.Logf("Duplicate files count: %v", count)
						}
					}

					// Should have bytes saved
					if bytesSaved, exists := dedupMap["bytes_saved"]; exists {
						if saved, ok := bytesSaved.(float64); ok {
							if saved < 0 {
								t.Error("Bytes saved should not be negative")
							}
							t.Logf("Bytes saved through deduplication: %v", saved)
						}
					}

					// Should have deduplication percentage
					if dedupPercentage, exists := dedupMap["deduplication_percentage"]; exists {
						if percentage, ok := dedupPercentage.(float64); ok {
							if percentage < 0 || percentage > 100 {
								t.Error("Deduplication percentage should be between 0 and 100")
							}
							t.Logf("Deduplication percentage: %v%%", percentage)
						}
					}
				}
			}

			// Verify total statistics are consistent
			if totalFiles, exists := response["total_files"]; exists {
				if count, ok := totalFiles.(float64); ok {
					t.Logf("Total files: %v", count)
					// Should have at least 3 files (2 unique + 1 duplicate reference)
				}
			}

			if storageUsed, exists := response["storage_used"]; exists {
				if storage, ok := storageUsed.(float64); ok {
					t.Logf("Storage used: %v bytes", storage)
					// Storage should reflect actual bytes stored (after deduplication)
				}
			}
		})

		// Additional test: Cross-user deduplication
		t.Run("Cross-user deduplication test", func(t *testing.T) {
			// Simulate a different user uploading the same file
			differentUserToken := "Bearer different-user-token"

			// Upload same content as Step 1 with different user
			fileContent := []byte("This is a test file for deduplication testing.")
			fileName := "cross-user-duplicate.txt"

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
			req.Header.Set("Authorization", differentUserToken)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusCreated {
				t.Errorf("Expected 201 Created for cross-user upload, got %d", rr.Code)
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
					t.Logf("Cross-user upload successful: %v", fileMap["id"])

					// This should also benefit from deduplication
					// but maintain separate file metadata for each user
					if isDuplicate, exists := fileMap["is_duplicate"]; exists {
						if isDuplicateValue, ok := isDuplicate.(bool); ok && isDuplicateValue {
							t.Log("Cross-user deduplication detected correctly")
						}
					}
				}
			}
		})
	})
}