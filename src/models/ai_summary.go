package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type AiSummary struct {
	ID              uuid.UUID       `json:"id"`
	FileID          uuid.UUID       `json:"file_id"`
	UserID          uuid.UUID       `json:"user_id"`
	Summary         string          `json:"summary"`
	Recommendations []string        `json:"recommendations"`
	Status          string          `json:"status"`
	ErrorMessage    string          `json:"error_message,omitempty"`
	History         json.RawMessage `json:"history"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type SummaryHistoryEntry struct {
	Summary   string `json:"summary"`
	Command   string `json:"command"`
	CreatedAt string `json:"created_at"`
}
