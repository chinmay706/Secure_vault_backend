-- Migration 002: Create blobs table  
-- Creates the blobs table for deduplication storage

CREATE TABLE IF NOT EXISTS blobs (
    hash VARCHAR(64) PRIMARY KEY,
    size_bytes BIGINT NOT NULL,
    mime_type VARCHAR(255) NOT NULL,
    storage_path TEXT NOT NULL,
    ref_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_blobs_mime_type ON blobs(mime_type);