package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// TestFilesDetailSimple tests basic file detail functionality
func TestFilesDetailSimple(t *testing.T) {
	app := TestApp(t)

	t.Run("Invalid file ID format should return 400", func(t *testing.T) {
		userToken := createTestUserAndGetToken(t, app, "filetest@example.com", "testpass123")

		req := httptest.NewRequest("GET", "/api/v1/files/invalid-id", nil)
		req.Header.Set("Authorization", "Bearer "+userToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d. Response: %s", rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &response)
		if err != nil {
			t.Errorf("Failed to parse JSON response: %v", err)
		}

		if errorData, exists := response["error"]; exists {
			if errorMap, ok := errorData.(map[string]interface{}); ok {
				if code, exists := errorMap["code"].(string); exists && code == "INVALID_FILE_ID" {
					// Test passes
				} else {
					t.Error("Expected error code INVALID_FILE_ID")
				}
			}
		}
	})

	t.Run("Non-existent file should return 404", func(t *testing.T) {
		userToken := createTestUserAndGetToken(t, app, "filetest2@example.com", "testpass123")

		req := httptest.NewRequest("GET", "/api/v1/files/"+uuid.New().String(), nil)
		req.Header.Set("Authorization", "Bearer "+userToken)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d. Response: %s", rr.Code, rr.Body.String())
		}
	})
}