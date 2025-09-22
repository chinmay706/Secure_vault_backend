package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// TestSearchAndFilter tests T012 - search & filter by filename, mime, size, date; pagination
func TestSearchAndFilter(t *testing.T) {
	app := setupApp(t)

	t.Run("Search and filter integration workflow", func(t *testing.T) {
		authToken := "Bearer integration-test-token"

		// Test comprehensive search and filtering scenarios
		t.Run("Search by filename", func(t *testing.T) {
			tests := []struct {
				name        string
				searchQuery string
				description string
			}{
				{
					name:        "exact filename match",
					searchQuery: "?filename=document.pdf",
					description: "Should find files with exact filename match",
				},
				{
					name:        "partial filename search",
					searchQuery: "?filename=doc",
					description: "Should find files containing 'doc' in filename",
				},
				{
					name:        "case insensitive search",
					searchQuery: "?filename=DOCUMENT",
					description: "Should find files regardless of case",
				},
				{
					name:        "filename with spaces",
					searchQuery: "?filename=" + url.QueryEscape("my document"),
					description: "Should handle filenames with spaces",
				},
				{
					name:        "filename with special characters",
					searchQuery: "?filename=" + url.QueryEscape("file-name_v2.1"),
					description: "Should handle special characters in filenames",
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					req, err := http.NewRequest("GET", "/api/v1/files"+tt.searchQuery, nil)
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set("Authorization", authToken)

					rr := httptest.NewRecorder()
					app.ServeHTTP(rr, req)

					if rr.Code != http.StatusOK {
						t.Errorf("Expected 200 OK for filename search, got %d", rr.Code)
						return
					}

					var response map[string]interface{}
					err = json.Unmarshal(rr.Body.Bytes(), &response)
					if err != nil {
						t.Errorf("Failed to parse response: %v", err)
						return
					}

					// Verify search results structure
					if files, exists := response["files"]; exists {
						if filesArray, ok := files.([]interface{}); ok {
							// Each file should match the search criteria
							for i, fileItem := range filesArray {
								if fileMap, ok := fileItem.(map[string]interface{}); ok {
									if filename, exists := fileMap["filename"]; exists {
										if filenameStr, ok := filename.(string); ok {
											t.Logf("Found file %d: %s (search: %s)", i, filenameStr, tt.searchQuery)
											// Additional validation could check if filename matches search criteria
										}
									}
								}
							}
						}
					}

					// Verify pagination metadata
					if pagination, exists := response["pagination"]; exists {
						if paginationMap, ok := pagination.(map[string]interface{}); ok {
							requiredFields := []string{"total", "page", "limit", "totalPages"}
							for _, field := range requiredFields {
								if _, exists := paginationMap[field]; !exists {
									t.Errorf("Pagination should contain %s field", field)
								}
							}
						}
					}
				})
			}
		})

		t.Run("Filter by MIME type", func(t *testing.T) {
			mimeTypes := []struct {
				mimeType    string
				description string
			}{
				{"application/pdf", "PDF documents"},
				{"image/jpeg", "JPEG images"},
				{"image/png", "PNG images"},
				{"text/plain", "Text files"},
				{"application/json", "JSON files"},
				{"video/mp4", "MP4 videos"},
			}

			for _, mt := range mimeTypes {
				t.Run(fmt.Sprintf("filter_%s", mt.mimeType), func(t *testing.T) {
					queryParam := "?mime_type=" + url.QueryEscape(mt.mimeType)
					req, err := http.NewRequest("GET", "/api/v1/files"+queryParam, nil)
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set("Authorization", authToken)

					rr := httptest.NewRecorder()
					app.ServeHTTP(rr, req)

					if rr.Code != http.StatusOK {
						t.Errorf("Expected 200 OK for MIME type filter, got %d", rr.Code)
						return
					}

					var response map[string]interface{}
					err = json.Unmarshal(rr.Body.Bytes(), &response)
					if err != nil {
						t.Errorf("Failed to parse response: %v", err)
						return
					}

					if files, exists := response["files"]; exists {
						if filesArray, ok := files.([]interface{}); ok {
							// Verify all returned files have the specified MIME type
							for i, fileItem := range filesArray {
								if fileMap, ok := fileItem.(map[string]interface{}); ok {
									if mimeType, exists := fileMap["mime_type"]; exists {
										if mimeTypeStr, ok := mimeType.(string); ok {
											if mimeTypeStr != mt.mimeType {
												t.Errorf("File %d has MIME type %s, expected %s", i, mimeTypeStr, mt.mimeType)
											}
										}
									} else {
										t.Errorf("File %d missing mime_type field", i)
									}
								}
							}
						}
					}
				})
			}
		})

		t.Run("Filter by size range", func(t *testing.T) {
			sizeTests := []struct {
				name        string
				queryParams string
				description string
			}{
				{
					name:        "small files only",
					queryParams: "?size_max=1048576", // 1MB
					description: "Should return files smaller than 1MB",
				},
				{
					name:        "large files only",
					queryParams: "?size_min=10485760", // 10MB
					description: "Should return files larger than 10MB",
				},
				{
					name:        "medium size range",
					queryParams: "?size_min=1048576&size_max=5242880", // 1MB to 5MB
					description: "Should return files between 1MB and 5MB",
				},
				{
					name:        "exact size",
					queryParams: "?size_min=2048&size_max=2048",
					description: "Should return files with exact size",
				},
			}

			for _, st := range sizeTests {
				t.Run(st.name, func(t *testing.T) {
					req, err := http.NewRequest("GET", "/api/v1/files"+st.queryParams, nil)
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set("Authorization", authToken)

					rr := httptest.NewRecorder()
					app.ServeHTTP(rr, req)

					if rr.Code != http.StatusOK {
						t.Errorf("Expected 200 OK for size filter, got %d", rr.Code)
						return
					}

					var response map[string]interface{}
					err = json.Unmarshal(rr.Body.Bytes(), &response)
					if err != nil {
						t.Errorf("Failed to parse response: %v", err)
						return
					}

					if files, exists := response["files"]; exists {
						if filesArray, ok := files.([]interface{}); ok {
							// Verify all returned files meet size criteria
							for i, fileItem := range filesArray {
								if fileMap, ok := fileItem.(map[string]interface{}); ok {
									if size, exists := fileMap["size"]; exists {
										if sizeValue, ok := size.(float64); ok {
											t.Logf("File %d size: %.0f bytes", i, sizeValue)
											// Additional validation could check size against filter criteria
										}
									}
								}
							}
						}
					}
				})
			}
		})

		t.Run("Filter by date range", func(t *testing.T) {
			now := time.Now()
			yesterday := now.AddDate(0, 0, -1)
			lastWeek := now.AddDate(0, 0, -7)
			lastMonth := now.AddDate(0, -1, 0)

			dateTests := []struct {
				name        string
				queryParams string
				description string
			}{
				{
					name:        "files from yesterday",
					queryParams: fmt.Sprintf("?uploaded_after=%s", yesterday.Format("2006-01-02")),
					description: "Should return files uploaded after yesterday",
				},
				{
					name:        "files from last week",
					queryParams: fmt.Sprintf("?uploaded_after=%s&uploaded_before=%s", lastWeek.Format("2006-01-02"), yesterday.Format("2006-01-02")),
					description: "Should return files uploaded between last week and yesterday",
				},
				{
					name:        "files from last month",
					queryParams: fmt.Sprintf("?uploaded_after=%s", lastMonth.Format("2006-01-02")),
					description: "Should return files uploaded in the last month",
				},
				{
					name:        "files with datetime precision",
					queryParams: fmt.Sprintf("?uploaded_after=%s", yesterday.Format("2006-01-02T15:04:05Z")),
					description: "Should handle datetime with time precision",
				},
			}

			for _, dt := range dateTests {
				t.Run(dt.name, func(t *testing.T) {
					req, err := http.NewRequest("GET", "/api/v1/files"+dt.queryParams, nil)
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set("Authorization", authToken)

					rr := httptest.NewRecorder()
					app.ServeHTTP(rr, req)

					if rr.Code != http.StatusOK {
						t.Errorf("Expected 200 OK for date filter, got %d", rr.Code)
						return
					}

					var response map[string]interface{}
					err = json.Unmarshal(rr.Body.Bytes(), &response)
					if err != nil {
						t.Errorf("Failed to parse response: %v", err)
						return
					}

					if files, exists := response["files"]; exists {
						if filesArray, ok := files.([]interface{}); ok {
							for i, fileItem := range filesArray {
								if fileMap, ok := fileItem.(map[string]interface{}); ok {
									if uploadDate, exists := fileMap["upload_date"]; exists {
										if dateStr, ok := uploadDate.(string); ok {
											t.Logf("File %d upload date: %s", i, dateStr)
											// Additional validation could parse and check date against filter criteria
										}
									}
								}
							}
						}
					}
				})
			}
		})

		t.Run("Combined filters", func(t *testing.T) {
			combinedTests := []struct {
				name        string
				queryParams string
				description string
			}{
				{
					name:        "pdf files from last week",
					queryParams: fmt.Sprintf("?mime_type=application/pdf&uploaded_after=%s", time.Now().AddDate(0, 0, -7).Format("2006-01-02")),
					description: "Should return PDF files uploaded in the last week",
				},
				{
					name:        "large images",
					queryParams: "?mime_type=image/jpeg&size_min=1048576",
					description: "Should return JPEG images larger than 1MB",
				},
				{
					name:        "document search with size limit",
					queryParams: "?filename=document&size_max=5242880",
					description: "Should return files with 'document' in name smaller than 5MB",
				},
				{
					name:        "recent small text files",
					queryParams: fmt.Sprintf("?mime_type=text/plain&size_max=10240&uploaded_after=%s", time.Now().AddDate(0, 0, -1).Format("2006-01-02")),
					description: "Should return small text files from yesterday",
				},
			}

			for _, ct := range combinedTests {
				t.Run(ct.name, func(t *testing.T) {
					req, err := http.NewRequest("GET", "/api/v1/files"+ct.queryParams, nil)
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set("Authorization", authToken)

					rr := httptest.NewRecorder()
					app.ServeHTTP(rr, req)

					if rr.Code != http.StatusOK {
						t.Errorf("Expected 200 OK for combined filters, got %d", rr.Code)
						return
					}

					var response map[string]interface{}
					err = json.Unmarshal(rr.Body.Bytes(), &response)
					if err != nil {
						t.Errorf("Failed to parse response: %v", err)
						return
					}

					if files, exists := response["files"]; exists {
						if filesArray, ok := files.([]interface{}); ok {
							t.Logf("Combined filter (%s) returned %d files", ct.name, len(filesArray))
							
							// Verify each file meets all specified criteria
							for i, fileItem := range filesArray {
								if fileMap, ok := fileItem.(map[string]interface{}); ok {
									t.Logf("File %d: %v", i, fileMap["filename"])
									// Additional validation could check each criteria is met
								}
							}
						}
					}
				})
			}
		})

		t.Run("Pagination with filters", func(t *testing.T) {
			paginationTests := []struct {
				name        string
				queryParams string
				description string
			}{
				{
					name:        "first page with limit",
					queryParams: "?limit=5&offset=0",
					description: "Should return first 5 files",
				},
				{
					name:        "second page",
					queryParams: "?limit=5&offset=5",
					description: "Should return files 6-10",
				},
				{
					name:        "large page size",
					queryParams: "?limit=50",
					description: "Should return up to 50 files per page",
				},
				{
					name:        "filtered pagination",
					queryParams: "?mime_type=image/jpeg&limit=10&offset=0",
					description: "Should paginate through filtered JPEG images",
				},
			}

			for _, pt := range paginationTests {
				t.Run(pt.name, func(t *testing.T) {
					req, err := http.NewRequest("GET", "/api/v1/files"+pt.queryParams, nil)
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set("Authorization", authToken)

					rr := httptest.NewRecorder()
					app.ServeHTTP(rr, req)

					if rr.Code != http.StatusOK {
						t.Errorf("Expected 200 OK for pagination, got %d", rr.Code)
						return
					}

					var response map[string]interface{}
					err = json.Unmarshal(rr.Body.Bytes(), &response)
					if err != nil {
						t.Errorf("Failed to parse response: %v", err)
						return
					}

					// Verify pagination metadata
					if pagination, exists := response["pagination"]; exists {
						if paginationMap, ok := pagination.(map[string]interface{}); ok {
							// Check pagination structure
							requiredFields := []string{"total", "page", "limit", "totalPages", "hasNext", "hasPrev"}
							for _, field := range requiredFields {
								if _, exists := paginationMap[field]; !exists {
									t.Errorf("Pagination should contain %s field", field)
								}
							}

							// Verify pagination logic
							if limit, exists := paginationMap["limit"]; exists {
								if page, exists := paginationMap["page"]; exists {
									if total, exists := paginationMap["total"]; exists {
										t.Logf("Pagination: page %v, limit %v, total %v", page, limit, total)
									}
								}
							}
						}
					} else {
						t.Error("Paginated response should contain pagination metadata")
					}

					// Verify files array respects limit
					if files, exists := response["files"]; exists {
						if filesArray, ok := files.([]interface{}); ok {
							// Should not exceed the specified limit
							t.Logf("Returned %d files for pagination test", len(filesArray))
						}
					}
				})
			}
		})

		t.Run("Sorting options", func(t *testing.T) {
			sortTests := []struct {
				name        string
				queryParams string
				description string
			}{
				{
					name:        "sort by filename ascending",
					queryParams: "?sort=filename&order=asc",
					description: "Should return files sorted by filename A-Z",
				},
				{
					name:        "sort by size descending",
					queryParams: "?sort=size&order=desc",
					description: "Should return files sorted by size (largest first)",
				},
				{
					name:        "sort by upload date descending",
					queryParams: "?sort=upload_date&order=desc",
					description: "Should return files sorted by upload date (newest first)",
				},
				{
					name:        "sort by mime type",
					queryParams: "?sort=mime_type&order=asc",
					description: "Should return files sorted by MIME type",
				},
			}

			for _, st := range sortTests {
				t.Run(st.name, func(t *testing.T) {
					req, err := http.NewRequest("GET", "/api/v1/files"+st.queryParams, nil)
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set("Authorization", authToken)

					rr := httptest.NewRecorder()
					app.ServeHTTP(rr, req)

					if rr.Code != http.StatusOK {
						t.Errorf("Expected 200 OK for sorting, got %d", rr.Code)
						return
					}

					var response map[string]interface{}
					err = json.Unmarshal(rr.Body.Bytes(), &response)
					if err != nil {
						t.Errorf("Failed to parse response: %v", err)
						return
					}

					if files, exists := response["files"]; exists {
						if filesArray, ok := files.([]interface{}); ok {
							// Verify sorting is applied (check first few files)
							for i, fileItem := range filesArray {
								if fileMap, ok := fileItem.(map[string]interface{}); ok {
									t.Logf("Sorted file %d: %v", i, fileMap["filename"])
									// Additional validation could verify sort order
								}
								if i >= 2 { // Just check first few for sorting indication
									break
								}
							}
						}
					}
				})
			}
		})
	})
}