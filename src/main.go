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
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"securevault-backend/src/internal/app"
	_ "securevault-backend/src/swaggerdocs"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	application, err := app.NewApp()
	if err != nil {
		log.Fatalf("Failed to initialize app: %v", err)
	}
	defer application.Close()

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           application.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("Server starting on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Forced shutdown: %v", err)
	}
	log.Println("Server stopped")
}