package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"securevault-backend/src/api"
	"securevault-backend/src/api/middleware"
	graphqlServer "securevault-backend/src/graphql"
	"securevault-backend/src/internal/db"
	"securevault-backend/src/services"

	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger"
)

// App represents the application with its dependencies
type App struct {
	router         *mux.Router
	db             *db.DB
	handlers       *Handlers
	limitsMiddleware *middleware.LimitsMiddleware
	authService    *services.AuthService
}

// Handlers groups all HTTP handlers
type Handlers struct {
	Auth           *api.AuthHandlers
	Files          *api.FilesHandlers
	Folders        *api.FoldersHandlers
	PublicDownload *api.PublicDownloadHandlers
	Public         *api.PublicHandlers
	Stats          *api.StatsHandlers
	Admin          *api.AdminHandlers
	Summary        *api.SummaryHandlers
	Conversion     *api.ConversionHandlers
}

// NewApp creates and configures a new App instance
func NewApp() (*App, error) {
	// Initialize database
	database, err := db.NewDB()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Run migrations
	if err := database.RunMigrations(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	// Initialize services
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		if os.Getenv("ENVIRONMENT") == "production" {
			return nil, fmt.Errorf("JWT_SECRET must be set in production")
		}
		log.Println("Warning: JWT_SECRET not set, using default (development only)")
		jwtSecret = "default-secret-key-change-in-production"
	}

	authService := services.NewAuthService(database.DB, jwtSecret)
	statsService := services.NewStatsService(database.DB)
	
	// Initialize storage service with S3 integration
	storageService, err := services.NewStorageService(database.DB, "./storage", 100*1024*1024) // 100MB max file size
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage service: %w", err)
	}

	// Initialize file service with storage service for S3 cleanup
	fileService := services.NewFileService(database.DB, storageService)
	
	// Initialize folder service
	folderService := services.NewFolderService(database.DB)
	
	// Set up circular dependency between folder and file services
	folderService.SetFileService(fileService)

	// Initialize AI tag service
	aiProvider := os.Getenv("AI_PROVIDER") // "gemini" or "groq" (auto-detects if empty)
	geminiAPIKey := os.Getenv("GEMINI_API_KEY")
	groqAPIKey := os.Getenv("GROQ_API_KEY")
	aiDailyLimit := 100
	if envLimit := os.Getenv("AI_DAILY_LIMIT_PER_USER"); envLimit != "" {
		if parsed, err := fmt.Sscanf(envLimit, "%d", &aiDailyLimit); err != nil || parsed == 0 {
			aiDailyLimit = 100
		}
	}
	groqModel := os.Getenv("GROQ_MODEL") // defaults to llama-3.3-70b-versatile if empty
	aiTagService := services.NewAiTagService(database.DB, storageService, aiProvider, geminiAPIKey, groqAPIKey, groqModel, aiDailyLimit)
	if aiTagService.IsEnabled() {
		log.Printf("AI tag generation enabled (provider: %s, daily limit: %d)", aiTagService.Provider(), aiDailyLimit)
	} else {
		log.Println("AI tag generation disabled (no GEMINI_API_KEY or GROQ_API_KEY set)")
	}

	// Initialize AI summary service
	aiSummaryService := services.NewAiSummaryService(database.DB, storageService, groqAPIKey, groqModel)
	if aiSummaryService.IsEnabled() {
		log.Println("AI summary service enabled")
	}

	// Initialize conversion service
	conversionService := services.NewConversionService(database.DB, storageService, fileService, "./conversions")
	conversionService.StartCleanupLoop()
	log.Println("File conversion service enabled")

	// Initialize limits middleware
	quotaService := &middleware.DefaultQuotaService{} // Using default quota service
	limitsMiddleware := middleware.NewLimitsMiddleware(5.0, quotaService) // 5 requests per second

	// Initialize handlers
	handlers := &Handlers{
		Auth:           api.NewAuthHandlers(authService),
		Files:          api.NewFilesHandlers(fileService, storageService, authService, aiTagService),
		Folders:        api.NewFoldersHandlers(folderService, fileService, authService),
		PublicDownload: api.NewPublicDownloadHandlers(fileService, storageService),
		Public:         api.NewPublicHandlers(fileService, folderService, storageService),
		Stats:          api.NewStatsHandlers(statsService, authService),
		Admin:          api.NewAdminHandlers(statsService, fileService, authService),
		Summary:        api.NewSummaryHandlers(aiSummaryService, authService),
		Conversion:     api.NewConversionHandlers(conversionService, authService),
	}

	app := &App{
		router:           mux.NewRouter(),
		db:               database,
		handlers:         handlers,
		limitsMiddleware: limitsMiddleware,
		authService:      authService,
	}

	// Setup middleware
	app.setupMiddleware()

	// Setup routes with services
	app.setupRoutes(authService, fileService, folderService, statsService, storageService, aiTagService)

	return app, nil
}

// NewTestApp creates a test app instance with test database
func NewTestApp() (*App, error) {
	// Initialize test database
	database, err := db.NewTestDB()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize test database: %w", err)
	}

	// Run migrations
	if err := database.RunMigrations(); err != nil {
		return nil, fmt.Errorf("failed to run test migrations: %w", err)
	}

	// Clear any existing data
	if err := database.ClearAllData(); err != nil {
		return nil, fmt.Errorf("failed to clear test data: %w", err)
	}

	// Initialize services
	jwtSecret := "test-jwt-secret-key"
	authService := services.NewAuthService(database.DB, jwtSecret)
	statsService := services.NewStatsService(database.DB)
	
	// Initialize storage service with S3 integration
	storageService, err := services.NewStorageService(database.DB, "./test-storage", 100*1024*1024) // 100MB max file size
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage service: %w", err)
	}

	// Initialize file service with storage service for S3 cleanup
	fileService := services.NewFileService(database.DB, storageService)
	
	// Initialize folder service
	folderService := services.NewFolderService(database.DB)
	
	// Set up circular dependency between folder and file services
	folderService.SetFileService(fileService)

	// Initialize AI tag service (no API key in test)
	aiTagService := services.NewAiTagService(database.DB, storageService, "", "", "", "", 100)

	// Initialize AI summary service (no API key in test)
	aiSummaryService := services.NewAiSummaryService(database.DB, storageService, "", "")

	// Initialize conversion service for testing
	conversionService := services.NewConversionService(database.DB, storageService, fileService, "./test-conversions")

	// Initialize limits middleware for testing
	quotaService := &middleware.DefaultQuotaService{} // Using default quota service
	limitsMiddleware := middleware.NewLimitsMiddleware(5.0, quotaService) // 5 requests per second

	// Initialize handlers
	handlers := &Handlers{
		Auth:           api.NewAuthHandlers(authService),
		Files:          api.NewFilesHandlers(fileService, storageService, authService, aiTagService),
		Folders:        api.NewFoldersHandlers(folderService, fileService, authService),
		PublicDownload: api.NewPublicDownloadHandlers(fileService, storageService),
		Public:         api.NewPublicHandlers(fileService, folderService, storageService),
		Stats:          api.NewStatsHandlers(statsService, authService),
		Admin:          api.NewAdminHandlers(statsService, fileService, authService),
		Summary:        api.NewSummaryHandlers(aiSummaryService, authService),
		Conversion:     api.NewConversionHandlers(conversionService, authService),
	}

	app := &App{
		router:           mux.NewRouter(),
		db:               database,
		handlers:         handlers,
		limitsMiddleware: limitsMiddleware,
		authService:      authService,
	}

	// Setup middleware
	app.setupMiddleware()

	// Setup routes with services
	app.setupRoutes(authService, fileService, folderService, statsService, storageService, aiTagService)

	return app, nil
}

// Router returns the configured HTTP router
func (a *App) Router() http.Handler {
	return a.router
}

// Close closes the app and cleans up resources
func (a *App) Close() error {
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}

// setupMiddleware configures global middleware
func (a *App) setupMiddleware() {
	// Security headers middleware
	a.router.Use(securityHeadersMiddleware)

	// CORS middleware
	a.router.Use(corsMiddleware)

	// Add authentication context middleware (sets user_id context from JWT)
	a.router.Use(a.authContextMiddleware())

	// Add rate limiting middleware
	a.router.Use(a.limitsMiddleware.RateLimitMiddleware())
	
	// Add quota middleware for file uploads
	a.router.Use(a.limitsMiddleware.QuotaMiddleware())
}

// setupRoutes configures all API routes
func (a *App) setupRoutes(authService *services.AuthService, fileService *services.FileService, folderService *services.FolderService, statsService *services.StatsService, storageService *services.StorageService, aiTagService *services.AiTagService) {
	// Health check endpoint
	a.router.HandleFunc("/health", a.handleHealth).Methods("GET")
	
	// Swagger documentation endpoint
	a.router.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)
	
	// API v1 routes
	api := a.router.PathPrefix("/api/v1").Subrouter()
	
	// Health check under API v1
	api.HandleFunc("/health", a.handleHealth).Methods("GET")
	
	// Auth routes
	api.HandleFunc("/auth/signup", a.handlers.Auth.HandleSignup).Methods("POST", "OPTIONS")
	api.HandleFunc("/auth/login", a.handlers.Auth.HandleLogin).Methods("POST", "OPTIONS")
	api.HandleFunc("/auth/google", a.handlers.Auth.HandleGoogleLogin).Methods("POST", "OPTIONS")
	
	// User management routes (authenticated)
	api.HandleFunc("/users/{id}", a.handlers.Auth.HandleDeleteUser).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/users/{id}/password", a.handlers.Auth.HandleUpdatePassword).Methods("PATCH", "OPTIONS")
	
	// File routes (bulk routes must be registered before {id} routes for gorilla/mux)
	api.HandleFunc("/files/bulk-ai-tags", a.handlers.Files.HandleBulkAiTags).Methods("POST", "OPTIONS")
	api.HandleFunc("/files", a.handlers.Files.HandleFilesList).Methods("GET", "OPTIONS")
	api.HandleFunc("/files", a.handlers.Files.HandleFileUpload).Methods("POST", "OPTIONS")
	api.HandleFunc("/files/upload", a.handlers.Files.HandleFileUpload).Methods("POST", "OPTIONS")
	api.HandleFunc("/files/{id}", a.handlers.Files.HandleFileDetails).Methods("GET", "OPTIONS")
	api.HandleFunc("/files/{id}", a.handlers.Files.HandleFileDelete).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/files/{id}/download", a.handlers.Files.HandleFileDownload).Methods("GET", "OPTIONS")
	api.HandleFunc("/files/{id}/public", a.handlers.Files.HandleTogglePublic).Methods("PATCH", "OPTIONS")
	api.HandleFunc("/files/{id}/move", a.handlers.Files.HandleFileMove).Methods("PATCH", "OPTIONS")
	api.HandleFunc("/files/{id}/ai-tags", a.handlers.Files.HandleGetAiTags).Methods("GET", "OPTIONS")
	api.HandleFunc("/files/{id}/ai-tags", a.handlers.Files.HandleTriggerAiTags).Methods("POST", "OPTIONS")
	api.HandleFunc("/files/{id}/ai-describe", a.handlers.Files.HandleAiDescribe).Methods("POST", "OPTIONS")
	api.HandleFunc("/files/{id}/ai-summary", a.handlers.Summary.HandleGetAiSummary).Methods("GET", "OPTIONS")
	api.HandleFunc("/files/{id}/ai-summary", a.handlers.Summary.HandleGenerateAiSummary).Methods("POST", "OPTIONS")
	api.HandleFunc("/files/{id}/ai-summary/refine", a.handlers.Summary.HandleRefineAiSummary).Methods("POST", "OPTIONS")
	api.HandleFunc("/files/{id}/convert", a.handlers.Conversion.HandleStartConversion).Methods("POST", "OPTIONS")

	// Conversion routes
	api.HandleFunc("/conversions", a.handlers.Conversion.HandleConversionHistory).Methods("GET", "OPTIONS")
	api.HandleFunc("/conversions/{jobId}", a.handlers.Conversion.HandleGetConversionJob).Methods("GET", "OPTIONS")
	api.HandleFunc("/conversions/{jobId}", a.handlers.Conversion.HandleDeleteConversion).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/conversions/{jobId}/download", a.handlers.Conversion.HandleDownloadConversion).Methods("GET", "OPTIONS")
	
	// Folder routes
	api.HandleFunc("/folders", a.handlers.Folders.HandleCreateFolder).Methods("POST", "OPTIONS")
	api.HandleFunc("/folders", a.handlers.Folders.HandleListFolders).Methods("GET", "OPTIONS")
	api.HandleFunc("/folders/{id}", a.handlers.Folders.HandleGetFolder).Methods("GET", "OPTIONS")
	api.HandleFunc("/folders/{id}", a.handlers.Folders.HandleUpdateFolder).Methods("PATCH", "OPTIONS")
	api.HandleFunc("/folders/{id}", a.handlers.Folders.HandleDeleteFolder).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/folders/{id}/share", a.handlers.Folders.HandleCreateFolderShareLinkWithFilePublicity).Methods("POST", "OPTIONS")
	api.HandleFunc("/folders/{id}/share", a.handlers.Folders.HandleDeleteFolderShareLinkWithFilePublicity).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/folders/{id}/share/status", a.handlers.Folders.HandleCheckFolderShareLinkStatus).Methods("GET", "OPTIONS")
	
	// Public download
	api.HandleFunc("/p/f/{token}", a.handlers.Folders.HandlePublicFolderAccess).Methods("GET")
	api.HandleFunc("/p/{token}", a.handlers.PublicDownload.HandlePublicDownload).Methods("GET", "HEAD")
	
	// Stats routes
	api.HandleFunc("/stats/me", a.handlers.Stats.HandleStatsMe).Methods("GET", "OPTIONS")
	
	// Public routes (no authentication required)
	api.HandleFunc("/public/files/owner/{owner_id}", a.handlers.Public.HandlePublicFilesByOwner).Methods("GET", "OPTIONS")
	api.HandleFunc("/public/files/{id}", a.handlers.Public.HandlePublicFileByID).Methods("GET", "OPTIONS")
	api.HandleFunc("/public/files/share/{token}", a.handlers.Public.HandlePublicFileByShareToken).Methods("GET", "OPTIONS")
	api.HandleFunc("/public/folders/share/{token}", a.handlers.Public.HandlePublicFolderByShareToken).Methods("GET", "OPTIONS")
	
	// Public download routes (no authentication required)
	api.HandleFunc("/public/files/{id}/download", a.handlers.Public.HandlePublicFileDownload).Methods("GET", "OPTIONS")
	api.HandleFunc("/public/files/share/{token}/download", a.handlers.Public.HandlePublicFileDownloadByToken).Methods("GET", "OPTIONS")
	
	// Admin routes
	api.HandleFunc("/admin/signup", a.handlers.Admin.HandleAdminSignup).Methods("POST", "OPTIONS")
	api.HandleFunc("/admin/promote", a.handlers.Admin.HandlePromoteToAdmin).Methods("POST", "OPTIONS")
	api.HandleFunc("/admin/files", a.handlers.Admin.HandleAdminFiles).Methods("GET", "OPTIONS")
	api.HandleFunc("/admin/files/{id}", a.handlers.Admin.HandleAdminDeleteFile).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/admin/stats", a.handlers.Admin.HandleAdminStats).Methods("GET", "OPTIONS")
	api.HandleFunc("/admin/users/{id}/quota", a.handlers.Admin.HandleAdminUpdateUserQuota).Methods("PATCH", "OPTIONS")
	api.HandleFunc("/admin/users/{id}/suspend", a.handlers.Admin.HandleAdminSuspendUser).Methods("POST", "OPTIONS")
	
	// GraphQL routes
	a.setupGraphQLRoutes(api, authService, fileService, folderService, statsService, storageService, aiTagService)
}

// handleHealth provides a basic health check endpoint
// @Summary Health check
// @Description Check if the service is running and healthy
// @Tags Health
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string "Service is healthy"
// @Router /health [get]
func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","service":"securevault-backend"}`)
}

// handleNotImplemented provides a placeholder for unimplemented endpoints
func (a *App) handleNotImplemented(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	fmt.Fprintf(w, `{"error":{"code":"NOT_IMPLEMENTED","message":"This endpoint is not yet implemented"}}`)
}

// authContextMiddleware extracts user ID from JWT token and adds it to request context
func (a *App) authContextMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth context for public routes
			if strings.HasPrefix(r.URL.Path, "/health") || 
			   strings.HasPrefix(r.URL.Path, "/swagger/") ||
			   strings.HasPrefix(r.URL.Path, "/api/v1/auth/") ||
			   strings.HasPrefix(r.URL.Path, "/api/v1/public/") {
				next.ServeHTTP(w, r)
				return
			}

			// Extract JWT token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				// No token provided, continue without setting context
				// The individual handlers will handle auth requirements
				next.ServeHTTP(w, r)
				return
			}

			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			if tokenString == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Validate JWT token using the auth service
			user, err := a.authService.GetUserFromToken(tokenString)
			if err != nil {
				// Invalid token, continue without setting context
				// The individual handlers will handle auth requirements
				next.ServeHTTP(w, r)
				return
			}

			// Add user ID to request context
			ctx := context.WithValue(r.Context(), "user_id", user.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// securityHeadersMiddleware adds security headers to all responses
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware handles CORS headers and preflight requests
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use configured origin in production, wildcard for development
		allowedOrigin := os.Getenv("CORS_ALLOWED_ORIGIN")
		if allowedOrigin == "" {
			allowedOrigin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Type, Content-Disposition")
		w.Header().Set("Access-Control-Max-Age", "86400") // 24 hours

		// Handle preflight OPTIONS requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// setupGraphQLRoutes configures GraphQL endpoints
func (a *App) setupGraphQLRoutes(api *mux.Router, authService *services.AuthService, fileService *services.FileService, folderService *services.FolderService, statsService *services.StatsService, storageService *services.StorageService, aiTagService *services.AiTagService) {
	// GraphQL endpoint
	graphqlHandler := graphqlServer.NewGraphQLHandler(authService, fileService, folderService, statsService, storageService, aiTagService)
	api.Handle("/graphql", graphqlHandler).Methods("POST", "OPTIONS")
	
	// GraphQL Playground - only available in non-production environments
	if os.Getenv("ENVIRONMENT") != "production" {
		playgroundHandler := graphqlServer.NewPlaygroundHandler("/api/v1/graphql")
		api.Handle("/graphql/playground", playgroundHandler).Methods("GET")
	}
}