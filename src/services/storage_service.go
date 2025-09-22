package services

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"securevault-backend/src/models"

	// "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

var (
	ErrInvalidMimeType     = errors.New("invalid or unsafe MIME type")
	ErrFileSizeExceeded    = errors.New("file size exceeds maximum allowed")
	ErrStorageFailure      = errors.New("storage operation failed")
	ErrFileNotFoundStorage = errors.New("file not found in storage")
)

// StorageService handles file storage operations with S3 integration
type StorageService struct {
	db          *sql.DB
	s3Client    *s3.Client
	bucketName  string
	storagePath string // Fallback local storage path
	maxFileSize int64  // Maximum file size in bytes
	useS3       bool   // Whether to use S3 or local storage
}

// NewStorageService creates a new StorageService with S3 integration
func NewStorageService(db *sql.DB, storagePath string, maxFileSize int64) (*StorageService, error) {
	service := &StorageService{
		db:          db,
		storagePath: storagePath,
		maxFileSize: maxFileSize,
		useS3:       false,
	}

	// Try to initialize S3 client if AWS credentials are available
	bucketName := os.Getenv("S3_BUCKET_NAME")
	if bucketName != "" {
		cfg, err := config.LoadDefaultConfig(context.TODO(),
			config.WithRegion(os.Getenv("AWS_REGION")),
		)
		if err == nil {
			service.s3Client = s3.NewFromConfig(cfg)
			service.bucketName = bucketName
			service.useS3 = true
		}
	}

	// Ensure local storage directory exists as fallback
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	return service, nil
}

// UploadRequest represents file upload request
type UploadRequest struct {
	Reader       io.Reader
	Filename     string
	ContentType  string
	Size         int64
	OwnerID      uuid.UUID
}

// UploadResult represents file upload result
type UploadResult struct {
	File         *models.File
	Blob         *models.Blob
	IsDeuplicate bool
	Hash         string
}

// StreamingUpload handles file upload with streaming SHA-256 hashing and MIME validation
func (s *StorageService) StreamingUpload(req *UploadRequest) (*UploadResult, error) {
	if req.Reader == nil {
		return nil, errors.New("reader cannot be nil")
	}

	if req.OwnerID == uuid.Nil {
		return nil, errors.New("owner ID cannot be nil")
	}

	if req.Filename == "" {
		return nil, errors.New("filename cannot be empty")
	}

	// Check file size limit
	if req.Size > s.maxFileSize {
		return nil, fmt.Errorf("%w: file size %d exceeds maximum %d", ErrFileSizeExceeded, req.Size, s.maxFileSize)
	}

	// Create a temporary file for processing
	tempFile, err := s.createTempFile()
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Hash while streaming to temp file
	hasher := sha256.New()
	multiWriter := io.MultiWriter(tempFile, hasher)

	// Copy data while hashing
	bytesWritten, err := io.Copy(multiWriter, req.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to stream file: %w", err)
	}

	// Verify size matches if provided
	if req.Size > 0 && bytesWritten != req.Size {
		return nil, fmt.Errorf("size mismatch: expected %d, got %d", req.Size, bytesWritten)
	}

	// Calculate final hash
	hash := fmt.Sprintf("%x", hasher.Sum(nil))

	// Reset file position for MIME validation
	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("failed to reset file position: %w", err)
	}

	// Validate MIME type against file content
	detectedMimeType, err := s.detectMimeType(tempFile)
	if err != nil {
		return nil, fmt.Errorf("failed to detect MIME type: %w", err)
	}

	// Validate MIME type safety
	if !s.isSafeMimeType(detectedMimeType) {
		return nil, fmt.Errorf("%w: detected unsafe MIME type %s", ErrInvalidMimeType, detectedMimeType)
	}

	// Use detected MIME type if not provided or if mismatch
	finalMimeType := detectedMimeType
	if req.ContentType != "" && req.ContentType != detectedMimeType {
		// Log potential MIME spoofing attempt but use detected type
		finalMimeType = detectedMimeType
	}

	// Check if blob already exists (deduplication)
	existingBlob, err := s.getBlobByHash(hash)
	isDuplicate := false
	var blob *models.Blob

	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to check existing blob: %w", err)
	}

	if existingBlob != nil {
		// File is a duplicate - increment reference count
		isDuplicate = true
		blob = existingBlob
		err = s.incrementBlobRefCount(hash)
		if err != nil {
			return nil, fmt.Errorf("failed to increment blob reference count: %w", err)
		}
	} else {
		// New file - store blob and create record
		err = s.storeBlobToPath(tempFile, hash)
		if err != nil {
			return nil, fmt.Errorf("failed to store blob: %w", err)
		}

		// Create blob record with MIME type and storage path
		storagePath := hash // For S3, use hash as the key; for local, it's the relative path
		blob = models.NewBlob(hash, bytesWritten, finalMimeType, storagePath)
		err = s.createBlobRecord(blob)
		if err != nil {
			// Clean up stored file on database failure
			s.deleteBlobFromPath(hash)
			return nil, fmt.Errorf("failed to create blob record: %w", err)
		}
	}

	// Create file record
	file := models.NewFile(req.OwnerID, hash, req.Filename, finalMimeType, bytesWritten)
	err = s.createFileRecord(file)
	if err != nil {
		// If this is a new blob and file creation fails, clean up
		if !isDuplicate {
			s.deleteBlobFromPath(hash)
			s.deleteBlobRecord(hash)
		} else {
			// Decrement reference count for existing blob
			s.decrementBlobRefCount(hash)
		}
		return nil, fmt.Errorf("failed to create file record: %w", err)
	}

	return &UploadResult{
		File:         file,
		Blob:         blob,
		IsDeuplicate: isDuplicate,
		Hash:         hash,
	}, nil
}

// DownloadFile retrieves file content by hash
func (s *StorageService) DownloadFile(hash string) (io.ReadCloser, error) {
	if hash == "" {
		return nil, errors.New("hash cannot be empty")
	}

	// Verify blob exists in database
	_, err := s.getBlobByHash(hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrFileNotFoundStorage
		}
		return nil, fmt.Errorf("failed to verify blob: %w", err)
	}

	if s.useS3 {
		// Download from S3
		result, err := s.s3Client.GetObject(context.TODO(), &s3.GetObjectInput{
			Bucket: &s.bucketName,
			Key:    &hash,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to download from S3: %w", err)
		}
		return result.Body, nil
	}

	// Open file from local storage
	filePath := s.getBlobPath(hash)
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrFileNotFoundStorage
		}
		return nil, fmt.Errorf("failed to open stored file: %w", err)
	}

	return file, nil
}

// DeleteBlob removes a blob from storage if no longer referenced
func (s *StorageService) DeleteBlob(hash string) error {
	if hash == "" {
		return errors.New("hash cannot be empty")
	}

	// Check if blob has any remaining references
	blob, err := s.getBlobByHash(hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to check blob: %w", err)
	}

	if blob.RefCount > 0 {
		return errors.New("cannot delete blob with remaining references")
	}

	// Delete from storage
	err = s.deleteBlobFromPath(hash)
	if err != nil {
		return fmt.Errorf("failed to delete blob from storage: %w", err)
	}

	// Delete from database
	err = s.deleteBlobRecord(hash)
	if err != nil {
		return fmt.Errorf("failed to delete blob record: %w", err)
	}

	return nil
}

// createTempFile creates a temporary file for processing
func (s *StorageService) createTempFile() (*os.File, error) {
	return os.CreateTemp("", "upload_*")
}

// detectMimeType detects MIME type from file content
func (s *StorageService) detectMimeType(file *os.File) (string, error) {
	// Read first 512 bytes for MIME detection
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read file for MIME detection: %w", err)
	}

	// Detect MIME type from content
	mimeType := http.DetectContentType(buffer[:n])
	
	// Reset file position
	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return "", fmt.Errorf("failed to reset file position: %w", err)
	}

	return mimeType, nil
}

// isSafeMimeType validates that the MIME type is safe to store
func (s *StorageService) isSafeMimeType(mimeType string) bool {
	// List of dangerous MIME types to block
	dangerousMimes := []string{
		"application/x-executable",
		"application/x-msdownload",
		"application/x-msdos-program",
		"application/x-winexe",
		"application/x-shockwave-flash",
		"text/html", // Prevent HTML injection
		"text/javascript",
		"application/javascript",
	}

	for _, dangerous := range dangerousMimes {
		if strings.Contains(strings.ToLower(mimeType), dangerous) {
			return false
		}
	}

	// Allow common safe types
	safePrefixes := []string{
		"text/plain",
		"image/",
		"video/",
		"audio/",
		"application/pdf",
		"application/zip",
		"application/json",
		"application/xml",
		"application/msword",
		"application/vnd.openxmlformats-officedocument",
		"application/vnd.ms-excel",
		"application/octet-stream", // Generic binary (needs careful handling)
	}

	for _, safe := range safePrefixes {
		if strings.HasPrefix(strings.ToLower(mimeType), safe) {
			return true
		}
	}

	// Default to safe for unknown types (can be configured)
	return true
}

// storeBlobToPath stores blob content to the storage path or S3
func (s *StorageService) storeBlobToPath(tempFile *os.File, hash string) error {
	// Reset temp file position
	_, err := tempFile.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to reset temp file position: %w", err)
	}

	if s.useS3 {
		// Upload to S3
		_, err = s.s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket: &s.bucketName,
			Key:    &hash,
			Body:   tempFile,
		})
		if err != nil {
			return fmt.Errorf("failed to upload blob to S3: %w", err)
		}
		return nil
	}

	// Use local file storage as fallback
	// Ensure storage directory exists
	err = os.MkdirAll(s.storagePath, 0755)
	if err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Create final path using hash
	finalPath := s.getBlobPath(hash)

	// Create final file
	finalFile, err := os.Create(finalPath)
	if err != nil {
		return fmt.Errorf("failed to create final file: %w", err)
	}
	defer finalFile.Close()

	// Copy from temp to final location
	_, err = io.Copy(finalFile, tempFile)
	if err != nil {
		os.Remove(finalPath) // Clean up on failure
		return fmt.Errorf("failed to copy file to final location: %w", err)
	}

	return nil
}

// deleteBlobFromPath removes blob file from storage or S3
func (s *StorageService) deleteBlobFromPath(hash string) error {
	if s.useS3 {
		// Delete from S3
		_, err := s.s3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
			Bucket: &s.bucketName,
			Key:    &hash,
		})
		if err != nil {
			return fmt.Errorf("failed to delete blob from S3: %w", err)
		}
		return nil
	}

	// Use local file storage
	path := s.getBlobPath(hash)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete blob file: %w", err)
	}
	return nil
}

// getBlobPath generates storage path for a blob hash
func (s *StorageService) getBlobPath(hash string) string {
	// Use first 2 characters of hash for subdirectory (sharding)
	if len(hash) < 2 {
		return filepath.Join(s.storagePath, hash)
	}
	
	subdir := hash[:2]
	filename := hash[2:]
	
	return filepath.Join(s.storagePath, subdir, filename)
}

// Database helper methods

func (s *StorageService) getBlobByHash(hash string) (*models.Blob, error) {
	var blob models.Blob
	query := `SELECT hash, size_bytes, mime_type, storage_path, ref_count, created_at FROM blobs WHERE hash = $1`
	
	err := s.db.QueryRow(query, hash).Scan(
		&blob.Hash,
		&blob.SizeBytes,
		&blob.MimeType,
		&blob.StoragePath,
		&blob.RefCount,
		&blob.CreatedAt,
	)
	
	if err != nil {
		return nil, err
	}
	
	return &blob, nil
}

func (s *StorageService) createBlobRecord(blob *models.Blob) error {
	query := `
		INSERT INTO blobs (hash, size_bytes, mime_type, storage_path, ref_count, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	
	_, err := s.db.Exec(query, blob.Hash, blob.SizeBytes, blob.MimeType, blob.StoragePath, blob.RefCount, blob.CreatedAt)
	return err
}

func (s *StorageService) createFileRecord(file *models.File) error {
	query := `
		INSERT INTO files (id, owner_id, blob_hash, original_filename, mime_type, size_bytes, 
		                  is_public, download_count, tags, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	
	// Import pq for array handling
	_, err := s.db.Exec(query,
		file.ID,
		file.OwnerID,
		file.BlobHash,
		file.OriginalFilename,
		file.MimeType,
		file.SizeBytes,
		file.IsPublic,
		file.DownloadCount,
		"{}", // Empty array for tags initially
		file.CreatedAt,
		file.UpdatedAt,
	)
	
	return err
}

func (s *StorageService) incrementBlobRefCount(hash string) error {
	query := `UPDATE blobs SET ref_count = ref_count + 1 WHERE hash = $1`
	_, err := s.db.Exec(query, hash)
	return err
}

func (s *StorageService) decrementBlobRefCount(hash string) error {
	query := `UPDATE blobs SET ref_count = ref_count - 1 WHERE hash = $1 AND ref_count > 0`
	_, err := s.db.Exec(query, hash)
	return err
}

func (s *StorageService) deleteBlobRecord(hash string) error {
	query := `DELETE FROM blobs WHERE hash = $1`
	_, err := s.db.Exec(query, hash)
	return err
}

// GetStorageStats returns storage statistics
func (s *StorageService) GetStorageStats() (*StorageStats, error) {
	query := `
		SELECT 
			COUNT(*) as total_blobs,
			COALESCE(SUM(size_bytes), 0) as total_storage,
			COALESCE(SUM(ref_count), 0) as total_references
		FROM blobs
	`
	
	var stats StorageStats
	err := s.db.QueryRow(query).Scan(
		&stats.TotalBlobs,
		&stats.TotalStorage,
		&stats.TotalReferences,
	)
	
	if err != nil {
		return nil, fmt.Errorf("failed to get storage stats: %w", err)
	}
	
	return &stats, nil
}

// StorageStats represents storage statistics
type StorageStats struct {
	TotalBlobs      int   `json:"total_blobs"`
	TotalStorage    int64 `json:"total_storage"`
	TotalReferences int64 `json:"total_references"`
}

// ValidateFileExtension validates file extension against MIME type
func (s *StorageService) ValidateFileExtension(filename, mimeType string) bool {
	ext := filepath.Ext(filename)
	if ext == "" {
		return true // No extension to validate
	}
	
	// Get MIME type from extension
	expectedMime := mime.TypeByExtension(ext)
	if expectedMime == "" {
		return true // Unknown extension, allow
	}
	
	// Compare base MIME types (ignore parameters)
	expectedBase := strings.Split(expectedMime, ";")[0]
	actualBase := strings.Split(mimeType, ";")[0]
	
	return strings.EqualFold(expectedBase, actualBase)
}