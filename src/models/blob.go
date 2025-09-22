package models

import (
	"time"
)

// Blob represents a deduplicated file blob in the system
// The hash serves as the primary key for deduplication
type Blob struct {
	Hash        string    `json:"hash" db:"hash"`                 // SHA-256 hash (primary key)
	SizeBytes   int64     `json:"size_bytes" db:"size_bytes"`
	MimeType    string    `json:"mime_type" db:"mime_type"`       // MIME type of the blob
	StoragePath string    `json:"storage_path" db:"storage_path"` // Storage path (S3 key or local path)
	RefCount    int       `json:"ref_count" db:"ref_count"`       // Number of files referencing this blob
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// NewBlob creates a new blob with the given hash, size, MIME type, and storage path
func NewBlob(hash string, sizeBytes int64, mimeType, storagePath string) *Blob {
	return &Blob{
		Hash:        hash,
		SizeBytes:   sizeBytes,
		MimeType:    mimeType,
		StoragePath: storagePath,
		RefCount:    1, // Start with 1 reference (the first file)
		CreatedAt:   time.Now().UTC(),
	}
}

// IncrementRefCount increments the reference count for this blob
func (b *Blob) IncrementRefCount() {
	b.RefCount++
}

// DecrementRefCount decrements the reference count for this blob
// Returns true if the blob should be deleted (refCount reaches 0)
func (b *Blob) DecrementRefCount() bool {
	if b.RefCount > 0 {
		b.RefCount--
	}
	return b.RefCount == 0
}

// IsOrphaned returns true if the blob has no references
func (b *Blob) IsOrphaned() bool {
	return b.RefCount == 0
}