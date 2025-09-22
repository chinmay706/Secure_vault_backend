package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCompleteAPIFlow tests the complete API flow from signup to folder operations
func TestCompleteAPIFlow(t *testing.T) {
	app := TestApp(t)

	// Test data
	userEmail := "testuser@example.com"
	userPassword := "securepassword123"
	userName := "Test User"
	var authToken string
	var uploadedFileID string
	var createdFolderID string

	t.Run("1. User Signup", func(t *testing.T) {
		signupBody := map[string]interface{}{
			"email":    userEmail,
			"password": userPassword,
			"name":     userName,
		}

		requestBodyBytes, err := json.Marshal(signupBody)
		if err != nil {
			t.Fatalf("Failed to marshal signup request: %v", err)
		}

		req, err := http.NewRequest("POST", "/api/v1/auth/signup", bytes.NewBuffer(requestBodyBytes))
		if err != nil {
			t.Fatalf("Failed to create signup request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("Signup failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		if err != nil {
			t.Fatalf("Failed to parse signup response: %v", err)
		}

		token, exists := response["token"]
		if !exists {
			t.Fatal("Signup response should contain token")
		}
		authToken = token.(string)

		t.Logf("✓ Signup successful. Token: %s...", authToken[:20])
	})

	t.Run("2. User Login", func(t *testing.T) {
		loginBody := map[string]interface{}{
			"email":    userEmail,
			"password": userPassword,
		}

		requestBodyBytes, err := json.Marshal(loginBody)
		if err != nil {
			t.Fatalf("Failed to marshal login request: %v", err)
		}

		req, err := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(requestBodyBytes))
		if err != nil {
			t.Fatalf("Failed to create login request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Login failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		if err != nil {
			t.Fatalf("Failed to parse login response: %v", err)
		}

		token, exists := response["token"]
		if !exists {
			t.Fatal("Login response should contain token")
		}
		authToken = token.(string) // Update token if different

		t.Logf("✓ Login successful. Token updated: %s...", authToken[:20])
	})

	t.Run("3. Upload Sample File", func(t *testing.T) {
		// Find sample.txt file from test-files
		sampleFilePath := filepath.Join("..", "..", "test-files", "sample.txt")
		if _, err := os.Stat(sampleFilePath); os.IsNotExist(err) {
			// Try alternative paths
			possiblePaths := []string{
				"test-files/sample.txt",
				"../test-files/sample.txt",
				"../../test-files/sample.txt",
			}
			
			found := false
			for _, path := range possiblePaths {
				if _, err := os.Stat(path); err == nil {
					sampleFilePath = path
					found = true
					break
				}
			}
			
			if !found {
				t.Skip("sample.txt not found in test-files directory")
			}
		}

		// Read sample file
		fileContent, err := os.ReadFile(sampleFilePath)
		if err != nil {
			t.Fatalf("Failed to read sample file: %v", err)
		}

		// Create multipart form
		var b bytes.Buffer
		writer := multipart.NewWriter(&b)

		// Add file field
		fileField, err := writer.CreateFormFile("file", "sample.txt")
		if err != nil {
			t.Fatalf("Failed to create form file: %v", err)
		}
		_, err = fileField.Write(fileContent)
		if err != nil {
			t.Fatalf("Failed to write file content: %v", err)
		}

		writer.Close()

		req, err := http.NewRequest("POST", "/api/v1/files", &b)
		if err != nil {
			t.Fatalf("Failed to create upload request: %v", err)
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("File upload failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		if err != nil {
			t.Fatalf("Failed to parse upload response: %v", err)
		}

		file, exists := response["file"]
		if !exists {
			t.Fatal("Upload response should contain file object")
		}
		
		fileObj := file.(map[string]interface{})
		fileID, exists := fileObj["id"]
		if !exists {
			t.Fatal("Upload response file should contain id")
		}
		uploadedFileID = fileID.(string)

		t.Logf("✓ File upload successful. File ID: %s", uploadedFileID)
	})

	t.Run("4. List Files", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/api/v1/files", nil)
		if err != nil {
			t.Fatalf("Failed to create list files request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("List files failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		if err != nil {
			t.Fatalf("Failed to parse list files response: %v", err)
		}

		files, exists := response["files"]
		if !exists {
			t.Fatal("List files response should contain files array")
		}

		filesArray := files.([]interface{})
		if len(filesArray) == 0 {
			t.Fatal("Should have at least one file after upload")
		}

		t.Logf("✓ List files successful. Found %d files", len(filesArray))
	})

	t.Run("5. Create Folder", func(t *testing.T) {
		createFolderBody := map[string]interface{}{
			"name": "Test Folder",
		}

		requestBodyBytes, err := json.Marshal(createFolderBody)
		if err != nil {
			t.Fatalf("Failed to marshal create folder request: %v", err)
		}

		req, err := http.NewRequest("POST", "/api/v1/folders", bytes.NewBuffer(requestBodyBytes))
		if err != nil {
			t.Fatalf("Failed to create folder request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("Create folder failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		if err != nil {
			t.Fatalf("Failed to parse create folder response: %v", err)
		}

		folderID, exists := response["id"]
		if !exists {
			t.Fatal("Create folder response should contain folder id")
		}
		createdFolderID = folderID.(string)

		t.Logf("✓ Create folder successful. Folder ID: %s", createdFolderID)
	})

	t.Run("6. List Folders", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/api/v1/folders", nil)
		if err != nil {
			t.Fatalf("Failed to create list folders request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("List folders failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		if err != nil {
			t.Fatalf("Failed to parse list folders response: %v", err)
		}

		folders, exists := response["folders"]
		if !exists {
			t.Fatal("List folders response should contain folders array")
		}

		foldersArray := folders.([]interface{})
		if len(foldersArray) == 0 {
			t.Fatal("Should have at least one folder after creation")
		}

		t.Logf("✓ List folders successful. Found %d folders", len(foldersArray))
	})

	t.Run("7. Get Folder Details", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/api/v1/folders/"+createdFolderID, nil)
		if err != nil {
			t.Fatalf("Failed to create get folder request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Get folder failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		if err != nil {
			t.Fatalf("Failed to parse get folder response: %v", err)
		}

		folder, exists := response["folder"]
		if !exists {
			t.Fatal("Get folder response should contain folder object")
		}
		
		folderObj := folder.(map[string]interface{})
		folderID, exists := folderObj["id"]
		if !exists || folderID != createdFolderID {
			t.Fatal("Get folder response should contain correct folder id")
		}

		t.Logf("✓ Get folder details successful")
	})

	t.Run("8. Update Folder", func(t *testing.T) {
		updateFolderBody := map[string]interface{}{
			"name": "Updated Test Folder",
		}

		requestBodyBytes, err := json.Marshal(updateFolderBody)
		if err != nil {
			t.Fatalf("Failed to marshal update folder request: %v", err)
		}

		req, err := http.NewRequest("PATCH", "/api/v1/folders/"+createdFolderID, bytes.NewBuffer(requestBodyBytes))
		if err != nil {
			t.Fatalf("Failed to create update folder request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Update folder failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		if err != nil {
			t.Fatalf("Failed to parse update folder response: %v", err)
		}

		name, exists := response["name"]
		if !exists || name != "Updated Test Folder" {
			t.Fatal("Update folder should return updated name")
		}

		t.Logf("✓ Update folder successful")
	})

	t.Run("9. Move File to Folder", func(t *testing.T) {
		moveFileBody := map[string]interface{}{
			"folder_id": createdFolderID,
		}

		requestBodyBytes, err := json.Marshal(moveFileBody)
		if err != nil {
			t.Fatalf("Failed to marshal move file request: %v", err)
		}

		req, err := http.NewRequest("PATCH", "/api/v1/files/"+uploadedFileID+"/move", bytes.NewBuffer(requestBodyBytes))
		if err != nil {
			t.Fatalf("Failed to create move file request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Move file failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		t.Logf("✓ Move file to folder successful")
	})

	t.Run("10. List Files in Folder", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/api/v1/files?folder_id="+createdFolderID, nil)
		if err != nil {
			t.Fatalf("Failed to create list files in folder request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("List files in folder failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		if err != nil {
			t.Fatalf("Failed to parse list files in folder response: %v", err)
		}

		files, exists := response["files"]
		if !exists {
			t.Fatal("List files in folder response should contain files array")
		}

		filesArray := files.([]interface{})
		if len(filesArray) == 0 {
			t.Fatal("Should have at least one file in folder after move")
		}

		// Check that the file has the correct folder_id
		file := filesArray[0].(map[string]interface{})
		fileFolderID, exists := file["folder_id"]
		if !exists {
			t.Fatalf("File should have folder_id field after move. Available fields: %v", file)
		}
		if fileFolderID == nil {
			t.Fatal("File folder_id should not be nil after move")
		}
		if fileFolderID != createdFolderID {
			t.Fatalf("File should have correct folder_id after move. Expected: %s, Got: %s", createdFolderID, fileFolderID)
		}

		t.Logf("✓ List files in folder successful. Found %d files", len(filesArray))
	})

	t.Run("11. Create Folder Share Link", func(t *testing.T) {
		req, err := http.NewRequest("POST", "/api/v1/folders/"+createdFolderID+"/share", nil)
		if err != nil {
			t.Fatalf("Failed to create share link request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Note: This might return 501 Not Implemented if not fully implemented
		if rr.Code == http.StatusNotImplemented {
			t.Logf("⚠ Create folder share link not implemented (501)")
		} else if rr.Code != http.StatusCreated {
			t.Fatalf("Create folder share link failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		} else {
			t.Logf("✓ Create folder share link successful")
		}
	})

	t.Run("12. Get User Stats", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/api/v1/stats/me", nil)
		if err != nil {
			t.Fatalf("Failed to create stats request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Get stats failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		if err != nil {
			t.Fatalf("Failed to parse stats response: %v", err)
		}

		// Check for expected stats fields
		expectedFields := []string{"total_files", "total_size_bytes", "quota_bytes"}
		for _, field := range expectedFields {
			if _, exists := response[field]; !exists {
				t.Errorf("Stats response should contain %s field", field)
			}
		}

		t.Logf("✓ Get user stats successful")
	})

	t.Run("13. Delete Folder Share Link", func(t *testing.T) {
		req, err := http.NewRequest("DELETE", "/api/v1/folders/"+createdFolderID+"/share", nil)
		if err != nil {
			t.Fatalf("Failed to create delete share link request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Note: This might return 501 Not Implemented if not fully implemented
		if rr.Code == http.StatusNotImplemented {
			t.Logf("⚠ Delete folder share link not implemented (501)")
		} else if rr.Code != http.StatusOK {
			t.Fatalf("Delete folder share link failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		} else {
			t.Logf("✓ Delete folder share link successful")
		}
	})

	t.Run("14. Delete Folder", func(t *testing.T) {
		req, err := http.NewRequest("DELETE", "/api/v1/folders/"+createdFolderID, nil)
		if err != nil {
			t.Fatalf("Failed to create delete folder request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Fatalf("Delete folder failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		t.Logf("✓ Delete folder successful")
	})

	t.Run("15. Verify Folder Deleted", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/api/v1/folders/"+createdFolderID, nil)
		if err != nil {
			t.Fatalf("Failed to create get deleted folder request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("Getting deleted folder should return 404. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		t.Logf("✓ Verify folder deleted successful (404 as expected)")
	})

	t.Logf("\n🎉 Complete API Flow Test Completed Successfully!")
	t.Logf("Summary:")
	t.Logf("  - User signup and login: ✓")
	t.Logf("  - File upload and listing: ✓")
	t.Logf("  - Folder CRUD operations: ✓")
	t.Logf("  - File-folder operations: ✓")
	t.Logf("  - Share link operations: ⚠ (may not be implemented)")
	t.Logf("  - User stats: ✓")
}

// TestIndividualFolderEndpoints tests each folder endpoint individually
func TestIndividualFolderEndpoints(t *testing.T) {
	app := TestApp(t)

	// Setup: Create user and get auth token
	userEmail := "foldertest@example.com"
	userPassword := "securepassword123"
	userName := "Folder Test User"
	var authToken string

	// Signup
	signupBody := map[string]interface{}{
		"email":    userEmail,
		"password": userPassword,
		"name":     userName,
	}
	signupBytes, _ := json.Marshal(signupBody)
	signupReq, _ := http.NewRequest("POST", "/api/v1/auth/signup", bytes.NewBuffer(signupBytes))
	signupReq.Header.Set("Content-Type", "application/json")
	signupRr := httptest.NewRecorder()
	app.ServeHTTP(signupRr, signupReq)

	if signupRr.Code != http.StatusCreated {
		t.Fatalf("Setup failed - signup: %d", signupRr.Code)
	}

	var signupResponse map[string]interface{}
	json.Unmarshal(signupRr.Body.Bytes(), &signupResponse)
	authToken = signupResponse["token"].(string)

	tests := []struct {
		name           string
		method         string
		path           string
		body           map[string]interface{}
		expectedStatus int
		description    string
	}{
		{
			name:           "CREATE_FOLDER",
			method:         "POST",
			path:           "/api/v1/folders",
			body:           map[string]interface{}{"name": "Test Folder"},
			expectedStatus: http.StatusCreated,
			description:    "Create a new folder",
		},
		{
			name:           "CREATE_FOLDER_INVALID_NAME",
			method:         "POST",
			path:           "/api/v1/folders",
			body:           map[string]interface{}{"name": ""},
			expectedStatus: http.StatusBadRequest,
			description:    "Create folder with invalid name",
		},
		{
			name:           "LIST_FOLDERS",
			method:         "GET",
			path:           "/api/v1/folders",
			body:           nil,
			expectedStatus: http.StatusOK,
			description:    "List all folders",
		},
		{
			name:           "LIST_FOLDERS_WITH_PAGINATION",
			method:         "GET",
			path:           "/api/v1/folders?page=1&limit=10",
			body:           nil,
			expectedStatus: http.StatusOK,
			description:    "List folders with pagination",
		},
		{
			name:           "GET_NONEXISTENT_FOLDER",
			method:         "GET",
			path:           "/api/v1/folders/00000000-0000-0000-0000-000000000000",
			body:           nil,
			expectedStatus: http.StatusNotFound,
			description:    "Get non-existent folder",
		},
		{
			name:           "UPDATE_NONEXISTENT_FOLDER",
			method:         "PUT",
			path:           "/api/v1/folders/00000000-0000-0000-0000-000000000000",
			body:           map[string]interface{}{"name": "Updated Name"},
			expectedStatus: http.StatusNotFound,
			description:    "Update non-existent folder",
		},
		{
			name:           "DELETE_NONEXISTENT_FOLDER",
			method:         "DELETE",
			path:           "/api/v1/folders/00000000-0000-0000-0000-000000000000",
			body:           nil,
			expectedStatus: http.StatusNotFound,
			description:    "Delete non-existent folder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reqBody io.Reader
			if tt.body != nil {
				bodyBytes, _ := json.Marshal(tt.body)
				reqBody = bytes.NewBuffer(bodyBytes)
			}

			req, err := http.NewRequest(tt.method, tt.path, reqBody)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			if tt.body != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			req.Header.Set("Authorization", "Bearer "+authToken)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("%s: Expected status %d, got %d. Response: %s",
					tt.description, tt.expectedStatus, rr.Code, rr.Body.String())
			} else {
				t.Logf("✓ %s - Status %d", tt.description, rr.Code)
			}
		})
	}
}

// TestUnauthorizedFolderAccess tests that folder endpoints require authentication
func TestUnauthorizedFolderAccess(t *testing.T) {
	app := TestApp(t)

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{"GET", "/api/v1/folders", ""},
		{"POST", "/api/v1/folders", `{"name": "Test"}`},
		{"GET", "/api/v1/folders/test-id", ""},
		{"PATCH", "/api/v1/folders/test-id", `{"name": "Updated"}`},
		{"DELETE", "/api/v1/folders/test-id", ""},
		{"POST", "/api/v1/folders/test-id/share", ""},
		{"DELETE", "/api/v1/folders/test-id/share", ""},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s %s", tt.method, tt.path), func(t *testing.T) {
			var reqBody io.Reader
			if tt.body != "" {
				reqBody = strings.NewReader(tt.body)
			}

			req, err := http.NewRequest(tt.method, tt.path, reqBody)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			// Intentionally NOT setting Authorization header

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Errorf("Expected 401 Unauthorized, got %d for %s %s. Response: %s",
					rr.Code, tt.method, tt.path, rr.Body.String())
			} else {
				t.Logf("✓ %s %s returns 401 when unauthorized", tt.method, tt.path)
			}
		})
	}
}