package models

import (
	"time"

	"github.com/google/uuid"
)

// UserRole represents the role of a user in the system
type UserRole string

const (
	UserRoleUser  UserRole = "user"
	UserRoleAdmin UserRole = "admin"
)

// User represents a user in the system
type User struct {
	ID                  uuid.UUID `json:"id" db:"id"`
	Email               string    `json:"email" db:"email"`
	PasswordHash        string    `json:"-" db:"password_hash"` // Never serialize password hash
	Role                UserRole  `json:"role" db:"role"`
	RateLimitRps        int       `json:"rate_limit_rps" db:"rate_limit_rps"`
	StorageQuotaBytes   int64     `json:"storage_quota_bytes" db:"storage_quota_bytes"`
	CreatedAt           time.Time `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time `json:"updated_at" db:"updated_at"`
}

// NewUser creates a new user with default settings
func NewUser(email, passwordHash string, role UserRole) *User {
	return &User{
		ID:                uuid.New(),
		Email:             email,
		PasswordHash:      passwordHash,
		Role:              role,
		RateLimitRps:      2,                    // Default 2 RPS as per spec
		StorageQuotaBytes: 10 * 1024 * 1024,    // Default 10 MB as per spec
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
}

// IsAdmin returns true if the user has admin role
func (u *User) IsAdmin() bool {
	return u.Role == UserRoleAdmin
}

// HasStorageQuota checks if the user has enough storage quota for the given size
func (u *User) HasStorageQuota(usedBytes, newFileBytes int64) bool {
	return (usedBytes + newFileBytes) <= u.StorageQuotaBytes
}

// UpdateTimestamp updates the updated_at timestamp
func (u *User) UpdateTimestamp() {
	u.UpdatedAt = time.Now().UTC()
}