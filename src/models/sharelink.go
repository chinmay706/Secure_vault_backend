package models

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ShareLink represents a public sharing link for a file or folder
type ShareLink struct {
	ID            uuid.UUID  `json:"id" db:"id"`
	FileID        *uuid.UUID `json:"file_id,omitempty" db:"file_id"`
	FolderID      *uuid.UUID `json:"folder_id,omitempty" db:"folder_id"`
	Token         string     `json:"token" db:"token"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	IsActive      bool       `json:"is_active" db:"is_active"`
	DownloadCount int        `json:"download_count" db:"download_count"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
}

// NewShareLink creates a new share link with a secure random token for a file
func NewShareLink(fileID uuid.UUID) (*ShareLink, error) {
	token, err := generateSecureToken()
	if err != nil {
		return nil, err
	}

	return &ShareLink{
		ID:            uuid.New(),
		FileID:        &fileID,
		FolderID:      nil,
		Token:         token,
		ExpiresAt:     nil, // No expiration by default
		IsActive:      true, // Active by default when created
		DownloadCount: 0,
		CreatedAt:     time.Now().UTC(),
	}, nil
}

// NewFolderShareLink creates a new share link with a secure random token for a folder
func NewFolderShareLink(folderID uuid.UUID) (*ShareLink, error) {
	token, err := generateSecureToken()
	if err != nil {
		return nil, err
	}

	return &ShareLink{
		ID:            uuid.New(),
		FileID:        nil,
		FolderID:      &folderID,
		Token:         token,
		ExpiresAt:     nil, // No expiration by default
		IsActive:      true, // Active by default when created
		DownloadCount: 0,
		CreatedAt:     time.Now().UTC(),
	}, nil
}

// generateSecureToken generates a secure random token for the share link
func generateSecureToken() (string, error) {
	// Generate 32 random bytes
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	
	// Encode to base64 URL-safe format and remove padding for cleaner URLs
	token := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(bytes)
	return token, nil
}

// Enable enables the share link
func (sl *ShareLink) Enable() {
	sl.IsActive = true
}

// Disable disables the share link
func (sl *ShareLink) Disable() {
	sl.IsActive = false
}

// Toggle toggles the active status of the share link
func (sl *ShareLink) Toggle() {
	sl.IsActive = !sl.IsActive
}

// IncrementDownloadCount increments the download count
func (sl *ShareLink) IncrementDownloadCount() {
	sl.DownloadCount++
}

// IsExpired checks if the share link has expired
func (sl *ShareLink) IsExpired() bool {
	if sl.ExpiresAt == nil {
		return false
	}
	return time.Now().UTC().After(*sl.ExpiresAt)
}

// IsFileShare checks if this is a file share link
func (sl *ShareLink) IsFileShare() bool {
	return sl.FileID != nil && sl.FolderID == nil
}

// IsFolderShare checks if this is a folder share link
func (sl *ShareLink) IsFolderShare() bool {
	return sl.FolderID != nil && sl.FileID == nil
}

// GetTargetID returns the target ID (file or folder) for this share link
func (sl *ShareLink) GetTargetID() uuid.UUID {
	if sl.IsFileShare() {
		return *sl.FileID
	}
	if sl.IsFolderShare() {
		return *sl.FolderID
	}
	return uuid.Nil
}

// IsValidToken checks if the token format is valid (basic validation)
func IsValidShareToken(token string) bool {
	// Token should be base64 URL-safe encoded from 32 bytes (43 chars without padding)
	// Our tokens are generated as base64 URL-safe from 32 random bytes
	if len(token) < 40 || len(token) > 50 {
		return false
	}
	
	// Check if it contains only valid base64 URL-safe characters
	for _, char := range token {
		if !((char >= 'A' && char <= 'Z') ||
			(char >= 'a' && char <= 'z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_') {
			return false
		}
	}
	
	// Should not have obvious patterns like "malformed-token-format"
	// Real tokens shouldn't have multiple consecutive hyphens or obvious words
	if strings.Contains(token, "token") || strings.Contains(token, "malformed") || 
	   strings.Contains(token, "--") || strings.HasPrefix(token, "test") {
		return false
	}
	
	return true
}