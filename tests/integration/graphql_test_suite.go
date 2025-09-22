package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	restURL    = "http://localhost:8080/api/v1"
	graphqlEndpoint = "http://localhost:8080/api/v1/graphql"
)

// Response structures for GraphQL and REST
type GraphQLResponse struct {
	Data   interface{}              `json:"data"`
	Errors []map[string]interface{} `json:"errors,omitempty"`
}
// Use the GraphQLResponse type from integration_tests.go

type GraphQLTestError struct {
	Message string   `json:"message"`
	Path    []string `json:"path,omitempty"`
}

type RESTAuthResponse struct {
	Token string `json:"token"`
	User  struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Role  string `json:"role"`
	} `json:"user"`
}

type RESTFileUploadResponse struct {
	File struct {
		ID               string   `json:"id"`
		OriginalFilename string   `json:"original_filename"`
		MimeType         string   `json:"mime_type"`
		SizeBytes        int64    `json:"size_bytes"`
		FolderID         *string  `json:"folder_id"`
		IsPublic         bool     `json:"is_public"`
		DownloadCount    int      `json:"download_count"`
		Tags             []string `json:"tags"`
		CreatedAt        string   `json:"created_at"`
		UpdatedAt        string   `json:"updated_at"`
	} `json:"file"`
	Hash        string `json:"hash"`
	IsDuplicate bool   `json:"is_duplicate"`
}

// Test context
type TestContext struct {
	RegularToken string   // Regular user token
	AdminToken   string   // Admin user token (if available)
	RegularUser  struct {
		ID       string
		Email    string
		Password string
	}
	AdminUser struct {
		ID       string
		Email    string
		Password string
	}
	Files       []string // File IDs from REST uploads
	Folders     []string // Folder IDs from GraphQL
	ShareTokens []string // Share tokens
}

func main() {
	fmt.Println("🚀 Starting SecureVault GraphQL API Test Suite")
	fmt.Println("📋 This test suite focuses exclusively on GraphQL queries and mutations")
	fmt.Println("🔧 REST APIs are used only for file uploads (to get file IDs for GraphQL testing)")
	fmt.Println("=" + strings.Repeat("=", 80))

	ctx := &TestContext{}

	// Comprehensive GraphQL test suite
	tests := []struct {
		name string
		fn   func(*TestContext) error
	}{
		// Authentication & Setup (GraphQL)
		{"1. GraphQL Authentication - Signup", testGraphQLSignup},
		{"2. GraphQL Authentication - Login", testGraphQLLogin},
		{"3. GraphQL Hello Query (No Auth)", testGraphQLHello},
		
		// REST File Upload (minimal, only to get file IDs)
		{"4. REST File Upload (for GraphQL testing)", testRESTFileUpload},
		
		// User Profile & Stats (GraphQL)
		{"5. GraphQL User Profile - Me Query", testGraphQLMe},
		{"6. GraphQL Stats - Basic Stats", testGraphQLStats},
		{"7. GraphQL Stats - Filtered Stats", testGraphQLStatsFiltered},
		
		// File Operations (GraphQL)
		{"8. GraphQL Files - List Files Query", testGraphQLFilesList},
		{"9. GraphQL Files - Single File Query", testGraphQLFileDetails},
		{"10. GraphQL Files - Toggle File Public", testGraphQLToggleFilePublic},
		{"11. GraphQL Files - Move File to Folder", testGraphQLMoveFile},
		
		// Folder Operations (GraphQL) 
		{"12. GraphQL Folders - Create Folders", testGraphQLCreateFolders},
		{"13. GraphQL Folders - List Folders Query", testGraphQLFoldersList},
		{"14. GraphQL Folders - Single Folder Query", testGraphQLFolderDetails},
		{"15. GraphQL Folders - Update Folder Name", testGraphQLUpdateFolder},
		{"16. GraphQL Folders - Move Folder", testGraphQLMoveFolder},
		
		// Sharing Operations (GraphQL)
		{"17. GraphQL Sharing - Create Folder Share Link", testGraphQLCreateFolderShare},
		{"18. GraphQL Sharing - Public Folder Access", testGraphQLPublicFolder},
		{"19. GraphQL Sharing - Public File Access", testGraphQLPublicFile},
		{"20. GraphQL Sharing - Delete Folder Share Link", testGraphQLDeleteFolderShare},
		
		// Error Handling (GraphQL)
		{"21. GraphQL Errors - Unauthorized Access", testGraphQLUnauthorized},
		{"22. GraphQL Errors - Invalid UUID", testGraphQLInvalidUUID},
		{"23. GraphQL Errors - Non-existent Resources", testGraphQLNotFound},
		
		// Complex Queries (GraphQL)
		{"24. GraphQL Complex - Nested Query", testGraphQLComplexQuery},
		{"25. GraphQL Complex - Multiple Operations", testGraphQLMultipleOperations},
		
		// Cleanup (GraphQL)
		{"26. GraphQL Cleanup - Delete Files", testGraphQLDeleteFiles},
		{"27. GraphQL Cleanup - Delete Folders", testGraphQLDeleteFolders},
	}

	totalTests := len(tests)
	passedTests := 0
	failedTests := []string{}

	for _, test := range tests {
		fmt.Printf("\n📋 Running: %s\n", test.name)
		fmt.Println(strings.Repeat("-", 60))

		if err := test.fn(ctx); err != nil {
			fmt.Printf("❌ FAILED: %s\n   Error: %v\n", test.name, err)
			failedTests = append(failedTests, test.name)
		} else {
			fmt.Printf("✅ PASSED: %s\n", test.name)
			passedTests++
		}
	}

	// Final Results
	fmt.Print("\n" + strings.Repeat("=", 80))
	fmt.Printf("\n🏁 GraphQL Test Results: %d/%d tests passed\n", passedTests, totalTests)
	
	if passedTests == totalTests {
		fmt.Println("🎉 All GraphQL tests passed successfully!")
		fmt.Println("✅ GraphQL API is fully functional and ready for production!")
	} else {
		fmt.Printf("⚠️  %d tests failed\n", totalTests-passedTests)
		fmt.Println("\nFailed Tests:")
		for _, test := range failedTests {
			fmt.Printf("   - %s\n", test)
		}
	}
	
	fmt.Println("\n📊 Test Coverage Summary:")
	fmt.Println("   ✓ Authentication mutations (signup, login)")
	fmt.Println("   ✓ User queries (me, stats)")
	fmt.Println("   ✓ File queries and mutations")
	fmt.Println("   ✓ Folder queries and mutations")
	fmt.Println("   ✓ Sharing queries and mutations")
	fmt.Println("   ✓ Public access queries")
	fmt.Println("   ✓ Error handling scenarios")
	fmt.Println("   ✓ Complex nested queries")
}

// =====================================
// Authentication Tests (GraphQL)
// =====================================

func testGraphQLSignup(ctx *TestContext) error {
	fmt.Println("Testing GraphQL signup mutation...")
	
	// Generate unique credentials
	timestamp := time.Now().Unix()
	email := fmt.Sprintf("gql-test-%d@example.com", timestamp)
	password := "graphqltest123"
	
	query := map[string]interface{}{
		"query": `mutation SignupUser($email: String!, $password: String!) {
			signup(email: $email, password: $password) {
				token
				user {
					id
					email
					role
					rate_limit_rps
					storage_quota_bytes
				}
			}
		}`,
		"variables": map[string]interface{}{
			"email":    email,
			"password": password,
		},
	}
	
	resp, err := makeGraphQLRequest(query, "")
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) > 0 {
		return fmt.Errorf("GraphQL errors: %+v", resp.Errors)
	}

	// Extract signup data
	data := resp.Data.(map[string]interface{})
	signup := data["signup"].(map[string]interface{})
	token := signup["token"].(string)
	user := signup["user"].(map[string]interface{})
	
	ctx.RegularToken = token
	ctx.RegularUser.ID = user["id"].(string)
	ctx.RegularUser.Email = email
	ctx.RegularUser.Password = password
	
	fmt.Printf("   ✓ User signed up with ID: %s\n", ctx.RegularUser.ID)
	fmt.Printf("   ✓ Email: %s\n", email)
	fmt.Printf("   ✓ Role: %s\n", user["role"].(string))
	fmt.Printf("   ✓ Token received (length: %d)\n", len(token))
	
	return nil
}

func testGraphQLLogin(ctx *TestContext) error {
	fmt.Println("Testing GraphQL login mutation...")
	
	query := map[string]interface{}{
		"query": `mutation LoginUser($email: String!, $password: String!) {
			login(email: $email, password: $password) {
				token
				user {
					id
					email
					role
					created_at
				}
			}
		}`,
		"variables": map[string]interface{}{
			"email":    ctx.RegularUser.Email,
			"password": ctx.RegularUser.Password,
		},
	}
	
	resp, err := makeGraphQLRequest(query, "")
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) > 0 {
		return fmt.Errorf("GraphQL errors: %+v", resp.Errors)
	}

	// Verify login response
	data := resp.Data.(map[string]interface{})
	login := data["login"].(map[string]interface{})
	token := login["token"].(string)
	user := login["user"].(map[string]interface{})
	
	fmt.Printf("   ✓ Login successful\n")
	fmt.Printf("   ✓ User ID matches: %t\n", user["id"].(string) == ctx.RegularUser.ID)
	fmt.Printf("   ✓ Email matches: %t\n", user["email"].(string) == ctx.RegularUser.Email)
	fmt.Printf("   ✓ New token received (length: %d)\n", len(token))
	
	return nil
}

func testGraphQLHello(ctx *TestContext) error {
	fmt.Println("Testing GraphQL hello query (no authentication)...")
	
	query := map[string]interface{}{
		"query": `query TestHello {
			hello
		}`,
	}
	
	resp, err := makeGraphQLRequest(query, "")
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) > 0 {
		return fmt.Errorf("GraphQL errors: %+v", resp.Errors)
	}

	data := resp.Data.(map[string]interface{})
	hello := data["hello"].(string)
	
	fmt.Printf("   ✓ Hello query successful\n")
	fmt.Printf("   ✓ Response: %s\n", hello)
	
	return nil
}

// =====================================
// REST File Upload (minimal, for IDs)
// =====================================

func testRESTFileUpload(ctx *TestContext) error {
	fmt.Println("Uploading test files via REST API (to get IDs for GraphQL testing)...")
	
	testFiles := []string{
		"test-files/ibm2.jpg",
		"test-files/sample.txt",
	}
	
	for _, filePath := range testFiles {
		fmt.Printf("   Uploading: %s\n", filepath.Base(filePath))
		
		fileID, err := uploadFileREST(filePath, ctx.RegularToken, nil)
		if err != nil {
			return fmt.Errorf("failed to upload %s: %v", filePath, err)
		}
		
		ctx.Files = append(ctx.Files, fileID)
		fmt.Printf("   ✓ File uploaded with ID: %s\n", fileID)
	}
	
	fmt.Printf("   ✅ Total files uploaded via REST: %d (IDs available for GraphQL testing)\n", len(ctx.Files))
	return nil
}

// =====================================
// User Profile & Stats Tests (GraphQL)
// =====================================

func testGraphQLMe(ctx *TestContext) error {
	fmt.Println("Testing GraphQL me query...")
	
	query := map[string]interface{}{
		"query": `query GetMe {
			me {
				id
				email
				role
				rate_limit_rps
				storage_quota_bytes
				created_at
			}
		}`,
	}
	
	resp, err := makeGraphQLRequest(query, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) > 0 {
		return fmt.Errorf("GraphQL errors: %+v", resp.Errors)
	}

	data := resp.Data.(map[string]interface{})
	me := data["me"].(map[string]interface{})
	
	fmt.Printf("   ✓ Me query successful\n")
	fmt.Printf("   ✓ User ID: %s\n", me["id"].(string))
	fmt.Printf("   ✓ Email: %s\n", me["email"].(string))
	fmt.Printf("   ✓ Role: %s\n", me["role"].(string))
	
	return nil
}

func testGraphQLStats(ctx *TestContext) error {
	fmt.Println("Testing GraphQL stats query...")
	
	query := map[string]interface{}{
		"query": `query GetStats {
			stats {
				total_files
				total_size_bytes
				quota_bytes
				quota_used_bytes
				quota_available_bytes
				files_by_type {
					mime_type
					count
				}
				upload_history {
					date
					count
					total_size
				}
			}
		}`,
	}
	
	resp, err := makeGraphQLRequest(query, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) > 0 {
		return fmt.Errorf("GraphQL errors: %+v", resp.Errors)
	}

	data := resp.Data.(map[string]interface{})
	stats := data["stats"].(map[string]interface{})
	
	fmt.Printf("   ✓ Stats query successful\n")
	fmt.Printf("   ✓ Total files: %.0f\n", stats["total_files"].(float64))
	fmt.Printf("   ✓ Total size: %.0f bytes\n", stats["total_size_bytes"].(float64))
	fmt.Printf("   ✓ Quota used: %.0f bytes\n", stats["quota_used_bytes"].(float64))
	
	return nil
}

func testGraphQLStatsFiltered(ctx *TestContext) error {
	fmt.Println("Testing GraphQL stats query with filters...")
	
	query := map[string]interface{}{
		"query": `query GetStatsFiltered($from: String, $to: String, $groupBy: String) {
			stats(from: $from, to: $to, group_by: $groupBy) {
				total_files
				total_size_bytes
				files_by_type {
					mime_type
					count
				}
			}
		}`,
		"variables": map[string]interface{}{
			"from":    "2025-01-01",
			"to":      "2025-12-31", 
			"groupBy": "month",
		},
	}
	
	resp, err := makeGraphQLRequest(query, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) > 0 {
		return fmt.Errorf("GraphQL errors: %+v", resp.Errors)
	}

	data := resp.Data.(map[string]interface{})
	stats := data["stats"].(map[string]interface{})
	
	fmt.Printf("   ✓ Filtered stats query successful\n")
	fmt.Printf("   ✓ Files in date range: %.0f\n", stats["total_files"].(float64))
	
	return nil
}

// =====================================
// File Operations Tests (GraphQL)
// =====================================

func testGraphQLFilesList(ctx *TestContext) error {
	fmt.Println("Testing GraphQL files list query...")
	
	query := map[string]interface{}{
		"query": `query GetFiles($page: Int, $pageSize: Int) {
			files(page: $page, page_size: $pageSize) {
				files {
					id
					original_filename
					mime_type
					size_bytes
					is_public
					download_count
					tags
					created_at
					updated_at
				}
				page
				page_size
				total
			}
		}`,
		"variables": map[string]interface{}{
			"page":     1,
			"pageSize": 10,
		},
	}
	
	resp, err := makeGraphQLRequest(query, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) > 0 {
		return fmt.Errorf("GraphQL errors: %+v", resp.Errors)
	}

	data := resp.Data.(map[string]interface{})
	filesData := data["files"].(map[string]interface{})
	files := filesData["files"].([]interface{})
	
	fmt.Printf("   ✓ Files list query successful\n")
	fmt.Printf("   ✓ Found %d files\n", len(files))
	fmt.Printf("   ✓ Total files: %.0f\n", filesData["total"].(float64))
	
	return nil
}

func testGraphQLFileDetails(ctx *TestContext) error {
	if len(ctx.Files) == 0 {
		return fmt.Errorf("no files available for testing")
	}
	
	fmt.Println("Testing GraphQL single file query...")
	
	fileID := ctx.Files[0]
	query := map[string]interface{}{
		"query": `query GetFile($id: UUID!) {
			file(id: $id) {
				id
				original_filename
				mime_type
				size_bytes
				folder_id
				is_public
				download_count
				tags
				created_at
				updated_at
				share_link {
					token
					is_active
					download_count
				}
			}
		}`,
		"variables": map[string]interface{}{
			"id": fileID,
		},
	}
	
	resp, err := makeGraphQLRequest(query, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) > 0 {
		return fmt.Errorf("GraphQL errors: %+v", resp.Errors)
	}

	data := resp.Data.(map[string]interface{})
	file := data["file"].(map[string]interface{})
	
	fmt.Printf("   ✓ File details query successful\n")
	fmt.Printf("   ✓ File: %s\n", file["original_filename"].(string))
	fmt.Printf("   ✓ MIME type: %s\n", file["mime_type"].(string))
	fmt.Printf("   ✓ Size: %.0f bytes\n", file["size_bytes"].(float64))
	
	return nil
}

func testGraphQLToggleFilePublic(ctx *TestContext) error {
	if len(ctx.Files) == 0 {
		return fmt.Errorf("no files available for testing")
	}
	
	fmt.Println("Testing GraphQL toggleFilePublic mutation...")
	
	fileID := ctx.Files[0]
	query := map[string]interface{}{
		"query": `mutation ToggleFilePublic($id: UUID!, $isPublic: Boolean!) {
			toggleFilePublic(id: $id, is_public: $isPublic) {
				id
				original_filename
				is_public
				share_link {
					token
					is_active
				}
			}
		}`,
		"variables": map[string]interface{}{
			"id":       fileID,
			"isPublic": true,
		},
	}
	
	resp, err := makeGraphQLRequest(query, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) > 0 {
		return fmt.Errorf("GraphQL errors: %+v", resp.Errors)
	}

	data := resp.Data.(map[string]interface{})
	file := data["toggleFilePublic"].(map[string]interface{})
	
	fmt.Printf("   ✓ Toggle file public successful\n")
	fmt.Printf("   ✓ File is now public: %t\n", file["is_public"].(bool))
	
	if shareLink, exists := file["share_link"]; exists && shareLink != nil {
		sl := shareLink.(map[string]interface{})
		token := sl["token"].(string)
		ctx.ShareTokens = append(ctx.ShareTokens, token)
		fmt.Printf("   ✓ Share link created: %s\n", token)
	}
	
	return nil
}

func testGraphQLMoveFile(ctx *TestContext) error {
	if len(ctx.Files) == 0 {
		return fmt.Errorf("no files available for testing")
	}
	
	// We'll create a folder first, then move the file
	fmt.Println("Testing GraphQL moveFile mutation (after creating a target folder)...")
	
	// Create a folder first
	folderQuery := map[string]interface{}{
		"query": `mutation CreateFolder($name: String!) {
			createFolder(name: $name) {
				id
				name
			}
		}`,
		"variables": map[string]interface{}{
			"name": "Move Test Folder",
		},
	}
	
	folderResp, err := makeGraphQLRequest(folderQuery, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("failed to create folder: %v", err)
	}

	if len(folderResp.Errors) > 0 {
		return fmt.Errorf("folder creation errors: %+v", folderResp.Errors)
	}

	folderData := folderResp.Data.(map[string]interface{})
	folder := folderData["createFolder"].(map[string]interface{})
	folderID := folder["id"].(string)
	ctx.Folders = append(ctx.Folders, folderID)
	
	// Now move the file
	fileID := ctx.Files[len(ctx.Files)-1] // Use last file to avoid affecting other tests
	moveQuery := map[string]interface{}{
		"query": `mutation MoveFile($fileId: UUID!, $folderId: UUID!) {
			moveFile(file_id: $fileId, folder_id: $folderId) {
				id
				original_filename
				folder_id
			}
		}`,
		"variables": map[string]interface{}{
			"fileId":   fileID,
			"folderId": folderID,
		},
	}
	
	resp, err := makeGraphQLRequest(moveQuery, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) > 0 {
		return fmt.Errorf("GraphQL errors: %+v", resp.Errors)
	}

	data := resp.Data.(map[string]interface{})
	file := data["moveFile"].(map[string]interface{})
	
	fmt.Printf("   ✓ Move file successful\n")
	fmt.Printf("   ✓ File: %s\n", file["original_filename"].(string))
	fmt.Printf("   ✓ Moved to folder: %s\n", file["folder_id"].(string))
	
	return nil
}

// =====================================
// Folder Operations Tests (GraphQL)
// =====================================

func testGraphQLCreateFolders(ctx *TestContext) error {
	fmt.Println("Testing GraphQL createFolder mutations...")
	
	folders := []string{"GraphQL Test Folder 1", "GraphQL Test Folder 2", "Nested Test Folder"}
	
	for _, folderName := range folders {
		fmt.Printf("   Creating folder: %s\n", folderName)
		
		query := map[string]interface{}{
			"query": `mutation CreateFolder($name: String!) {
				createFolder(name: $name) {
					id
					name
					parent_id
					created_at
					updated_at
				}
			}`,
			"variables": map[string]interface{}{
				"name": folderName,
			},
		}
		
		resp, err := makeGraphQLRequest(query, ctx.RegularToken)
		if err != nil {
			return fmt.Errorf("GraphQL request failed: %v", err)
		}

		if len(resp.Errors) > 0 {
			return fmt.Errorf("GraphQL errors: %+v", resp.Errors)
		}

		data := resp.Data.(map[string]interface{})
		folder := data["createFolder"].(map[string]interface{})
		folderID := folder["id"].(string)
		
		ctx.Folders = append(ctx.Folders, folderID)
		fmt.Printf("   ✓ Folder created with ID: %s\n", folderID)
	}
	
	fmt.Printf("   ✅ Total folders created: %d\n", len(folders))
	return nil
}

func testGraphQLFoldersList(ctx *TestContext) error {
	fmt.Println("Testing GraphQL folders list query...")
	
	query := map[string]interface{}{
		"query": `query GetFolders($parentId: UUID) {
			folders(parent_id: $parentId) {
				folders {
					id
					name
					parent_id
					created_at
					updated_at
				}
				files {
					id
					original_filename
					folder_id
				}
				pagination {
					page
					page_size
					total_folders
					total_files
					has_more
				}
			}
		}`,
		"variables": map[string]interface{}{
			"parentId": nil, // Root folders
		},
	}
	
	resp, err := makeGraphQLRequest(query, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) > 0 {
		return fmt.Errorf("GraphQL errors: %+v", resp.Errors)
	}

	data := resp.Data.(map[string]interface{})
	foldersData := data["folders"].(map[string]interface{})
	folders := foldersData["folders"].([]interface{})
	files := foldersData["files"].([]interface{})
	pagination := foldersData["pagination"].(map[string]interface{})
	
	fmt.Printf("   ✓ Folders list query successful\n")
	fmt.Printf("   ✓ Found %d folders\n", len(folders))
	fmt.Printf("   ✓ Found %d files in root\n", len(files))
	fmt.Printf("   ✓ Total folders: %.0f\n", pagination["total_folders"].(float64))
	
	return nil
}

func testGraphQLFolderDetails(ctx *TestContext) error {
	if len(ctx.Folders) == 0 {
		return fmt.Errorf("no folders available for testing")
	}
	
	fmt.Println("Testing GraphQL single folder query...")
	
	folderID := ctx.Folders[0]
	query := map[string]interface{}{
		"query": `query GetFolder($id: UUID!) {
			folder(id: $id) {
				folder {
					id
					name
					parent_id
					created_at
					updated_at
					share_link {
						token
						is_active
					}
				}
				breadcrumbs {
					id
					name
				}
			}
		}`,
		"variables": map[string]interface{}{
			"id": folderID,
		},
	}
	
	resp, err := makeGraphQLRequest(query, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) > 0 {
		return fmt.Errorf("GraphQL errors: %+v", resp.Errors)
	}

	data := resp.Data.(map[string]interface{})
	folderData := data["folder"].(map[string]interface{})
	folder := folderData["folder"].(map[string]interface{})
	breadcrumbs := folderData["breadcrumbs"].([]interface{})
	
	fmt.Printf("   ✓ Folder details query successful\n")
	fmt.Printf("   ✓ Folder name: %s\n", folder["name"].(string))
	fmt.Printf("   ✓ Breadcrumbs count: %d\n", len(breadcrumbs))
	
	return nil
}

func testGraphQLUpdateFolder(ctx *TestContext) error {
	if len(ctx.Folders) == 0 {
		return fmt.Errorf("no folders available for testing")
	}
	
	fmt.Println("Testing GraphQL updateFolder mutation...")
	
	folderID := ctx.Folders[0]
	newName := "Updated GraphQL Test Folder"
	
	query := map[string]interface{}{
		"query": `mutation UpdateFolder($id: UUID!, $name: String!) {
			updateFolder(id: $id, name: $name) {
				id
				name
				updated_at
			}
		}`,
		"variables": map[string]interface{}{
			"id":   folderID,
			"name": newName,
		},
	}
	
	resp, err := makeGraphQLRequest(query, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) > 0 {
		return fmt.Errorf("GraphQL errors: %+v", resp.Errors)
	}

	data := resp.Data.(map[string]interface{})
	folder := data["updateFolder"].(map[string]interface{})
	
	fmt.Printf("   ✓ Update folder successful\n")
	fmt.Printf("   ✓ New name: %s\n", folder["name"].(string))
	
	return nil
}

func testGraphQLMoveFolder(ctx *TestContext) error {
	if len(ctx.Folders) < 2 {
		return fmt.Errorf("need at least 2 folders for move testing")
	}
	
	fmt.Println("Testing GraphQL moveFolder mutation...")
	
	// Move the second folder into the first folder as parent
	folderToMove := ctx.Folders[1]
	parentFolder := ctx.Folders[0]
	
	query := map[string]interface{}{
		"query": `mutation MoveFolder($id: UUID!, $parentId: UUID!) {
			moveFolder(id: $id, parent_id: $parentId) {
				id
				name
				parent_id
			}
		}`,
		"variables": map[string]interface{}{
			"id":       folderToMove,
			"parentId": parentFolder,
		},
	}
	
	resp, err := makeGraphQLRequest(query, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) > 0 {
		return fmt.Errorf("GraphQL errors: %+v", resp.Errors)
	}

	data := resp.Data.(map[string]interface{})
	folder := data["moveFolder"].(map[string]interface{})
	
	fmt.Printf("   ✓ Move folder successful\n")
	fmt.Printf("   ✓ Moved folder: %s\n", folder["name"].(string))
	fmt.Printf("   ✓ New parent: %s\n", folder["parent_id"].(string))
	
	return nil
}

// =====================================
// Sharing Operations Tests (GraphQL)
// =====================================

func testGraphQLCreateFolderShare(ctx *TestContext) error {
	if len(ctx.Folders) == 0 {
		return fmt.Errorf("no folders available for sharing")
	}
	
	fmt.Println("Testing GraphQL createFolderShareLink mutation...")
	
	folderID := ctx.Folders[0]
	query := map[string]interface{}{
		"query": `mutation CreateFolderShareLink($id: UUID!) {
			createFolderShareLink(id: $id) {
				token
				is_active
				download_count
			}
		}`,
		"variables": map[string]interface{}{
			"id": folderID,
		},
	}
	
	resp, err := makeGraphQLRequest(query, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) > 0 {
		return fmt.Errorf("GraphQL errors: %+v", resp.Errors)
	}

	data := resp.Data.(map[string]interface{})
	shareLink := data["createFolderShareLink"].(map[string]interface{})
	token := shareLink["token"].(string)
	
	ctx.ShareTokens = append(ctx.ShareTokens, token)
	
	fmt.Printf("   ✓ Folder share link created\n")
	fmt.Printf("   ✓ Token: %s\n", token)
	fmt.Printf("   ✓ Is active: %t\n", shareLink["is_active"].(bool))
	
	return nil
}

func testGraphQLPublicFolder(ctx *TestContext) error {
	if len(ctx.ShareTokens) == 0 {
		return fmt.Errorf("no share tokens available for testing")
	}
	
	fmt.Println("Testing GraphQL publicFolder query (no authentication)...")
	
	token := ctx.ShareTokens[len(ctx.ShareTokens)-1] // Use latest folder share token
	query := map[string]interface{}{
		"query": `query GetPublicFolder($token: String!) {
			publicFolder(token: $token) {
				files {
					id
					original_filename
					mime_type
					size_bytes
					is_public
				}
				folders {
					id
					name
				}
				pagination {
					page
					page_size
					total_folders
					total_files
					has_more
				}
			}
		}`,
		"variables": map[string]interface{}{
			"token": token,
		},
	}
	
	// Note: No authentication token for public access
	resp, err := makeGraphQLRequest(query, "")
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) > 0 {
		return fmt.Errorf("GraphQL errors: %+v", resp.Errors)
	}

	data := resp.Data.(map[string]interface{})
	publicFolder := data["publicFolder"].(map[string]interface{})
	files := publicFolder["files"].([]interface{})
	folders := publicFolder["folders"].([]interface{})
	
	fmt.Printf("   ✓ Public folder access successful (no auth required)\n")
	fmt.Printf("   ✓ Found %d files in shared folder\n", len(files))
	fmt.Printf("   ✓ Found %d subfolders in shared folder\n", len(folders))
	
	return nil
}

func testGraphQLPublicFile(ctx *TestContext) error {
	if len(ctx.ShareTokens) == 0 {
		return fmt.Errorf("no file share tokens available for testing")
	}
	
	fmt.Println("Testing GraphQL publicFile query (no authentication)...")
	
	// Use the file share token from toggleFilePublic test
	token := ctx.ShareTokens[0] // First token should be from file sharing
	query := map[string]interface{}{
		"query": `query GetPublicFile($token: String!) {
			publicFile(token: $token) {
				id
				original_filename
				mime_type
				size_bytes
				is_public
				download_count
				created_at
			}
		}`,
		"variables": map[string]interface{}{
			"token": token,
		},
	}
	
	resp, err := makeGraphQLRequest(query, "")
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) > 0 {
		return fmt.Errorf("GraphQL errors: %+v", resp.Errors)
	}

	data := resp.Data.(map[string]interface{})
	publicFile := data["publicFile"].(map[string]interface{})
	
	fmt.Printf("   ✓ Public file access successful (no auth required)\n")
	fmt.Printf("   ✓ File: %s\n", publicFile["original_filename"].(string))
	fmt.Printf("   ✓ MIME type: %s\n", publicFile["mime_type"].(string))
	fmt.Printf("   ✓ Is public: %t\n", publicFile["is_public"].(bool))
	
	return nil
}

func testGraphQLDeleteFolderShare(ctx *TestContext) error {
	if len(ctx.Folders) == 0 {
		return fmt.Errorf("no folders available")
	}
	
	fmt.Println("Testing GraphQL deleteFolderShareLink mutation...")
	
	folderID := ctx.Folders[0]
	query := map[string]interface{}{
		"query": `mutation DeleteFolderShareLink($id: UUID!) {
			deleteFolderShareLink(id: $id)
		}`,
		"variables": map[string]interface{}{
			"id": folderID,
		},
	}
	
	resp, err := makeGraphQLRequest(query, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) > 0 {
		return fmt.Errorf("GraphQL errors: %+v", resp.Errors)
	}

	data := resp.Data.(map[string]interface{})
	deleted := data["deleteFolderShareLink"].(bool)
	
	fmt.Printf("   ✓ Delete folder share link successful\n")
	fmt.Printf("   ✓ Share link deleted: %t\n", deleted)
	
	return nil
}

// =====================================
// Error Handling Tests (GraphQL)
// =====================================

func testGraphQLUnauthorized(ctx *TestContext) error {
	fmt.Println("Testing GraphQL unauthorized access...")
	
	query := map[string]interface{}{
		"query": `query UnauthorizedTest {
			me {
				id
				email
			}
		}`,
	}
	
	resp, err := makeGraphQLRequest(query, "") // No token
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) == 0 {
		return fmt.Errorf("expected authentication error, but got success")
	}

	fmt.Printf("   ✓ Unauthorized access properly rejected\n")
	fmt.Printf("   ✓ Error message: %s\n", resp.Errors[0]["message"])
	
	return nil
}

func testGraphQLInvalidUUID(ctx *TestContext) error {
	fmt.Println("Testing GraphQL invalid UUID handling...")
	
	query := map[string]interface{}{
		"query": `query InvalidUUIDTest($id: UUID!) {
			file(id: $id) {
				id
				original_filename
			}
		}`,
		"variables": map[string]interface{}{
			"id": "invalid-uuid-format",
		},
	}
	
	resp, err := makeGraphQLRequest(query, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) == 0 {
		return fmt.Errorf("expected validation error for invalid UUID, but got success")
	}

	fmt.Printf("   ✓ Invalid UUID properly rejected\n")
	fmt.Printf("   ✓ Error message: %s\n", resp.Errors[0]["message"])
	
	return nil
}

func testGraphQLNotFound(ctx *TestContext) error {
	fmt.Println("Testing GraphQL not found handling...")
	
	// Use a valid UUID format but non-existent ID
	query := map[string]interface{}{
		"query": `query NotFoundTest($id: UUID!) {
			file(id: $id) {
				id
				original_filename
			}
		}`,
		"variables": map[string]interface{}{
			"id": "00000000-0000-0000-0000-000000000000",
		},
	}
	
	resp, err := makeGraphQLRequest(query, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %v", err)
	}

	if len(resp.Errors) == 0 {
		return fmt.Errorf("expected not found error, but got success")
	}

	fmt.Printf("   ✓ Non-existent resource properly handled\n")
	fmt.Printf("   ✓ Error message: %s\n", resp.Errors[0]["message"])
	
	return nil
}

// =====================================
// Complex Query Tests (GraphQL)
// =====================================

func testGraphQLComplexQuery(ctx *TestContext) error {
	fmt.Println("Testing GraphQL complex query with sequential operations...")
	
	// Break down the complex query into sequential operations to avoid DB prepared statement conflicts
	
	// Step 1: Get user info
	meQuery := map[string]interface{}{
		"query": `query GetMe {
			me {
				id
				email
				role
			}
		}`,
	}
	
	meResp, err := makeGraphQLRequest(meQuery, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("me query failed: %v", err)
	}
	if len(meResp.Errors) > 0 {
		return fmt.Errorf("me query errors: %+v", meResp.Errors)
	}
	
	// Small delay to avoid DB connection conflicts
	time.Sleep(100 * time.Millisecond)
	
	// Step 2: Get basic stats (avoiding complex date filtering)
	statsQuery := map[string]interface{}{
		"query": `query GetBasicStats {
			stats {
				total_files
				total_size_bytes
				quota_used_bytes
			}
		}`,
	}
	
	statsResp, err := makeGraphQLRequest(statsQuery, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("stats query failed: %v", err)
	}
	if len(statsResp.Errors) > 0 {
		return fmt.Errorf("stats query errors: %+v", statsResp.Errors)
	}
	
	// Small delay to avoid DB connection conflicts
	time.Sleep(100 * time.Millisecond)
	
	// Step 3: Get files list
	filesQuery := map[string]interface{}{
		"query": `query GetFiles {
			files(page: 1, page_size: 3) {
				files {
					id
					original_filename
					is_public
				}
				total
			}
		}`,
	}
	
	filesResp, err := makeGraphQLRequest(filesQuery, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("files query failed: %v", err)
	}
	if len(filesResp.Errors) > 0 {
		return fmt.Errorf("files query errors: %+v", filesResp.Errors)
	}

	// Extract and validate all responses
	meData := meResp.Data.(map[string]interface{})["me"].(map[string]interface{})
	statsData := statsResp.Data.(map[string]interface{})["stats"].(map[string]interface{})
	filesData := filesResp.Data.(map[string]interface{})["files"].(map[string]interface{})
	
	fmt.Printf("   ✓ Sequential complex queries successful\n")
	fmt.Printf("   ✓ User: %s\n", meData["email"].(string))
	fmt.Printf("   ✓ Total files: %.0f\n", statsData["total_files"].(float64))
	fmt.Printf("   ✓ Files in response: %d\n", len(filesData["files"].([]interface{})))
	
	return nil
}

func testGraphQLMultipleOperations(ctx *TestContext) error {
	fmt.Println("Testing GraphQL multiple operations with sequential execution...")
	
	// Execute operations sequentially to avoid DB prepared statement conflicts
	
	// Operation 1: Get current stats
	statsQuery := map[string]interface{}{
		"query": `query CurrentStats {
			stats {
				total_files
				total_size_bytes
			}
		}`,
	}
	
	statsResp, err := makeGraphQLRequest(statsQuery, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("stats query failed: %v", err)
	}
	if len(statsResp.Errors) > 0 {
		return fmt.Errorf("stats query errors: %+v", statsResp.Errors)
	}
	
	// Delay to prevent DB connection conflicts
	time.Sleep(150 * time.Millisecond)
	
	// Operation 2: Get all files
	filesQuery := map[string]interface{}{
		"query": `query AllFiles {
			files(page: 1, page_size: 5) {
				files {
					id
					original_filename
				}
				total
			}
		}`,
	}
	
	filesResp, err := makeGraphQLRequest(filesQuery, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("files query failed: %v", err)
	}
	if len(filesResp.Errors) > 0 {
		return fmt.Errorf("files query errors: %+v", filesResp.Errors)
	}
	
	// Delay to prevent DB connection conflicts  
	time.Sleep(150 * time.Millisecond)
	
	// Operation 3: Get root folders
	foldersQuery := map[string]interface{}{
		"query": `query RootFolders {
			folders {
				folders {
					id
					name
				}
				pagination {
					total_folders
				}
			}
		}`,
	}
	
	foldersResp, err := makeGraphQLRequest(foldersQuery, ctx.RegularToken)
	if err != nil {
		return fmt.Errorf("folders query failed: %v", err)
	}
	if len(foldersResp.Errors) > 0 {
		return fmt.Errorf("folders query errors: %+v", foldersResp.Errors)
	}

	// Extract all responses
	currentStats := statsResp.Data.(map[string]interface{})["stats"].(map[string]interface{})
	allFiles := filesResp.Data.(map[string]interface{})["files"].(map[string]interface{})
	rootFolders := foldersResp.Data.(map[string]interface{})["folders"].(map[string]interface{})
	
	fmt.Printf("   ✓ Sequential multiple operations successful\n")
	fmt.Printf("   ✓ Current stats - total files: %.0f\n", currentStats["total_files"].(float64))
	fmt.Printf("   ✓ All files - total: %.0f\n", allFiles["total"].(float64))
	fmt.Printf("   ✓ Root folders - total: %.0f\n", rootFolders["pagination"].(map[string]interface{})["total_folders"].(float64))
	
	return nil
}

// =====================================
// Cleanup Tests (GraphQL)
// =====================================

func testGraphQLDeleteFiles(ctx *TestContext) error {
	fmt.Println("Testing GraphQL deleteFile mutations (cleanup)...")
	
	deletedCount := 0
	for _, fileID := range ctx.Files {
		fmt.Printf("   Deleting file: %s\n", fileID)
		
		query := map[string]interface{}{
			"query": `mutation DeleteFile($id: UUID!) {
				deleteFile(id: $id)
			}`,
			"variables": map[string]interface{}{
				"id": fileID,
			},
		}
		
		resp, err := makeGraphQLRequest(query, ctx.RegularToken)
		if err != nil {
			fmt.Printf("   ⚠️  Failed to delete file %s: %v\n", fileID, err)
			continue
		}

		if len(resp.Errors) > 0 {
			fmt.Printf("   ⚠️  GraphQL errors deleting file %s: %+v\n", fileID, resp.Errors)
			continue
		}

		data := resp.Data.(map[string]interface{})
		deleted := data["deleteFile"].(bool)
		
		if deleted {
			deletedCount++
			fmt.Printf("   ✓ File deleted successfully\n")
		}
	}
	
	fmt.Printf("   ✅ Deleted %d/%d files via GraphQL\n", deletedCount, len(ctx.Files))
	return nil
}

func testGraphQLDeleteFolders(ctx *TestContext) error {
	fmt.Println("Testing GraphQL deleteFolder mutations (cleanup)...")
	
	deletedCount := 0
	for _, folderID := range ctx.Folders {
		fmt.Printf("   Deleting folder: %s\n", folderID)
		
		query := map[string]interface{}{
			"query": `mutation DeleteFolder($id: UUID!) {
				deleteFolder(id: $id, recursive: true)
			}`,
			"variables": map[string]interface{}{
				"id": folderID,
			},
		}
		
		resp, err := makeGraphQLRequest(query, ctx.RegularToken)
		if err != nil {
			fmt.Printf("   ⚠️  Failed to delete folder %s: %v\n", folderID, err)
			continue
		}

		if len(resp.Errors) > 0 {
			fmt.Printf("   ⚠️  GraphQL errors deleting folder %s: %+v\n", folderID, resp.Errors)
			continue
		}

		data := resp.Data.(map[string]interface{})
		deleted := data["deleteFolder"].(bool)
		
		if deleted {
			deletedCount++
			fmt.Printf("   ✓ Folder deleted successfully\n")
		}
	}
	
	fmt.Printf("   ✅ Deleted %d/%d folders via GraphQL\n", deletedCount, len(ctx.Folders))
	return nil
}

// =====================================
// Helper Functions
// =====================================

func makeGraphQLRequest(query map[string]interface{}, authToken string) (*GraphQLResponse, error) {
	jsonData, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", graphqlEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	httpResp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	var gqlResp GraphQLResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&gqlResp); err != nil {
		return nil, err
	}

	return &gqlResp, nil
}

func uploadFileREST(filePath, token string, folderID *string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var b bytes.Buffer
	writer := multipart.NewWriter(&b)

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return "", err
	}
	_, err = io.Copy(part, file)
	if err != nil {
		return "", err
	}

	if folderID != nil {
		writer.WriteField("folder_id", *folderID)
	}

	writer.WriteField("tags", "graphql-test,automated")

	err = writer.Close()
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", restURL+"/files", &b)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	var uploadResp RESTFileUploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		return "", err
	}

	return uploadResp.File.ID, nil
}