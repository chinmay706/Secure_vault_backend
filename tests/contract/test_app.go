package contract

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	testingpkg "securevault-backend/src/testing"

	"github.com/joho/godotenv"
)

// TestApp provides a test instance of the application for contract testing
func TestApp(t *testing.T) http.Handler {
	// Set test environment variables
	setTestEnv()

	// Create test app
	router, cleanup, err := testingpkg.NewTestApp()
	if err != nil {
		t.Fatalf("Failed to create test app: %v", err)
	}

	// Clean up when test finishes
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			log.Printf("Failed to close test app: %v", err)
		}
	})

	return router
}

// setTestEnv sets up test environment variables
func setTestEnv() {
	// Try multiple possible paths for the .env file
	possiblePaths := []string{
		".env",                          // Current directory
		filepath.Join("..", "..", ".env"), // From tests/contract to backend root
		"../../.env",                    // Alternative notation
		"../../../.env",                 // In case of nested structure
	}
	
	loaded := false
	for _, path := range possiblePaths {
		if err := godotenv.Load(path); err == nil {
			log.Printf("Test: Loaded .env file from: %s", path)
			loaded = true
			break
		}
	}
	
	if !loaded {
		log.Println("Test: No .env file found, falling back to system envs")
	}
	
	// Verify required environment variables
	if dbUrl := os.Getenv("DB_URL"); dbUrl == "" {
		log.Fatal("DB_URL environment variable is required for testing")
	}
	
	// JWT secret for testing
	if jwtSecret := os.Getenv("JWT_SECRET"); jwtSecret == "" {
		os.Setenv("JWT_SECRET", "test-jwt-secret-for-testing")
	}
}