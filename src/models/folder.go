package models

import (
	"time"

	"github.com/google/uuid"
)

// Folder represents a hierarchical folder for file organization
type Folder struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	OwnerID   uuid.UUID  `json:"owner_id" db:"owner_id"`
	Name      string     `json:"name" db:"name"`
	ParentID  *uuid.UUID `json:"parent_id,omitempty" db:"parent_id"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
}

// NewFolder creates a new folder with the given parameters
func NewFolder(ownerID uuid.UUID, name string, parentID *uuid.UUID) *Folder {
	return &Folder{
		ID:        uuid.New(),
		OwnerID:   ownerID,
		Name:      name,
		ParentID:  parentID,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
}

// IsOwnedBy checks if the folder is owned by the given user ID
func (f *Folder) IsOwnedBy(userID uuid.UUID) bool {
	return f.OwnerID == userID
}

// IsRoot checks if the folder is a root folder (no parent)
func (f *Folder) IsRoot() bool {
	return f.ParentID == nil
}

// UpdateTimestamp updates the updated_at timestamp
func (f *Folder) UpdateTimestamp() {
	f.UpdatedAt = time.Now().UTC()
}

// SetName updates the folder name and timestamp
func (f *Folder) SetName(name string) {
	f.Name = name
	f.UpdateTimestamp()
}

// SetParent updates the folder's parent and timestamp
func (f *Folder) SetParent(parentID *uuid.UUID) {
	f.ParentID = parentID
	f.UpdateTimestamp()
}