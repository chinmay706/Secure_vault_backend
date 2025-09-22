@echo off
REM Script to regenerate Swagger documentation for SecureVault API
REM Run this from the backend directory

echo Regenerating Swagger documentation...
go run github.com/swaggo/swag/cmd/swag init -g src/main.go --output src/swaggerdocs

if %errorlevel% equ 0 (
    echo.
    echo ✓ Swagger documentation generated successfully!
    echo.
    echo Available at:
    echo   - Swagger UI: http://localhost:8080/swagger/index.html
    echo   - JSON API:   http://localhost:8080/swagger/doc.json
    echo.
    echo API Endpoints documented:
    echo   Authentication:
    echo     POST /auth/signup    - Register new user
    echo     POST /auth/login     - Authenticate user
    echo   Files:
    echo     GET    /files        - List user files
    echo     POST   /files        - Upload file
    echo     GET    /files/{id}   - Get file details
    echo     GET    /files/{id}/download - Download file
    echo     DELETE /files/{id}   - Delete file
    echo     PATCH  /files/{id}/public - Toggle public access
    echo   Statistics:
    echo     GET /stats/me        - User statistics
    echo   Admin:
    echo     GET    /admin/files  - List all files (admin)
    echo     DELETE /admin/files/{id} - Delete any file (admin)
    echo     GET    /admin/stats  - System statistics (admin)
    echo   Public:
    echo     GET/HEAD /p/{token}  - Public file download
    echo   Health:
    echo     GET /health          - Service health check
    echo.
    echo Files updated:
    echo   - src/swaggerdocs/docs.go
    echo   - src/swaggerdocs/swagger.json
    echo   - src/swaggerdocs/swagger.yaml
) else (
    echo ✗ Failed to generate Swagger documentation
    echo Check for syntax errors in your Go code and Swagger annotations
)

echo.
pause