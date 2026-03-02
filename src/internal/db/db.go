package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// DB wraps the database connection with additional functionality
type DB struct {
	*sql.DB
}
func init() {
	// Overload ensures .env values take priority over existing environment variables
	if err := godotenv.Overload(".env"); err != nil {
		log.Println("No .env file found in the current directory, falling back to system envs")
	} else {
		log.Println("Loaded .env file from the current directory")
	}
}

// NewDB creates a new database connection
func NewDB() (*DB, error) {
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		log.Println("DB_URL not set, constructing from individual environment variables")
		// Fallback to individual environment variables
		host := getEnvWithDefault("DB_HOST", "localhost")
		port := getEnvWithDefault("DB_PORT", "5432")
		user := getEnvWithDefault("DB_USER", "postgres")
		password := getEnvWithDefault("DB_PASSWORD", "postgres")
		dbname := getEnvWithDefault("DB_NAME", "securevault")
		sslmode := getEnvWithDefault("DB_SSLMODE", "disable")

		dbURL = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
			host, port, user, password, dbname, sslmode)
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	log.Println("Database connection established successfully")

	return &DB{DB: db}, nil
}

// NewTestDB creates a test database connection (uses same NeonDB for testing)
func NewTestDB() (*DB, error) {
	// For testing, use the same NeonDB connection
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DB_URL environment variable is required for testing")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open test database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping test database: %w", err)
	}

	// Configure connection pool for testing (smaller pool)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)

	log.Println("Test database connection established successfully")

	return &DB{DB: db}, nil
}

// RunMigrations executes database migrations from SQL files
func (db *DB) RunMigrations() error {
	// Find migrations directory relative to this file
	migrationsDir := filepath.Join("..", "..", "migrations")
	
	// Try alternative paths if the default doesn't exist
	possiblePaths := []string{
		migrationsDir,
		"src/migrations",
		"./migrations",
		"../migrations",
		"../../src/migrations",      // From tests/contract to src/migrations
		"../../../src/migrations",   // From deeper test directories
		"../../../../src/migrations", // From even deeper paths
	}
	
	var finalPath string
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			finalPath = path
			break
		}
	}
	
	if finalPath == "" {
		return fmt.Errorf("migrations directory not found. Tried: %v", possiblePaths)
	}

	// Read all .sql files from migrations directory
	files, err := os.ReadDir(finalPath)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory %s: %w", finalPath, err)
	}

	// Filter and sort migration files
	var migrationFiles []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".sql") {
			migrationFiles = append(migrationFiles, file.Name())
		}
	}
	
	if len(migrationFiles) == 0 {
		log.Printf("No migration files found in %s", finalPath)
		return nil
	}
	
	// Sort files to ensure they run in order (001_, 002_, etc.)
	sort.Strings(migrationFiles)

	log.Printf("Found %d migration files in %s", len(migrationFiles), finalPath)

	// Start transaction for all migrations
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute each migration file
	for _, filename := range migrationFiles {
		migrationPath := filepath.Join(finalPath, filename)
		
		log.Printf("Executing migration: %s", filename)
		
		// Read migration file
		content, err := os.ReadFile(migrationPath)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", migrationPath, err)
		}

		// Execute the migration SQL
		migrationSQL := string(content)
		if strings.TrimSpace(migrationSQL) == "" {
			log.Printf("Skipping empty migration file: %s", filename)
			continue
		}

		if _, err := tx.Exec(migrationSQL); err != nil {
			return fmt.Errorf("failed to execute migration %s: %w", filename, err)
		}
		
		log.Printf("Successfully executed migration: %s", filename)
	}

	// Commit all migrations
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migrations: %w", err)
	}

	log.Println("Database migrations completed successfully")
	return nil
}

// ClearAllData removes all data from tables (for testing)
func (db *DB) ClearAllData() error {
	// Order matters due to foreign key constraints
	tables := []string{"folder_file_publicity_tracking", "sharelinks", "files", "folders", "blobs", "users"}
	
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, table := range tables {
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			return fmt.Errorf("failed to clear table %s: %w", table, err)
		}
	}

	return tx.Commit()
}

func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}