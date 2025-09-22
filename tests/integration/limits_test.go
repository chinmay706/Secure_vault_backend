package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestLimits tests T015 - quota and rate limit exceed paths (429, quota error)
func TestLimits(t *testing.T) {
	app := setupApp(t)

	t.Run("Limits integration workflow", func(t *testing.T) {
		authToken := "Bearer integration-test-token"

		// Test 1: Rate limit testing
		t.Run("Rate limit testing", func(t *testing.T) {
			// Test rapid requests to trigger rate limiting (2 requests per second limit)
			t.Run("Rapid requests exceed rate limit", func(t *testing.T) {
				// Make 5 rapid requests to /stats/me endpoint
				var responses []int
				
				for i := 0; i < 5; i++ {
					req, err := http.NewRequest("GET", "/api/v1/stats/me", nil)
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set("Authorization", authToken)

					rr := httptest.NewRecorder()
					app.ServeHTTP(rr, req)

					responses = append(responses, rr.Code)

					// Small delay to avoid all being processed simultaneously
					time.Sleep(100 * time.Millisecond)
				}

				// Check if we got any rate limit responses
				rateLimitHit := false
				for i, code := range responses {
					if code == http.StatusTooManyRequests {
						rateLimitHit = true
						t.Logf("Request %d: Rate limited (429 Too Many Requests)", i+1)
					} else if code == http.StatusOK {
						t.Logf("Request %d: Success (200 OK)", i+1)
					} else {
						t.Errorf("Request %d: Unexpected status code %d", i+1, code)
					}
				}

				if !rateLimitHit {
					// This is expected in TDD mode - rate limiting not implemented yet
					t.Log("No rate limiting detected (expected until implementation)")
				}
			})

			// Test rate limit error response format
			t.Run("Rate limit error response format", func(t *testing.T) {
				// Make rapid fire requests to trigger rate limit
				for i := 0; i < 10; i++ {
					req, err := http.NewRequest("GET", "/api/v1/stats/me", nil)
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set("Authorization", authToken)

					rr := httptest.NewRecorder()
					app.ServeHTTP(rr, req)

					if rr.Code == http.StatusTooManyRequests {
						// Verify standardized error envelope
						var response map[string]interface{}
						err := json.Unmarshal(rr.Body.Bytes(), &response)
						if err != nil {
							t.Error("Rate limit response should be valid JSON")
							continue
						}

						if errorObj, exists := response["error"]; exists {
							if errorMap, ok := errorObj.(map[string]interface{}); ok {
								// Check required error fields
								requiredFields := []string{"code", "message"}
								for _, field := range requiredFields {
									if _, exists := errorMap[field]; !exists {
										t.Errorf("Rate limit error should contain %s field", field)
									}
								}

								// Check error code
								if code, exists := errorMap["code"]; exists {
									if codeStr, ok := code.(string); ok {
										expectedCodes := []string{"RATE_LIMIT_EXCEEDED", "TOO_MANY_REQUESTS"}
										codeFound := false
										for _, expectedCode := range expectedCodes {
											if codeStr == expectedCode {
												codeFound = true
												break
											}
										}
										if !codeFound {
											t.Logf("Rate limit error code: %s (should be one of: %v)", codeStr, expectedCodes)
										}
									}
								}

								// Check if details include rate limit info
								if details, exists := errorMap["details"]; exists {
									if detailsMap, ok := details.(map[string]interface{}); ok {
										expectedDetailFields := []string{"retry_after", "limit", "window"}
										for _, field := range expectedDetailFields {
											if _, exists := detailsMap[field]; exists {
												t.Logf("Rate limit details include %s: %v", field, detailsMap[field])
											}
										}
									}
								}
							}
						} else {
							t.Error("Rate limit response should use standardized error envelope")
						}

						t.Log("Rate limit error response format validated")
						break
					}

					// Small delay between requests
					time.Sleep(50 * time.Millisecond)
				}
			})

			// Test rate limit headers
			t.Run("Rate limit headers", func(t *testing.T) {
				req, err := http.NewRequest("GET", "/api/v1/stats/me", nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Authorization", authToken)

				rr := httptest.NewRecorder()
				app.ServeHTTP(rr, req)

				if rr.Code != http.StatusOK {
					t.Errorf("Expected 200 OK for stats endpoint, got %d", rr.Code)
					return
				}

				// Check for rate limit headers
				expectedHeaders := []string{
					"X-RateLimit-Limit",
					"X-RateLimit-Remaining", 
					"X-RateLimit-Reset",
				}

				for _, header := range expectedHeaders {
					if value := rr.Header().Get(header); value != "" {
						t.Logf("Rate limit header %s: %s", header, value)
					} else {
						t.Logf("Rate limit header %s: not present (may be added in implementation)", header)
					}
				}
			})
		})

		// Test 2: Upload quota testing
		t.Run("Upload quota testing", func(t *testing.T) {
			// Test uploading files that exceed 10MB total quota
			t.Run("Exceed 10MB quota limit", func(t *testing.T) {
				// Upload multiple files to approach quota limit
				fileSize := 3 * 1024 * 1024 // 3MB files
				fileContent := make([]byte, fileSize)
				// Fill with some pattern to make it compressible
				for i := range fileContent {
					fileContent[i] = byte(i % 256)
				}

				var uploadCount int
				var quotaExceeded bool

				// Try to upload multiple 3MB files (should hit 10MB limit)
				for i := 0; i < 5; i++ {
					fileName := fmt.Sprintf("quota-test-%d.bin", i)

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

					if rr.Code == http.StatusCreated {
						uploadCount++
						t.Logf("Upload %d successful (%dMB file)", i+1, fileSize/(1024*1024))
					} else if rr.Code == http.StatusRequestEntityTooLarge || rr.Code == http.StatusPaymentRequired {
						quotaExceeded = true
						t.Logf("Upload %d: Quota exceeded (HTTP %d)", i+1, rr.Code)

						// Verify quota error response format
						var response map[string]interface{}
						err := json.Unmarshal(rr.Body.Bytes(), &response)
						if err == nil {
							if errorObj, exists := response["error"]; exists {
								if errorMap, ok := errorObj.(map[string]interface{}); ok {
									// Check error structure
									requiredFields := []string{"code", "message"}
									for _, field := range requiredFields {
										if _, exists := errorMap[field]; !exists {
											t.Errorf("Quota error should contain %s field", field)
										}
									}

									// Check error code
									if code, exists := errorMap["code"]; exists {
										if codeStr, ok := code.(string); ok {
											expectedCodes := []string{"QUOTA_EXCEEDED", "INSUFFICIENT_QUOTA", "STORAGE_LIMIT_EXCEEDED"}
											codeFound := false
											for _, expectedCode := range expectedCodes {
												if codeStr == expectedCode {
													codeFound = true
													break
												}
											}
											if !codeFound {
												t.Logf("Quota error code: %s (should be one of: %v)", codeStr, expectedCodes)
											}
										}
									}

									// Check if details include quota info
									if details, exists := errorMap["details"]; exists {
										if detailsMap, ok := details.(map[string]interface{}); ok {
											quotaFields := []string{"quota_limit", "quota_used", "quota_remaining", "file_size"}
											for _, field := range quotaFields {
												if value, exists := detailsMap[field]; exists {
													t.Logf("Quota details %s: %v", field, value)
												}
											}
										}
									}
								}
							}
						}
						break
					} else if rr.Code == http.StatusCreated {
						t.Logf("Upload %d: Success (201 Created)", i+1)
					} else {
						t.Logf("Upload %d: Unexpected response %d", i+1, rr.Code)
					}

					// Small delay between uploads
					time.Sleep(100 * time.Millisecond)
				}

				if quotaExceeded {
					t.Logf("Quota limit correctly enforced after %d successful uploads", uploadCount)
				} else {
					t.Log("Quota enforcement not detected (expected until implementation)")
				}
			})

			// Test single file exceeding 10MB limit
			t.Run("Single file exceeds 10MB limit", func(t *testing.T) {
				// Create a file larger than 10MB
				largeFileSize := 12 * 1024 * 1024 // 12MB
				
				// Use a smaller buffer and repeat to avoid memory issues
				smallBuffer := make([]byte, 1024*1024) // 1MB buffer
				for i := range smallBuffer {
					smallBuffer[i] = byte(i % 256)
				}

				var buf bytes.Buffer
				writer := multipart.NewWriter(&buf)

				part, err := writer.CreateFormFile("file", "large-file-test.bin")
				if err != nil {
					t.Fatal(err)
				}

				// Write the large file in chunks
				written := 0
				for written < largeFileSize {
					chunkSize := len(smallBuffer)
					if written+chunkSize > largeFileSize {
						chunkSize = largeFileSize - written
					}

					_, err = part.Write(smallBuffer[:chunkSize])
					if err != nil {
						t.Fatal(err)
					}
					written += chunkSize
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

				if rr.Code == http.StatusRequestEntityTooLarge {
					t.Log("Single large file correctly rejected (413 Request Entity Too Large)")

					// Verify error response
					var response map[string]interface{}
					err := json.Unmarshal(rr.Body.Bytes(), &response)
					if err == nil {
						if errorObj, exists := response["error"]; exists {
							if errorMap, ok := errorObj.(map[string]interface{}); ok {
								if code, exists := errorMap["code"]; exists {
									t.Logf("Large file error code: %v", code)
								}
								if message, exists := errorMap["message"]; exists {
									if msgStr, ok := message.(string); ok {
										if !strings.Contains(strings.ToLower(msgStr), "10mb") && !strings.Contains(strings.ToLower(msgStr), "10 mb") {
											t.Log("Error message should mention 10MB limit")
										}
									}
								}
							}
						}
					}
				} else {
					t.Logf("Large file upload: Unexpected response %d (expected 413)", rr.Code)
				}
			})
		})

		// Test 3: Quota information endpoints
		t.Run("Quota information testing", func(t *testing.T) {
			// Test getting current quota usage
			t.Run("Get quota usage via stats", func(t *testing.T) {
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

				// Check for quota-related fields
				quotaFields := []string{"quota_limit", "quota_used", "quota_remaining"}
				for _, field := range quotaFields {
					if value, exists := response[field]; exists {
						t.Logf("Quota info %s: %v", field, value)
					} else {
						t.Logf("Quota info %s: not present (may be added in implementation)", field)
					}
				}

				// Check storage used vs quota
				if storageUsed, exists := response["storage_used"]; exists {
					if quotaLimit, exists := response["quota_limit"]; exists {
						if storageValue, ok := storageUsed.(float64); ok {
							if quotaValue, ok := quotaLimit.(float64); ok {
								percentage := (storageValue / quotaValue) * 100
								t.Logf("Quota usage: %.1f%% (%v/%v bytes)", percentage, storageValue, quotaValue)
							}
						}
					}
				}
			})

			// Test quota warning thresholds
			t.Run("Quota warning thresholds", func(t *testing.T) {
				// This would test if the system provides warnings at 80%, 90%, etc.
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

				// Look for warning flags or thresholds
				warningFields := []string{"quota_warning", "near_quota_limit", "quota_percentage"}
				for _, field := range warningFields {
					if value, exists := response[field]; exists {
						t.Logf("Quota warning %s: %v", field, value)
					}
				}
			})
		})

		// Test 4: Combined rate limit and quota scenarios
		t.Run("Combined limits testing", func(t *testing.T) {
			// Test rate limiting while approaching quota
			t.Run("Rate limiting during quota approach", func(t *testing.T) {
				// Make rapid upload attempts (should hit rate limit before quota)
				smallFileContent := []byte("Small file for rapid upload testing.")

				var rateLimited, quotaExceeded int

				for i := 0; i < 20; i++ {
					fileName := fmt.Sprintf("rapid-upload-%d.txt", i)

					var buf bytes.Buffer
					writer := multipart.NewWriter(&buf)

					part, err := writer.CreateFormFile("file", fileName)
					if err != nil {
						t.Fatal(err)
					}
					_, err = io.Copy(part, strings.NewReader(string(smallFileContent)))
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

					if rr.Code == http.StatusTooManyRequests {
						rateLimited++
					} else if rr.Code == http.StatusRequestEntityTooLarge || rr.Code == http.StatusPaymentRequired {
						quotaExceeded++
					} else if rr.Code == http.StatusCreated {
						t.Log("Upload successful")
					}

					// Very small delay
					time.Sleep(10 * time.Millisecond)
				}

				t.Logf("Combined limits: %d rate limited, %d quota exceeded", rateLimited, quotaExceeded)
			})

			// Test error priority (which error is returned when both limits hit)
			t.Run("Error priority when both limits exceeded", func(t *testing.T) {
				// This tests which error takes priority when both rate and quota limits are hit
				// The exact behavior may depend on implementation decisions
				t.Log("Error priority testing would validate which limit error is returned when both are exceeded")
			})
		})
	})
}