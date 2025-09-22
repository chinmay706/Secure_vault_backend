package handler

import (
    "net/http"
    "securevault-backend/src/internal/app"
)

// Handler is the main entry point for Vercel
func Handler(w http.ResponseWriter, r *http.Request) {
    // Initialize the app
    application, err := app.Initialize()
    if err != nil {
        http.Error(w, "Failed to initialize app", http.StatusInternalServerError)
        return
    }

    // Serve the request using your existing router
    application.Router.ServeHTTP(w, r)
}