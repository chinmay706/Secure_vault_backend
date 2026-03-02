package services

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"securevault-backend/src/models"

	"github.com/google/uuid"
)

const (
	conversionDailyLimit  = 10
	conversionMaxFileSize = 50 * 1024 * 1024 // 50 MB
	conversionCleanupAge  = 24 * time.Hour
)

var (
	ErrConversionNotFound     = errors.New("conversion job not found")
	ErrConversionRateLimit    = errors.New("daily conversion limit reached (10/day)")
	ErrConversionUnsupported  = errors.New("unsupported conversion pair")
	ErrConversionFileTooLarge = errors.New("file exceeds 50 MB limit for conversion")
	ErrConversionNotCompleted = errors.New("conversion job has not completed yet")
)

type ConversionService struct {
	db             *sql.DB
	storageService *StorageService
	fileService    *FileService
	conversionDir  string
}

func NewConversionService(db *sql.DB, storageService *StorageService, fileService *FileService, conversionDir string) *ConversionService {
	if err := os.MkdirAll(conversionDir, 0755); err != nil {
		log.Printf("Warning: could not create conversion dir %s: %v", conversionDir, err)
	}
	return &ConversionService{
		db:             db,
		storageService: storageService,
		fileService:    fileService,
		conversionDir:  conversionDir,
	}
}

func (s *ConversionService) StartCleanupLoop() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			s.cleanupOldJobs()
		}
	}()
}

func (s *ConversionService) StartConversion(fileID, userID uuid.UUID, targetFormat string) (*models.ConversionJob, error) {
	targetFormat = strings.ToLower(strings.TrimSpace(targetFormat))

	file, err := s.fileService.GetFileByID(fileID)
	if err != nil {
		return nil, err
	}
	if file.OwnerID != userID {
		return nil, ErrUnauthorizedAccess
	}

	if file.SizeBytes > conversionMaxFileSize {
		return nil, ErrConversionFileTooLarge
	}

	sourceFormat := strings.TrimPrefix(strings.ToLower(filepath.Ext(file.OriginalFilename)), ".")
	if sourceFormat == "" {
		return nil, fmt.Errorf("cannot determine source format from filename %q", file.OriginalFilename)
	}

	if _, ok := GetConverter(sourceFormat, targetFormat); !ok {
		return nil, ErrConversionUnsupported
	}

	if err := s.checkRateLimit(userID); err != nil {
		return nil, err
	}

	job := &models.ConversionJob{
		ID:               uuid.New(),
		FileID:           fileID,
		UserID:           userID,
		OriginalFilename: file.OriginalFilename,
		SourceFormat:     sourceFormat,
		TargetFormat:     targetFormat,
		Status:           "processing",
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}

	query := `
		INSERT INTO conversion_jobs (id, file_id, user_id, original_filename, source_format, target_format, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err = s.db.Exec(query,
		job.ID, job.FileID, job.UserID, job.OriginalFilename,
		job.SourceFormat, job.TargetFormat, job.Status,
		job.CreatedAt, job.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert conversion job: %w", err)
	}

	if !TryRunBackground(func() { s.processConversion(job, file) }) {
		s.failJob(job.ID, "server too busy, try again later")
	}

	return job, nil
}

func (s *ConversionService) processConversion(job *models.ConversionJob, file *models.File) {
	defer func() {
		if r := recover(); r != nil {
			s.failJob(job.ID, fmt.Sprintf("panic: %v", r))
		}
	}()

	log.Printf("[CONVERSION] Starting job %s: %s -> %s (file: %s, blob: %s)",
		job.ID, job.SourceFormat, job.TargetFormat, file.OriginalFilename, file.BlobHash)

	inputPath := filepath.Join(s.conversionDir, fmt.Sprintf("%s_input.%s", job.ID, job.SourceFormat))
	outputPath := filepath.Join(s.conversionDir, fmt.Sprintf("%s.%s", job.ID, job.TargetFormat))

	defer os.Remove(inputPath)

	log.Printf("[CONVERSION] Job %s: downloading blob %s", job.ID, file.BlobHash)
	content, err := s.storageService.DownloadFile(file.BlobHash)
	if err != nil {
		s.failJob(job.ID, fmt.Sprintf("download failed: %v", err))
		return
	}

	inputFile, err := os.Create(inputPath)
	if err != nil {
		content.Close()
		s.failJob(job.ID, fmt.Sprintf("create temp file failed: %v", err))
		return
	}
	written, err := io.Copy(inputFile, content)
	if err != nil {
		inputFile.Close()
		content.Close()
		s.failJob(job.ID, fmt.Sprintf("write temp file failed: %v", err))
		return
	}
	inputFile.Close()
	content.Close()
	log.Printf("[CONVERSION] Job %s: downloaded %d bytes to %s", job.ID, written, inputPath)

	converter, _ := GetConverter(job.SourceFormat, job.TargetFormat)
	log.Printf("[CONVERSION] Job %s: running converter %T", job.ID, converter)
	if err := converter.Convert(inputPath, outputPath); err != nil {
		s.failJob(job.ID, fmt.Sprintf("conversion failed: %v", err))
		return
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		s.failJob(job.ID, fmt.Sprintf("stat result failed: %v", err))
		return
	}

	now := time.Now().UTC()
	update := `
		UPDATE conversion_jobs
		SET status = 'completed', result_path = $1, result_size_bytes = $2, updated_at = $3
		WHERE id = $4
	`
	if _, err := s.db.Exec(update, outputPath, info.Size(), now, job.ID); err != nil {
		log.Printf("[CONVERSION] Job %s: failed to update to completed: %v", job.ID, err)
		return
	}
	log.Printf("[CONVERSION] Job %s: completed, output %s (%d bytes)", job.ID, outputPath, info.Size())
}

func (s *ConversionService) failJob(jobID uuid.UUID, errMsg string) {
	log.Printf("Conversion job %s failed: %s", jobID, errMsg)
	now := time.Now().UTC()
	update := `
		UPDATE conversion_jobs
		SET status = 'failed', error_message = $1, updated_at = $2
		WHERE id = $3
	`
	if _, err := s.db.Exec(update, errMsg, now, jobID); err != nil {
		log.Printf("Failed to update conversion job %s to failed: %v", jobID, err)
	}
}

func (s *ConversionService) GetJob(jobID, userID uuid.UUID) (*models.ConversionJob, error) {
	query := `
		SELECT id, file_id, user_id, original_filename, source_format, target_format,
		       status, error_message, result_path, result_size_bytes, created_at, updated_at
		FROM conversion_jobs
		WHERE id = $1 AND user_id = $2
	`
	var job models.ConversionJob
	err := s.db.QueryRow(query, jobID, userID).Scan(
		&job.ID, &job.FileID, &job.UserID, &job.OriginalFilename,
		&job.SourceFormat, &job.TargetFormat, &job.Status,
		&job.ErrorMessage, &job.ResultPath, &job.ResultSizeBytes,
		&job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrConversionNotFound
		}
		return nil, fmt.Errorf("failed to get conversion job: %w", err)
	}
	return &job, nil
}

func (s *ConversionService) DeleteJob(jobID, userID uuid.UUID) error {
	job, err := s.GetJob(jobID, userID)
	if err != nil {
		return err
	}

	if job.ResultPath != "" {
		os.Remove(job.ResultPath)
	}

	_, err = s.db.Exec(`DELETE FROM conversion_jobs WHERE id = $1 AND user_id = $2`, jobID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete conversion job: %w", err)
	}
	return nil
}

func (s *ConversionService) GetJobHistory(userID uuid.UUID) ([]*models.ConversionJob, error) {
	query := `
		SELECT id, file_id, user_id, original_filename, source_format, target_format,
		       status, error_message, result_path, result_size_bytes, created_at, updated_at
		FROM conversion_jobs
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 20
	`
	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query conversion history: %w", err)
	}
	defer rows.Close()

	var jobs []*models.ConversionJob
	for rows.Next() {
		var job models.ConversionJob
		if err := rows.Scan(
			&job.ID, &job.FileID, &job.UserID, &job.OriginalFilename,
			&job.SourceFormat, &job.TargetFormat, &job.Status,
			&job.ErrorMessage, &job.ResultPath, &job.ResultSizeBytes,
			&job.CreatedAt, &job.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan conversion job: %w", err)
		}
		jobs = append(jobs, &job)
	}
	return jobs, rows.Err()
}

func (s *ConversionService) GetResultFile(jobID, userID uuid.UUID) (*models.ConversionJob, io.ReadCloser, error) {
	job, err := s.GetJob(jobID, userID)
	if err != nil {
		return nil, nil, err
	}
	if job.Status == "failed" {
		return nil, nil, fmt.Errorf("conversion failed: %s", job.ErrorMessage)
	}
	if job.Status != "completed" {
		return nil, nil, ErrConversionNotCompleted
	}

	log.Printf("[CONVERSION] Download requested for job %s, result_path=%q", jobID, job.ResultPath)
	f, err := os.Open(job.ResultPath)
	if err != nil {
		return nil, nil, fmt.Errorf("result file not available at %s: %w", job.ResultPath, err)
	}
	return job, f, nil
}

func (s *ConversionService) checkRateLimit(userID uuid.UUID) error {
	var count int
	query := `
		SELECT COUNT(*) FROM conversion_jobs
		WHERE user_id = $1 AND created_at > NOW() - INTERVAL '1 day'
	`
	if err := s.db.QueryRow(query, userID).Scan(&count); err != nil {
		return fmt.Errorf("failed to check conversion rate limit: %w", err)
	}
	if count >= conversionDailyLimit {
		return ErrConversionRateLimit
	}
	return nil
}

func (s *ConversionService) cleanupOldJobs() {
	cutoff := time.Now().UTC().Add(-conversionCleanupAge)

	rows, err := s.db.Query(
		`SELECT id, result_path FROM conversion_jobs WHERE created_at < $1`, cutoff,
	)
	if err != nil {
		log.Printf("Conversion cleanup query failed: %v", err)
		return
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		var resultPath string
		if err := rows.Scan(&id, &resultPath); err != nil {
			continue
		}
		if resultPath != "" {
			os.Remove(resultPath)
		}
		ids = append(ids, id)
	}

	if len(ids) > 0 {
		_, err := s.db.Exec(
			`DELETE FROM conversion_jobs WHERE created_at < $1`, cutoff,
		)
		if err != nil {
			log.Printf("Conversion cleanup delete failed: %v", err)
		} else {
			log.Printf("Cleaned up %d expired conversion jobs", len(ids))
		}
	}
}
