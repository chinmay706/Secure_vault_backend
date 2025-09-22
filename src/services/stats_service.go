package services

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// StatsService handles statistics computation for deduplication and usage
type StatsService struct {
	db *sql.DB
}

// NewStatsService creates a new StatsService
func NewStatsService(db *sql.DB) *StatsService {
	return &StatsService{
		db: db,
	}
}

// GetDB returns the database connection for direct queries
func (s *StatsService) GetDB() *sql.DB {
	return s.db
}

// UserStats represents user-specific statistics
type UserStats struct {
	UserID              uuid.UUID `json:"user_id"`
	TotalFiles          int       `json:"total_files"`
	TotalSizeBytes      int64     `json:"total_size_bytes"`      // Actual storage used (deduped)
	OriginalSizeBytes   int64     `json:"original_size_bytes"`   // Size without deduplication
	SpaceSavedBytes     int64     `json:"space_saved_bytes"`     // Bytes saved through dedup
	SpaceSavedPercent   float64   `json:"space_saved_percent"`   // Percentage saved
	PublicFiles         int       `json:"public_files"`
	PrivateFiles        int       `json:"private_files"`
	TotalDownloads      int64     `json:"total_downloads"`
	StorageQuotaBytes   int64     `json:"storage_quota_bytes"`
	QuotaUsedPercent    float64   `json:"quota_used_percent"`
	DuplicateFiles      int       `json:"duplicate_files"`      // Files that reference existing blobs
}

// SystemStats represents system-wide statistics  
type SystemStats struct {
	TotalUsers          int     `json:"total_users"`
	TotalFiles          int     `json:"total_files"`
	TotalBlobs          int     `json:"total_blobs"`           // Unique content blobs
	TotalSizeBytes      int64   `json:"total_size_bytes"`      // Actual storage used
	OriginalSizeBytes   int64   `json:"original_size_bytes"`   // Size without dedup
	SpaceSavedBytes     int64   `json:"space_saved_bytes"`     // Total space saved
	SpaceSavedPercent   float64 `json:"space_saved_percent"`   // System-wide savings %
	TotalDownloads      int64   `json:"total_downloads"`
	PublicFiles         int     `json:"public_files"`
	PrivateFiles        int     `json:"private_files"`
	ActiveShareLinks    int     `json:"active_share_links"`
	DuplicateFiles      int     `json:"duplicate_files"`       // Total duplicate file instances
	DeduplicationRatio  float64 `json:"deduplication_ratio"`   // Files per unique blob
	AverageFileSize     int64   `json:"average_file_size"`
}

// DeduplicationStats represents detailed deduplication statistics
type DeduplicationStats struct {
	UniqueBlobs         int     `json:"unique_blobs"`
	TotalFileReferences int     `json:"total_file_references"`
	ActualStorageBytes  int64   `json:"actual_storage_bytes"`
	WouldBeStorageBytes int64   `json:"would_be_storage_bytes"`
	SpaceSavedBytes     int64   `json:"space_saved_bytes"`
	SavingsPercent      float64 `json:"savings_percent"`
	DeduplicationRatio  float64 `json:"deduplication_ratio"`
	TopDuplicatedBlobs  []BlobDeduplicationInfo `json:"top_duplicated_blobs"`
}

// BlobDeduplicationInfo represents deduplication info for a specific blob
type BlobDeduplicationInfo struct {
	Hash         string `json:"hash"`
	SizeBytes    int64  `json:"size_bytes"`
	RefCount     int    `json:"ref_count"`
	SpaceSaved   int64  `json:"space_saved"`   // (ref_count - 1) * size
	SampleFiles  []string `json:"sample_files"` // Sample filenames using this blob
}

// GetUserStats computes comprehensive statistics for a specific user
func (s *StatsService) GetUserStats(userID uuid.UUID) (*UserStats, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("user ID cannot be nil")
	}

	stats := &UserStats{UserID: userID}

	// Get user's quota information
	quotaQuery := `SELECT storage_quota_bytes FROM users WHERE id = $1`
	err := s.db.QueryRow(quotaQuery, userID).Scan(&stats.StorageQuotaBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to get user quota: %w", err)
	}

	// Get file counts and totals
	filesQuery := `
		SELECT 
			COUNT(*) as total_files,
			COALESCE(SUM(f.size_bytes), 0) as original_size,
			COUNT(CASE WHEN f.is_public = true THEN 1 END) as public_files,
			COUNT(CASE WHEN f.is_public = false THEN 1 END) as private_files,
			COALESCE(SUM(f.download_count), 0) as total_downloads
		FROM files f
		WHERE f.owner_id = $1
	`

	err = s.db.QueryRow(filesQuery, userID).Scan(
		&stats.TotalFiles,
		&stats.OriginalSizeBytes,
		&stats.PublicFiles,
		&stats.PrivateFiles,
		&stats.TotalDownloads,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get file stats: %w", err)
	}

	// Get actual storage used (deduplicated)
	storageQuery := `
		SELECT COALESCE(SUM(DISTINCT b.size_bytes), 0) as actual_storage
		FROM files f
		JOIN blobs b ON f.blob_hash = b.hash
		WHERE f.owner_id = $1
	`

	err = s.db.QueryRow(storageQuery, userID).Scan(&stats.TotalSizeBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage stats: %w", err)
	}

	// Get duplicate file count (files that reference blobs used by other files)
	duplicateQuery := `
		SELECT COUNT(*) as duplicate_files
		FROM files f
		WHERE f.owner_id = $1
		AND f.blob_hash IN (
			SELECT blob_hash 
			FROM files 
			GROUP BY blob_hash 
			HAVING COUNT(*) > 1
		)
	`

	err = s.db.QueryRow(duplicateQuery, userID).Scan(&stats.DuplicateFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to get duplicate stats: %w", err)
	}

	// Calculate derived statistics
	stats.SpaceSavedBytes = stats.OriginalSizeBytes - stats.TotalSizeBytes
	if stats.OriginalSizeBytes > 0 {
		stats.SpaceSavedPercent = float64(stats.SpaceSavedBytes) / float64(stats.OriginalSizeBytes) * 100
	}

	if stats.StorageQuotaBytes > 0 {
		stats.QuotaUsedPercent = float64(stats.TotalSizeBytes) / float64(stats.StorageQuotaBytes) * 100
	}

	return stats, nil
}

// GetSystemStats computes comprehensive system-wide statistics (admin only)
func (s *StatsService) GetSystemStats() (*SystemStats, error) {
	stats := &SystemStats{}

	// Get user count
	userQuery := `SELECT COUNT(*) FROM users`
	err := s.db.QueryRow(userQuery).Scan(&stats.TotalUsers)
	if err != nil {
		return nil, fmt.Errorf("failed to get user count: %w", err)
	}

	// Get file and blob counts with totals
	mainQuery := `
		SELECT 
			(SELECT COUNT(*) FROM files) as total_files,
			(SELECT COUNT(*) FROM blobs) as total_blobs,
			COALESCE(SUM(b.size_bytes), 0) as actual_storage,
			(SELECT COALESCE(SUM(size_bytes), 0) FROM files) as original_storage,
			(SELECT COALESCE(SUM(download_count), 0) FROM files) as total_downloads,
			(SELECT COUNT(*) FROM files WHERE is_public = true) as public_files,
			(SELECT COUNT(*) FROM files WHERE is_public = false) as private_files,
			(SELECT COUNT(*) FROM sharelinks WHERE is_active = true) as active_share_links
		FROM blobs b
	`

	var originalStorage int64
	err = s.db.QueryRow(mainQuery).Scan(
		&stats.TotalFiles,
		&stats.TotalBlobs,
		&stats.TotalSizeBytes,
		&originalStorage,
		&stats.TotalDownloads,
		&stats.PublicFiles,
		&stats.PrivateFiles,
		&stats.ActiveShareLinks,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get main stats: %w", err)
	}

	stats.OriginalSizeBytes = originalStorage

	// Get duplicate files count
	duplicateQuery := `
		SELECT COUNT(*) as duplicate_files
		FROM files f
		WHERE f.blob_hash IN (
			SELECT blob_hash 
			FROM files 
			GROUP BY blob_hash 
			HAVING COUNT(*) > 1
		)
	`

	err = s.db.QueryRow(duplicateQuery).Scan(&stats.DuplicateFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to get duplicate count: %w", err)
	}

	// Calculate derived statistics
	stats.SpaceSavedBytes = stats.OriginalSizeBytes - stats.TotalSizeBytes
	if stats.OriginalSizeBytes > 0 {
		stats.SpaceSavedPercent = float64(stats.SpaceSavedBytes) / float64(stats.OriginalSizeBytes) * 100
	}

	if stats.TotalBlobs > 0 {
		stats.DeduplicationRatio = float64(stats.TotalFiles) / float64(stats.TotalBlobs)
	}

	if stats.TotalFiles > 0 {
		stats.AverageFileSize = stats.OriginalSizeBytes / int64(stats.TotalFiles)
	}

	return stats, nil
}

// GetDeduplicationStats provides detailed deduplication analysis
func (s *StatsService) GetDeduplicationStats() (*DeduplicationStats, error) {
	stats := &DeduplicationStats{}

	// Get basic deduplication metrics
	basicQuery := `
		SELECT 
			COUNT(*) as unique_blobs,
			COALESCE(SUM(ref_count), 0) as total_references,
			COALESCE(SUM(size_bytes), 0) as actual_storage,
			COALESCE(SUM(size_bytes * ref_count), 0) as would_be_storage
		FROM blobs
		WHERE ref_count > 0
	`

	var totalReferences int64
	err := s.db.QueryRow(basicQuery).Scan(
		&stats.UniqueBlobs,
		&totalReferences,
		&stats.ActualStorageBytes,
		&stats.WouldBeStorageBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get basic dedup stats: %w", err)
	}

	stats.TotalFileReferences = int(totalReferences)

	// Calculate savings
	stats.SpaceSavedBytes = stats.WouldBeStorageBytes - stats.ActualStorageBytes
	if stats.WouldBeStorageBytes > 0 {
		stats.SavingsPercent = float64(stats.SpaceSavedBytes) / float64(stats.WouldBeStorageBytes) * 100
	}

	if stats.UniqueBlobs > 0 {
		stats.DeduplicationRatio = float64(stats.TotalFileReferences) / float64(stats.UniqueBlobs)
	}

	// Get top deduplicated blobs (most space-saving)
	topBlobsQuery := `
		SELECT 
			b.hash,
			b.size_bytes,
			b.ref_count,
			(b.ref_count - 1) * b.size_bytes as space_saved
		FROM blobs b
		WHERE b.ref_count > 1
		ORDER BY space_saved DESC
		LIMIT 10
	`

	rows, err := s.db.Query(topBlobsQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get top blobs: %w", err)
	}
	defer rows.Close()

	var topBlobs []BlobDeduplicationInfo
	for rows.Next() {
		var blob BlobDeduplicationInfo
		err := rows.Scan(
			&blob.Hash,
			&blob.SizeBytes,
			&blob.RefCount,
			&blob.SpaceSaved,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan blob info: %w", err)
		}

		// Get sample filenames for this blob
		sampleQuery := `
			SELECT original_filename 
			FROM files 
			WHERE blob_hash = $1 
			LIMIT 3
		`

		sampleRows, err := s.db.Query(sampleQuery, blob.Hash)
		if err == nil {
			var samples []string
			for sampleRows.Next() {
				var filename string
				if sampleRows.Scan(&filename) == nil {
					samples = append(samples, filename)
				}
			}
			sampleRows.Close()
			blob.SampleFiles = samples
		}

		topBlobs = append(topBlobs, blob)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate top blobs: %w", err)
	}

	stats.TopDuplicatedBlobs = topBlobs

	return stats, nil
}

// GetUserQuotaStatus returns quota usage information for a user
func (s *StatsService) GetUserQuotaStatus(userID uuid.UUID) (*QuotaStatus, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("user ID cannot be nil")
	}

	var status QuotaStatus

	// Get user quota and usage
	query := `
		SELECT 
			u.storage_quota_bytes,
			COALESCE(SUM(DISTINCT b.size_bytes), 0) as used_bytes
		FROM users u
		LEFT JOIN files f ON f.owner_id = u.id
		LEFT JOIN blobs b ON f.blob_hash = b.hash
		WHERE u.id = $1
		GROUP BY u.id, u.storage_quota_bytes
	`

	err := s.db.QueryRow(query, userID).Scan(
		&status.QuotaBytes,
		&status.UsedBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get quota status: %w", err)
	}

	status.RemainingBytes = status.QuotaBytes - status.UsedBytes
	if status.RemainingBytes < 0 {
		status.RemainingBytes = 0
	}

	if status.QuotaBytes > 0 {
		status.UsedPercent = float64(status.UsedBytes) / float64(status.QuotaBytes) * 100
	}

	status.IsNearLimit = status.UsedPercent >= 90
	status.IsOverLimit = status.UsedBytes > status.QuotaBytes

	return &status, nil
}

// QuotaStatus represents user quota status
type QuotaStatus struct {
	QuotaBytes     int64   `json:"quota_bytes"`
	UsedBytes      int64   `json:"used_bytes"`
	RemainingBytes int64   `json:"remaining_bytes"`
	UsedPercent    float64 `json:"used_percent"`
	IsNearLimit    bool    `json:"is_near_limit"`    // >= 90%
	IsOverLimit    bool    `json:"is_over_limit"`
}

// GetTopUploaders returns users with most uploads (admin function)
func (s *StatsService) GetTopUploaders(limit int) ([]UserUploadStats, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	query := `
		SELECT 
			u.id,
			u.email,
			COUNT(f.id) as file_count,
			COALESCE(SUM(f.size_bytes), 0) as total_size,
			COALESCE(SUM(DISTINCT b.size_bytes), 0) as actual_storage
		FROM users u
		LEFT JOIN files f ON f.owner_id = u.id
		LEFT JOIN blobs b ON f.blob_hash = b.hash
		GROUP BY u.id, u.email
		HAVING COUNT(f.id) > 0
		ORDER BY file_count DESC, actual_storage DESC
		LIMIT $1
	`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top uploaders: %w", err)
	}
	defer rows.Close()

	var uploaders []UserUploadStats
	for rows.Next() {
		var uploader UserUploadStats
		err := rows.Scan(
			&uploader.UserID,
			&uploader.Email,
			&uploader.FileCount,
			&uploader.TotalSizeBytes,
			&uploader.ActualStorageBytes,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan uploader stats: %w", err)
		}

		uploader.SpaceSavedBytes = uploader.TotalSizeBytes - uploader.ActualStorageBytes
		if uploader.TotalSizeBytes > 0 {
			uploader.SavingsPercent = float64(uploader.SpaceSavedBytes) / float64(uploader.TotalSizeBytes) * 100
		}

		uploaders = append(uploaders, uploader)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate uploaders: %w", err)
	}

	return uploaders, nil
}

// UserUploadStats represents upload statistics for a user
type UserUploadStats struct {
	UserID             uuid.UUID `json:"user_id"`
	Email              string    `json:"email"`
	FileCount          int       `json:"file_count"`
	TotalSizeBytes     int64     `json:"total_size_bytes"`
	ActualStorageBytes int64     `json:"actual_storage_bytes"`
	SpaceSavedBytes    int64     `json:"space_saved_bytes"`
	SavingsPercent     float64   `json:"savings_percent"`
}