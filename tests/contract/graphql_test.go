package contract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GraphQLRequest represents a GraphQL request
type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// GraphQLResponse represents a GraphQL response
type GraphQLResponse struct {
	Data   interface{}              `json:"data"`
	Errors []map[string]interface{} `json:"errors,omitempty"`
}

// TestGraphQLBasicQueries tests basic GraphQL queries
func TestGraphQLBasicQueries(t *testing.T) {
	app := TestApp(t)

	// Setup: Create user and get auth token
	userEmail := "graphql@example.com"
	userPassword := "securepassword123"
	userName := "GraphQL Test User"
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

	require.Equal(t, http.StatusCreated, signupRr.Code, "Signup should succeed")

	var signupResponse map[string]interface{}
	err := json.Unmarshal(signupRr.Body.Bytes(), &signupResponse)
	require.NoError(t, err)
	authToken = signupResponse["token"].(string)

	t.Run("Hello Query", func(t *testing.T) {
		query := GraphQLRequest{
			Query: "query { hello }",
		}
		response := executeGraphQLQuery(t, app, query, authToken)
		
		assert.Nil(t, response.Errors, "Should not have errors")
		assert.NotNil(t, response.Data, "Should have data")
		
		data := response.Data.(map[string]interface{})
		assert.Contains(t, data, "hello", "Should contain hello field")
	})

	t.Run("Me Query", func(t *testing.T) {
		query := GraphQLRequest{
			Query: "query { me { id email role created_at } }",
		}
		response := executeGraphQLQuery(t, app, query, authToken)
		
		assert.Nil(t, response.Errors, "Should not have errors")
		assert.NotNil(t, response.Data, "Should have data")
		
		data := response.Data.(map[string]interface{})
		me := data["me"].(map[string]interface{})
		assert.Equal(t, userEmail, me["email"], "Should return correct user email")
		assert.Contains(t, me, "id", "Should contain user ID")
		assert.Contains(t, me, "role", "Should contain user role")
	})

	t.Run("Files Query", func(t *testing.T) {
		query := GraphQLRequest{
			Query: `query { 
				files(folder_id: "root", page: 1, page_size: 10) { 
					files { 
						id 
						original_filename 
						mime_type 
					} 
					page
					page_size
					total
				} 
			}`,
		}
		response := executeGraphQLQuery(t, app, query, authToken)
		
		assert.Nil(t, response.Errors, "Should not have errors")
		assert.NotNil(t, response.Data, "Should have data")
		
		data := response.Data.(map[string]interface{})
		files := data["files"].(map[string]interface{})
		assert.Contains(t, files, "files", "Should contain files array")
		assert.Contains(t, files, "page", "Should contain page info")
		assert.Contains(t, files, "page_size", "Should contain page_size info")
		assert.Contains(t, files, "total", "Should contain total count")
	})
}

// TestGraphQLFolderOperations tests folder CRUD operations via GraphQL
func TestGraphQLFolderOperations(t *testing.T) {
	app := TestApp(t)

	// Setup: Create user and get auth token
	userEmail := "folderops@example.com"
	userPassword := "securepassword123"
	userName := "Folder Ops Test User"
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

	require.Equal(t, http.StatusCreated, signupRr.Code, "Signup should succeed")

	var signupResponse map[string]interface{}
	err := json.Unmarshal(signupRr.Body.Bytes(), &signupResponse)
	require.NoError(t, err)
	authToken = signupResponse["token"].(string)

	var testFolderID string

	t.Run("Create Folder", func(t *testing.T) {
		query := GraphQLRequest{
			Query: `mutation CreateFolder($name: String!) { 
				createFolder(name: $name) { 
					id 
					name 
					created_at 
				} 
			}`,
			Variables: map[string]interface{}{
				"name": "GraphQL Test Folder",
			},
		}
		response := executeGraphQLQuery(t, app, query, authToken)
		
		assert.Nil(t, response.Errors, "Should not have errors")
		assert.NotNil(t, response.Data, "Should have data")
		
		data := response.Data.(map[string]interface{})
		createFolder := data["createFolder"].(map[string]interface{})
		assert.Equal(t, "GraphQL Test Folder", createFolder["name"], "Should have correct folder name")
		assert.Contains(t, createFolder, "id", "Should contain folder ID")
		assert.Contains(t, createFolder, "created_at", "Should contain created_at")
		
		testFolderID = createFolder["id"].(string)
		assert.NotEmpty(t, testFolderID, "Folder ID should not be empty")
	})

	t.Run("List Folders", func(t *testing.T) {
		query := GraphQLRequest{
			Query: `query { 
				folders { 
					folders { 
						id 
						name 
						created_at 
					} 
					files { 
						id 
						original_filename 
					} 
					pagination { 
						page 
						page_size
						total_folders
						total_files
					} 
				} 
			}`,
		}
		response := executeGraphQLQuery(t, app, query, authToken)
		
		assert.Nil(t, response.Errors, "Should not have errors")
		assert.NotNil(t, response.Data, "Should have data")
		
		data := response.Data.(map[string]interface{})
		folders := data["folders"].(map[string]interface{})
		assert.Contains(t, folders, "folders", "Should contain folders array")
		assert.Contains(t, folders, "files", "Should contain files array")
		assert.Contains(t, folders, "pagination", "Should contain pagination")
		
		// Check if our created folder is in the list
		foldersList := folders["folders"].([]interface{})
		found := false
		for _, folder := range foldersList {
			folderMap := folder.(map[string]interface{})
			if folderMap["id"] == testFolderID {
				found = true
				assert.Equal(t, "GraphQL Test Folder", folderMap["name"], "Should find our test folder")
				break
			}
		}
		assert.True(t, found, "Should find our created folder in the list")
	})

	t.Run("Get Folder Details", func(t *testing.T) {
		if testFolderID == "" {
			t.Skip("No folder ID available from create test")
		}

		query := GraphQLRequest{
			Query: `query GetFolder($id: UUID!) { 
				folder(id: $id) { 
					folder { 
						id 
						name 
						created_at 
					} 
					breadcrumbs { 
						id 
						name 
					} 
				} 
			}`,
			Variables: map[string]interface{}{
				"id": testFolderID,
			},
		}
		response := executeGraphQLQuery(t, app, query, authToken)
		
		assert.Nil(t, response.Errors, "Should not have errors")
		assert.NotNil(t, response.Data, "Should have data")
		
		data := response.Data.(map[string]interface{})
		folderData := data["folder"].(map[string]interface{})
		folder := folderData["folder"].(map[string]interface{})
		
		assert.Equal(t, testFolderID, folder["id"], "Should return correct folder ID")
		assert.Equal(t, "GraphQL Test Folder", folder["name"], "Should return correct folder name")
		assert.Contains(t, folderData, "breadcrumbs", "Should contain breadcrumbs")
	})

	t.Run("Update Folder", func(t *testing.T) {
		if testFolderID == "" {
			t.Skip("No folder ID available from create test")
		}

		query := GraphQLRequest{
			Query: `mutation UpdateFolder($id: UUID!, $name: String!) { 
				updateFolder(id: $id, name: $name) { 
					id 
					name 
				} 
			}`,
			Variables: map[string]interface{}{
				"id":   testFolderID,
				"name": "Updated GraphQL Test Folder",
			},
		}
		response := executeGraphQLQuery(t, app, query, authToken)
		
		assert.Nil(t, response.Errors, "Should not have errors")
		assert.NotNil(t, response.Data, "Should have data")
		
		data := response.Data.(map[string]interface{})
		updateFolder := data["updateFolder"].(map[string]interface{})
		assert.Equal(t, testFolderID, updateFolder["id"], "Should return correct folder ID")
		assert.Equal(t, "Updated GraphQL Test Folder", updateFolder["name"], "Should have updated name")
	})

	t.Run("Delete Folder", func(t *testing.T) {
		if testFolderID == "" {
			t.Skip("No folder ID available from create test")
		}

		query := GraphQLRequest{
			Query: `mutation DeleteFolder($id: UUID!) { 
				deleteFolder(id: $id) 
			}`,
			Variables: map[string]interface{}{
				"id": testFolderID,
			},
		}
		response := executeGraphQLQuery(t, app, query, authToken)
		
		assert.Nil(t, response.Errors, "Should not have errors")
		assert.NotNil(t, response.Data, "Should have data")
		
		data := response.Data.(map[string]interface{})
		assert.True(t, data["deleteFolder"].(bool), "Should return true for successful deletion")
	})
}

// TestGraphQLErrorHandling tests GraphQL error scenarios
func TestGraphQLErrorHandling(t *testing.T) {
	app := TestApp(t)

	// Setup: Create user and get auth token
	userEmail := "errortest@example.com"
	userPassword := "securepassword123"
	userName := "Error Test User"
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

	require.Equal(t, http.StatusCreated, signupRr.Code, "Signup should succeed")

	var signupResponse map[string]interface{}
	err := json.Unmarshal(signupRr.Body.Bytes(), &signupResponse)
	require.NoError(t, err)
	authToken = signupResponse["token"].(string)

	t.Run("Invalid UUID Format", func(t *testing.T) {
		query := GraphQLRequest{
			Query: `query GetFolder($id: UUID!) { 
				folder(id: $id) { 
					folder { 
						id 
						name 
					} 
				} 
			}`,
			Variables: map[string]interface{}{
				"id": "invalid-uuid-format",
			},
		}
		response := executeGraphQLQuery(t, app, query, authToken)
		
		// Should have validation errors for invalid UUID
		assert.NotNil(t, response.Errors, "Should have validation errors")
		assert.NotEmpty(t, response.Errors, "Should have at least one error")
	})

	t.Run("Non-existent Folder", func(t *testing.T) {
		query := GraphQLRequest{
			Query: `query GetFolder($id: UUID!) { 
				folder(id: $id) { 
					folder { 
						id 
						name 
					} 
				} 
			}`,
			Variables: map[string]interface{}{
				"id": "550e8400-e29b-41d4-a716-446655440000", // Valid UUID format but non-existent
			},
		}
		response := executeGraphQLQuery(t, app, query, authToken)
		
		// Should handle non-existent folder gracefully
		if response.Errors != nil {
			// Errors are acceptable for non-existent resources
			assert.NotEmpty(t, response.Errors, "Should have error for non-existent folder")
		} else {
			// Or data should be null/empty
			assert.NotNil(t, response.Data, "Should have data field even if null")
		}
	})

	t.Run("Missing Required Fields", func(t *testing.T) {
		query := GraphQLRequest{
			Query: `mutation CreateFolder { 
				createFolder { 
					id 
					name 
				} 
			}`, // Missing required name parameter
		}
		response := executeGraphQLQuery(t, app, query, authToken)
		
		// Should have validation errors for missing required field
		assert.NotNil(t, response.Errors, "Should have validation errors for missing required field")
		assert.NotEmpty(t, response.Errors, "Should have at least one error")
	})
}

// TestGraphQLUnauthenticated tests GraphQL without authentication
func TestGraphQLUnauthenticated(t *testing.T) {
	app := TestApp(t)

	t.Run("Query Without Auth", func(t *testing.T) {
		query := GraphQLRequest{
			Query: "query { me { id email } }",
		}
		response := executeGraphQLQuery(t, app, query, "") // No auth token
		
		// Should have authentication error
		assert.NotNil(t, response.Errors, "Should have authentication error")
		assert.NotEmpty(t, response.Errors, "Should have at least one error")
	})
}

// Helper function to execute GraphQL queries
func executeGraphQLQuery(t *testing.T, app http.Handler, query GraphQLRequest, authToken string) *GraphQLResponse {
	queryBytes, err := json.Marshal(query)
	require.NoError(t, err, "Should marshal GraphQL query")

	req, err := http.NewRequest("POST", "/api/v1/graphql", bytes.NewBuffer(queryBytes))
	require.NoError(t, err, "Should create HTTP request")

	req.Header.Set("Content-Type", "application/json")
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)

	// GraphQL always returns 200, even for errors
	assert.Equal(t, http.StatusOK, rr.Code, "GraphQL should return 200 status")

	var response GraphQLResponse
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err, "Should unmarshal GraphQL response")

	return &response
}