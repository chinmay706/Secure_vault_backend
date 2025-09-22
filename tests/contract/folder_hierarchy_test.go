package contract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestFolderHierarchy tests the hierarchical folder structure with parent_id relationships
func TestFolderHierarchy(t *testing.T) {
	app := TestApp(t)

	// Setup: Create user and get auth token
	userEmail := "hierarchy@example.com"
	userPassword := "securepassword123"
	userName := "Hierarchy Test User"
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

	// Test folder hierarchy creation and navigation
	var rootFolderID, subFolderID, subSubFolderID string

	t.Run("1. Create Root Folder", func(t *testing.T) {
		createBody := map[string]interface{}{
			"name": "Documents",
		}
		bodyBytes, _ := json.Marshal(createBody)

		req, _ := http.NewRequest("POST", "/api/v1/folders", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("Create root folder failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		json.Unmarshal(rr.Body.Bytes(), &response)
		rootFolderID = response["id"].(string)

		t.Logf("✓ Root folder created: %s", rootFolderID)
	})

	t.Run("2. Create Subfolder", func(t *testing.T) {
		createBody := map[string]interface{}{
			"name":      "Work",
			"parent_id": rootFolderID,
		}
		bodyBytes, _ := json.Marshal(createBody)

		req, _ := http.NewRequest("POST", "/api/v1/folders", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("Create subfolder failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		json.Unmarshal(rr.Body.Bytes(), &response)
		subFolderID = response["id"].(string)

		// Verify parent_id is set correctly
		if response["parent_id"] != rootFolderID {
			t.Fatalf("Subfolder parent_id incorrect. Expected: %s, Got: %v", rootFolderID, response["parent_id"])
		}

		t.Logf("✓ Subfolder created: %s under parent: %s", subFolderID, rootFolderID)
	})

	t.Run("3. Create Sub-subfolder", func(t *testing.T) {
		createBody := map[string]interface{}{
			"name":      "Projects",
			"parent_id": subFolderID,
		}
		bodyBytes, _ := json.Marshal(createBody)

		req, _ := http.NewRequest("POST", "/api/v1/folders", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("Create sub-subfolder failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		json.Unmarshal(rr.Body.Bytes(), &response)
		subSubFolderID = response["id"].(string)

		// Verify parent_id is set correctly
		if response["parent_id"] != subFolderID {
			t.Fatalf("Sub-subfolder parent_id incorrect. Expected: %s, Got: %v", subFolderID, response["parent_id"])
		}

		t.Logf("✓ Sub-subfolder created: %s under parent: %s", subSubFolderID, subFolderID)
	})

	t.Run("4. List Root Level Folders", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/v1/folders", nil)
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("List root folders failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		json.Unmarshal(rr.Body.Bytes(), &response)

		folders := response["folders"].([]interface{})
		if len(folders) != 1 {
			t.Fatalf("Expected 1 root folder, got %d", len(folders))
		}

		folder := folders[0].(map[string]interface{})
		if folder["id"] != rootFolderID {
			t.Fatalf("Root folder ID mismatch. Expected: %s, Got: %s", rootFolderID, folder["id"])
		}

		// Root folder should have nil parent_id (omitted from JSON)
		if parentID, exists := folder["parent_id"]; exists && parentID != nil {
			t.Fatalf("Root folder should have no parent_id, but got: %v", parentID)
		}

		t.Logf("✓ Root level listing correct - found 1 root folder")
	})

	t.Run("5. List Subfolders", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/v1/folders?parent_id="+rootFolderID, nil)
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("List subfolders failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		json.Unmarshal(rr.Body.Bytes(), &response)

		folders := response["folders"].([]interface{})
		if len(folders) != 1 {
			t.Fatalf("Expected 1 subfolder, got %d", len(folders))
		}

		folder := folders[0].(map[string]interface{})
		if folder["id"] != subFolderID {
			t.Fatalf("Subfolder ID mismatch. Expected: %s, Got: %s", subFolderID, folder["id"])
		}

		if folder["parent_id"] != rootFolderID {
			t.Fatalf("Subfolder parent_id mismatch. Expected: %s, Got: %v", rootFolderID, folder["parent_id"])
		}

		t.Logf("✓ Subfolder listing correct - found 1 child folder")
	})

	t.Run("6. Test Breadcrumbs", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/v1/folders/"+subSubFolderID, nil)
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Get folder details failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		json.Unmarshal(rr.Body.Bytes(), &response)

		// Check breadcrumbs structure
		breadcrumbs, exists := response["breadcrumbs"]
		if !exists {
			t.Fatal("Response should contain breadcrumbs")
		}

		breadcrumbsArray := breadcrumbs.([]interface{})
		if len(breadcrumbsArray) != 3 {
			t.Fatalf("Expected 3 breadcrumbs (Documents -> Work -> Projects), got %d", len(breadcrumbsArray))
		}

		// Verify breadcrumb order (root -> current)
		crumb1 := breadcrumbsArray[0].(map[string]interface{})
		crumb2 := breadcrumbsArray[1].(map[string]interface{})
		crumb3 := breadcrumbsArray[2].(map[string]interface{})

		if crumb1["name"] != "Documents" || crumb1["id"] != rootFolderID {
			t.Fatalf("First breadcrumb should be Documents (%s), got %s (%s)", rootFolderID, crumb1["name"], crumb1["id"])
		}

		if crumb2["name"] != "Work" || crumb2["id"] != subFolderID {
			t.Fatalf("Second breadcrumb should be Work (%s), got %s (%s)", subFolderID, crumb2["name"], crumb2["id"])
		}

		if crumb3["name"] != "Projects" || crumb3["id"] != subSubFolderID {
			t.Fatalf("Third breadcrumb should be Projects (%s), got %s (%s)", subSubFolderID, crumb3["name"], crumb3["id"])
		}

		t.Logf("✓ Breadcrumbs correct: Documents -> Work -> Projects")
	})

	t.Run("7. Test Folder Deletion with Hierarchy", func(t *testing.T) {
		// Delete middle folder should cascade
		req, _ := http.NewRequest("DELETE", "/api/v1/folders/"+subFolderID, nil)
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Fatalf("Delete folder failed. Status: %d, Response: %s", rr.Code, rr.Body.String())
		}

		// Verify sub-subfolder is also deleted (CASCADE)
		req, _ = http.NewRequest("GET", "/api/v1/folders/"+subSubFolderID, nil)
		req.Header.Set("Authorization", "Bearer "+authToken)

		rr = httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("Sub-subfolder should be deleted due to CASCADE. Status: %d", rr.Code)
		}

		t.Logf("✓ Cascading delete works correctly")
	})

	t.Logf("\n🎉 Folder Hierarchy Test Completed Successfully!")
	t.Logf("Summary:")
	t.Logf("  - Root folder creation: ✓")
	t.Logf("  - Nested folder creation: ✓")
	t.Logf("  - Parent-child relationships: ✓")
	t.Logf("  - Hierarchical listing: ✓")
	t.Logf("  - Breadcrumb navigation: ✓")
	t.Logf("  - Cascading deletion: ✓")
}

// TestFolderParentIdValidation tests parent_id validation and edge cases
func TestFolderParentIdValidation(t *testing.T) {
	app := TestApp(t)

	// Setup: Create user and get auth token
	userEmail := "validation@example.com"
	userPassword := "securepassword123"
	userName := "Validation Test User"
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

	var signupResponse map[string]interface{}
	json.Unmarshal(signupRr.Body.Bytes(), &signupResponse)
	authToken = signupResponse["token"].(string)

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
		description    string
	}{
		{
			name:           "ROOT_FOLDER_NULL_PARENT",
			requestBody:    map[string]interface{}{"name": "Root1", "parent_id": nil},
			expectedStatus: http.StatusCreated,
			description:    "Create root folder with explicit null parent_id",
		},
		{
			name:           "ROOT_FOLDER_OMIT_PARENT",
			requestBody:    map[string]interface{}{"name": "Root2"},
			expectedStatus: http.StatusCreated,
			description:    "Create root folder by omitting parent_id",
		},
		{
			name:           "INVALID_UUID_PARENT",
			requestBody:    map[string]interface{}{"name": "Invalid", "parent_id": "not-a-uuid"},
			expectedStatus: http.StatusBadRequest,
			description:    "Reject invalid UUID format for parent_id",
		},
		{
			name:           "EMPTY_STRING_PARENT",
			requestBody:    map[string]interface{}{"name": "Empty", "parent_id": ""},
			expectedStatus: http.StatusBadRequest,
			description:    "Reject empty string as parent_id",
		},
		{
			name:           "NONEXISTENT_PARENT",
			requestBody:    map[string]interface{}{"name": "Orphan", "parent_id": "00000000-0000-0000-0000-000000000000"},
			expectedStatus: http.StatusNotFound,
			description:    "Reject nonexistent parent folder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.requestBody)

			req, _ := http.NewRequest("POST", "/api/v1/folders", bytes.NewBuffer(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
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