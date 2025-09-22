package models

import (
	"time"

	"github.com/google/uuid"
)

// FolderFilePublicityTracking represents the tracking of file publicity changes due to folder sharing
type FolderFilePublicityTracking struct {
	FolderID            uuid.UUID `json:"folder_id" db:"folder_id"`
	FileID              uuid.UUID `json:"file_id" db:"file_id"`
	WasOriginallyPublic bool      `json:"was_originally_public" db:"was_originally_public"`
	CreatedAt           time.Time `json:"created_at" db:"created_at"`
}

// NewFolderFilePublicityTracking creates a new tracking record
func NewFolderFilePublicityTracking(folderID, fileID uuid.UUID, wasOriginallyPublic bool) *FolderFilePublicityTracking {
	return &FolderFilePublicityTracking{
		FolderID:            folderID,
		FileID:              fileID,
		WasOriginallyPublic: wasOriginallyPublic,
		CreatedAt:           time.Now().UTC(),
	}
}