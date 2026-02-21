package models

import (
	"time"

	"github.com/google/uuid"
)

type ConversionJob struct {
	ID               uuid.UUID `json:"id"`
	FileID           uuid.UUID `json:"file_id"`
	UserID           uuid.UUID `json:"user_id"`
	OriginalFilename string    `json:"original_filename"`
	SourceFormat     string    `json:"source_format"`
	TargetFormat     string    `json:"target_format"`
	Status           string    `json:"status"`
	ErrorMessage     string    `json:"error_message,omitempty"`
	ResultPath       string    `json:"-"`
	ResultSizeBytes  int64     `json:"result_size_bytes"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
