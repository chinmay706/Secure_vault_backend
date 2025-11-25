// @title SecureVault API
// @version 1.0
// @description SecureVault is a secure file storage service with S3 integration, user authentication, file sharing, and administrative features.
// @termsOfService https://securevault-backend-1.onrender.com/terms/

// @contact.name API Support
// @contact.url https://securevault-backend-1.onrender.com/support
// @contact.email support@securevault.com

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host securevault-backend.onrender-1.com
// @schemes https http
// @BasePath /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

package main

import (
	"log"
	"net/http"
	"os"
	"securevault-backend/src/internal/app"
	_ "securevault-backend/src/swaggerdocs" // Import swagger docs for registration
)

func main() {
	// Load configuration from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Initialize the application
	app, err := app.NewApp()
	if err != nil {
		log.Fatalf("Failed to initialize app: %v", err)
	}

	// Start the server
	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, app.Router()); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}