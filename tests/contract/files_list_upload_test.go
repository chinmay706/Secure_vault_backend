package contract

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFilesListAndUpload tests the GET and POST /api/v1/files endpoints contract
func TestFilesListAndUpload(t *testing.T) {
	app := TestApp(t)

	t.Run("GET /api/v1/files - list files with filters and pagination", func(t *testing.T) {
		tests := []struct {
			name           string
			queryParams    string
			expectedStatus int
			checkResponse  func(t *testing.T, body []byte)
		}{
			{
				name:           "list files without filters",
				queryParams:    "",
				expectedStatus: http.StatusOK,
				checkResponse: func(t *testing.T, body []byte) {
					var response map[string]interface{}
					err := json.Unmarshal(body, &response)
					if err != nil {
						t.Errorf("Failed to parse JSON response: %v", err)
						return
					}

					// Check for files array
					if files, exists := response["files"]; !exists {
						t.Error("Response should contain files array")
					} else if filesArray, ok := files.([]interface{}); !ok {
						t.Error("Files should be an array")
					} else {
						t.Logf("Files array length: %d", len(filesArray))
					}

					// Check for pagination metadata
					if pagination, exists := response["pagination"]; !exists {
						t.Error("Response should contain pagination metadata")
					} else if paginationMap, ok := pagination.(map[string]interface{}); ok {
						expectedFields := []string{"page", "limit", "total", "totalPages"}
						for _, field := range expectedFields {
							if _, exists := paginationMap[field]; !exists {
								t.Errorf("Pagination should contain %s field", field)
							}
						}
					}
				},
			},
			{
				name:           "list files with filename search",
				queryParams:    "search=test.txt",
				expectedStatus: http.StatusOK,
				checkResponse: func(t *testing.T, body []byte) {
					var response map[string]interface{}
					err := json.Unmarshal(body, &response)
					if err != nil {
						t.Errorf("Failed to parse JSON response: %v", err)
						return
					}

					if _, exists := response["files"]; !exists {
						t.Error("Response should contain files array")
					}
				},
			},
			{
				name:           "list files with MIME type filter",
				queryParams:    "mimeType=image/jpeg",
				expectedStatus: http.StatusOK,
			},
			{
				name:           "list files with size range filter",
				queryParams:    "minSize=1000&maxSize=50000",
				expectedStatus: http.StatusOK,
			},
			{
				name:           "list files with date range filter",
				queryParams:    "startDate=2025-01-01&endDate=2025-12-31",
				expectedStatus: http.StatusOK,
			},
			{
				name:           "list files with pagination",
				queryParams:    "page=2&limit=10",
				expectedStatus: http.StatusOK,
			},
			{
				name:           "list files with combined filters",
				queryParams:    "search=document&mimeType=application/pdf&page=1&limit=5",
				expectedStatus: http.StatusOK,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				url := "/api/v1/files"
				if tt.queryParams != "" {
					url += "?" + tt.queryParams
				}

				req, err := http.NewRequest("GET", url, nil)
				if err != nil {
					t.Fatalf("Failed to create request: %v", err)
				}

				// Add Authorization header (mock for now)
				req.Header.Set("Authorization", "Bearer mock-jwt-token")

				rr := httptest.NewRecorder()
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
	})

	t.Run("POST /api/v1/files - upload files", func(t *testing.T) {
		tests := []struct {
			name           string
			fileName       string
			fileContent    string
			contentType    string
			expectedStatus int
			checkResponse  func(t *testing.T, body []byte)
		}{
			{
				name:           "upload single text file",
				fileName:       "test.txt",
				fileContent:    "Hello, World!",
				contentType:    "text/plain",
				expectedStatus: http.StatusCreated,
				checkResponse: func(t *testing.T, body []byte) {
					var response map[string]interface{}
					err := json.Unmarshal(body, &response)
					if err != nil {
						t.Errorf("Failed to parse JSON response: %v", err)
						return
					}

					// Check for uploaded file metadata
					if files, exists := response["files"]; !exists {
						t.Error("Response should contain files array")
					} else if filesArray, ok := files.([]interface{}); ok && len(filesArray) > 0 {
						if fileMap, ok := filesArray[0].(map[string]interface{}); ok {
							expectedFields := []string{"id", "filename", "size", "mimeType", "uploadedAt", "isDedup"}
							for _, field := range expectedFields {
								if _, exists := fileMap[field]; !exists {
									t.Errorf("File metadata should contain %s field", field)
								}
							}

							// Verify dedup info
							if dedupInfo, exists := fileMap["dedupInfo"]; exists {
								if dedupMap, ok := dedupInfo.(map[string]interface{}); ok {
									if _, exists := dedupMap["sha256"]; !exists {
										t.Error("Dedup info should contain sha256 hash")
									}
								}
							}
						}
					}
				},
			},
			{
				name:           "upload image file",
				fileName:       "image.jpg",
				fileContent:    "fake-jpeg-content",
				contentType:    "image/jpeg",
				expectedStatus: http.StatusCreated,
			},
			{
				name:           "upload file without content type - should detect automatically",
				fileName:       "document.pdf",
				fileContent:    "%PDF-1.4 fake pdf content",
				contentType:    "",
				expectedStatus: http.StatusCreated,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Create multipart form data
				var b bytes.Buffer
				writer := multipart.NewWriter(&b)

				// Add file field
				part, err := writer.CreateFormFile("files", tt.fileName)
				if err != nil {
					t.Fatalf("Failed to create form file: %v", err)
				}

				_, err = io.WriteString(part, tt.fileContent)
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
				req.Header.Set("Authorization", "Bearer mock-jwt-token")

				rr := httptest.NewRecorder()
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
	})

	t.Run("POST /api/v1/files - validation errors with standardized envelope", func(t *testing.T) {
		tests := []struct {
			name           string
			setupRequest   func() (*http.Request, error)
			expectedStatus int
		}{
			{
				name: "upload without authentication",
				setupRequest: func() (*http.Request, error) {
					var b bytes.Buffer
					writer := multipart.NewWriter(&b)
					part, _ := writer.CreateFormFile("files", "test.txt")
					io.WriteString(part, "content")
					writer.Close()

					req, err := http.NewRequest("POST", "/api/v1/files", &b)
					if err != nil {
						return nil, err
					}
					req.Header.Set("Content-Type", writer.FormDataContentType())
					// No Authorization header
					return req, nil
				},
				expectedStatus: http.StatusUnauthorized,
			},
			{
				name: "upload without files",
				setupRequest: func() (*http.Request, error) {
					req, err := http.NewRequest("POST", "/api/v1/files", strings.NewReader("{}"))
					if err != nil {
						return nil, err
					}
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", "Bearer mock-jwt-token")
					return req, nil
				},
				expectedStatus: http.StatusBadRequest,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req, err := tt.setupRequest()
				if err != nil {
					t.Fatalf("Failed to setup request: %v", err)
				}

				rr := httptest.NewRecorder()
				app.ServeHTTP(rr, req)

				// Check status code
				if rr.Code != tt.expectedStatus {
					t.Errorf("Expected status %d, got %d. Response: %s", 
						tt.expectedStatus, rr.Code, rr.Body.String())
					return
				}

				// Verify standardized error envelope
				{
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
	})
}

// TestFilesValidationErrors tests T006a - standardized error envelope for validation errors
func TestFilesValidationErrors(t *testing.T) {
	app := TestApp(t)

	t.Run("GET /api/v1/files - invalid filter validation", func(t *testing.T) {
		tests := []struct {
			name           string
			queryParams    string
			expectedStatus int
			expectedCode   string
		}{
			{
				name:           "Invalid size_min parameter",
				queryParams:    "?size_min=invalid",
				expectedStatus: http.StatusBadRequest,
				expectedCode:   "INVALID_SIZE_FILTER",
			},
			{
				name:           "Invalid size_max parameter", 
				queryParams:    "?size_max=not_a_number",
				expectedStatus: http.StatusBadRequest,
				expectedCode:   "INVALID_SIZE_FILTER",
			},
			{
				name:           "Invalid date format",
				queryParams:    "?uploaded_after=invalid-date",
				expectedStatus: http.StatusBadRequest,
				expectedCode:   "INVALID_DATE_FILTER",
			},
			{
				name:           "Invalid pagination limit (too high)",
				queryParams:    "?limit=10000",
				expectedStatus: http.StatusBadRequest,
				expectedCode:   "INVALID_PAGINATION",
			},
			{
				name:           "Invalid pagination limit (negative)",
				queryParams:    "?limit=-1",
				expectedStatus: http.StatusBadRequest,
				expectedCode:   "INVALID_PAGINATION",
			},
			{
				name:           "Invalid pagination offset",
				queryParams:    "?offset=-10",
				expectedStatus: http.StatusBadRequest,
				expectedCode:   "INVALID_PAGINATION",
			},
			{
				name:           "Conflicting size filters",
				queryParams:    "?size_min=1000&size_max=100",
				expectedStatus: http.StatusBadRequest,
				expectedCode:   "CONFLICTING_SIZE_FILTERS",
			},
			{
				name:           "Conflicting date filters",
				queryParams:    "?uploaded_after=2024-12-01&uploaded_before=2024-01-01",
				expectedStatus: http.StatusBadRequest,
				expectedCode:   "CONFLICTING_DATE_FILTERS",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req, err := http.NewRequest("GET", "/api/v1/files"+tt.queryParams, nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Authorization", "Bearer mock-jwt-token")

				rr := httptest.NewRecorder()
				app.ServeHTTP(rr, req)

				// Check status code
				if rr.Code != tt.expectedStatus {
					t.Errorf("Expected status %d, got %d. Response: %s", 
						tt.expectedStatus, rr.Code, rr.Body.String())
					return
				}

				// Verify standardized error envelope with specific error codes
				{
					var response map[string]interface{}
					err := json.Unmarshal(rr.Body.Bytes(), &response)
					if err != nil {
						t.Errorf("Failed to parse JSON response: %v", err)
						return
					}

					if errorData, exists := response["error"]; !exists {
						t.Error("Response should contain error field")
					} else if errorMap, ok := errorData.(map[string]interface{}); !ok {
						t.Error("Error should be an object")
					} else {
						// Check required error envelope fields
						if code, exists := errorMap["code"]; !exists {
							t.Error("Error should contain code field")
						} else if codeStr, ok := code.(string); !ok {
							t.Error("Error code should be a string")
						} else if codeStr != tt.expectedCode {
							t.Errorf("Expected error code %s, got %s", tt.expectedCode, codeStr)
						}

						if message, exists := errorMap["message"]; !exists {
							t.Error("Error should contain message field")
						} else if _, ok := message.(string); !ok {
							t.Error("Error message should be a string")
						}

						// Details field is optional but should be object if present
						if details, exists := errorMap["details"]; exists {
							if _, ok := details.(map[string]interface{}); !ok {
								t.Error("Error details should be an object if present")
							}
						}
					}
				}
			})
		}
	})

	t.Run("POST /api/v1/files - upload validation errors", func(t *testing.T) {
		tests := []struct {
			name           string
			setupRequest   func() (*http.Request, error)
			expectedStatus int
			expectedCode   string
		}{
			{
				name: "Empty multipart form",
				setupRequest: func() (*http.Request, error) {
					var b bytes.Buffer
					writer := multipart.NewWriter(&b)
					writer.Close()

					req, err := http.NewRequest("POST", "/api/v1/files", &b)
					if err != nil {
						return nil, err
					}
					req.Header.Set("Content-Type", writer.FormDataContentType())
					req.Header.Set("Authorization", "Bearer mock-jwt-token")
					return req, nil
				},
				expectedStatus: http.StatusBadRequest,
				expectedCode:   "NO_FILES_PROVIDED",
			},
			{
				name: "Invalid file extension",
				setupRequest: func() (*http.Request, error) {
					var b bytes.Buffer
					writer := multipart.NewWriter(&b)
					part, _ := writer.CreateFormFile("files", "malicious.exe")
					io.WriteString(part, "executable content")
					writer.Close()

					req, err := http.NewRequest("POST", "/api/v1/files", &b)
					if err != nil {
						return nil, err
					}
					req.Header.Set("Content-Type", writer.FormDataContentType())
					req.Header.Set("Authorization", "Bearer mock-jwt-token")
					return req, nil
				},
				expectedStatus: http.StatusBadRequest,
				expectedCode:   "INVALID_FILE_TYPE",
			},
			{
				name: "Invalid filename characters",
				setupRequest: func() (*http.Request, error) {
					var b bytes.Buffer
					writer := multipart.NewWriter(&b)
					part, _ := writer.CreateFormFile("files", "file<>:|?.txt")
					io.WriteString(part, "content")
					writer.Close()

					req, err := http.NewRequest("POST", "/api/v1/files", &b)
					if err != nil {
						return nil, err
					}
					req.Header.Set("Content-Type", writer.FormDataContentType())
					req.Header.Set("Authorization", "Bearer mock-jwt-token")
					return req, nil
				},
				expectedStatus: http.StatusBadRequest,
				expectedCode:   "INVALID_FILENAME",
			},
			{
				name: "Filename too long",
				setupRequest: func() (*http.Request, error) {
					var b bytes.Buffer
					writer := multipart.NewWriter(&b)
					longFilename := strings.Repeat("a", 256) + ".txt" // 260 chars total
					part, _ := writer.CreateFormFile("files", longFilename)
					io.WriteString(part, "content")
					writer.Close()

					req, err := http.NewRequest("POST", "/api/v1/files", &b)
					if err != nil {
						return nil, err
					}
					req.Header.Set("Content-Type", writer.FormDataContentType())
					req.Header.Set("Authorization", "Bearer mock-jwt-token")
					return req, nil
				},
				expectedStatus: http.StatusBadRequest,
				expectedCode:   "FILENAME_TOO_LONG",
			},
			{
				name: "Invalid Content-Type header",
				setupRequest: func() (*http.Request, error) {
					req, err := http.NewRequest("POST", "/api/v1/files", strings.NewReader("not multipart"))
					if err != nil {
						return nil, err
					}
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", "Bearer mock-jwt-token")
					return req, nil
				},
				expectedStatus: http.StatusBadRequest,
				expectedCode:   "INVALID_CONTENT_TYPE",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req, err := tt.setupRequest()
				if err != nil {
					t.Fatalf("Failed to setup request: %v", err)
				}

				rr := httptest.NewRecorder()
				app.ServeHTTP(rr, req)

				// Check status code
				if rr.Code != tt.expectedStatus {
					t.Errorf("Expected status %d, got %d. Response: %s", 
						tt.expectedStatus, rr.Code, rr.Body.String())
					return
				}

				// Verify standardized error envelope with specific error codes
				{
					var response map[string]interface{}
					err := json.Unmarshal(rr.Body.Bytes(), &response)
					if err != nil {
						t.Errorf("Failed to parse JSON response: %v", err)
						return
					}

					if errorData, exists := response["error"]; !exists {
						t.Error("Response should contain error field")
					} else if errorMap, ok := errorData.(map[string]interface{}); !ok {
						t.Error("Error should be an object")
					} else {
						// Check required error envelope fields
						if code, exists := errorMap["code"]; !exists {
							t.Error("Error should contain code field")
						} else if codeStr, ok := code.(string); !ok {
							t.Error("Error code should be a string")
						} else if codeStr != tt.expectedCode {
							t.Errorf("Expected error code %s, got %s", tt.expectedCode, codeStr)
						}

						if message, exists := errorMap["message"]; !exists {
							t.Error("Error should contain message field")
						} else if _, ok := message.(string); !ok {
							t.Error("Error message should be a string")
						}
					}
				}
			})
		}
	})
}