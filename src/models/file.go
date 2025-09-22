package models

import (
	"time"

	"github.com/google/uuid"
)

// File represents a file in the system with metadata and reference to blob
type File struct {
	ID               uuid.UUID  `json:"id" db:"id"`
	OwnerID          uuid.UUID  `json:"owner_id" db:"owner_id"`
	BlobHash         string     `json:"blob_hash" db:"blob_hash"`
	OriginalFilename string     `json:"original_filename" db:"original_filename"`
	MimeType         string     `json:"mime_type" db:"mime_type"`
	SizeBytes        int64      `json:"size_bytes" db:"size_bytes"`
	IsPublic         bool       `json:"is_public" db:"is_public"`
	DownloadCount    int        `json:"download_count" db:"download_count"`
	Tags             []string   `json:"tags" db:"tags"` // PostgreSQL array type
	FolderID         *uuid.UUID `json:"folder_id,omitempty" db:"folder_id"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}

// NewFile creates a new file with the given parameters
func NewFile(ownerID uuid.UUID, blobHash, originalFilename, mimeType string, sizeBytes int64) *File {
	return &File{
		ID:               uuid.New(),
		OwnerID:          ownerID,
		BlobHash:         blobHash,
		OriginalFilename: originalFilename,
		MimeType:         mimeType,
		SizeBytes:        sizeBytes,
		IsPublic:         false, // Default to private as per spec
		DownloadCount:    0,     // Default to 0 as per spec
		Tags:             []string{}, // Empty array by default
		FolderID:         nil,        // Default to root level
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
}

// IsOwnedBy checks if the file is owned by the given user ID
func (f *File) IsOwnedBy(userID uuid.UUID) bool {
	return f.OwnerID == userID
}

// TogglePublic toggles the public status of the file
func (f *File) TogglePublic() {
	f.IsPublic = !f.IsPublic
	f.UpdateTimestamp()
}

// SetPublic sets the public status of the file
func (f *File) SetPublic(isPublic bool) {
	f.IsPublic = isPublic
	f.UpdateTimestamp()
}

// IncrementDownloadCount increments the download count
func (f *File) IncrementDownloadCount() {
	f.DownloadCount++
	f.UpdateTimestamp()
}

// AddTag adds a tag to the file if it doesn't already exist
func (f *File) AddTag(tag string) {
	for _, existingTag := range f.Tags {
		if existingTag == tag {
			return // Tag already exists
		}
	}
	f.Tags = append(f.Tags, tag)
	f.UpdateTimestamp()
}

// RemoveTag removes a tag from the file
func (f *File) RemoveTag(tag string) {
	for i, existingTag := range f.Tags {
		if existingTag == tag {
			f.Tags = append(f.Tags[:i], f.Tags[i+1:]...)
			f.UpdateTimestamp()
			return
		}
	}
}

// UpdateTimestamp updates the updated_at timestamp
func (f *File) UpdateTimestamp() {
	f.UpdatedAt = time.Now().UTC()
}

// IsInFolder checks if the file is in a folder (not at root level)
func (f *File) IsInFolder() bool {
	return f.FolderID != nil
}

// IsInRoot checks if the file is at root level (no folder)
func (f *File) IsInRoot() bool {
	return f.FolderID == nil
}

// SetFolder moves the file to a different folder (or root if nil)
func (f *File) SetFolder(folderID *uuid.UUID) {
	f.FolderID = folderID
	f.UpdateTimestamp()
}