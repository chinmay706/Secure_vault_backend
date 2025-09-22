-- Migration 004: Create sharelinks table
-- Creates the sharelinks table for managing public sharing URLs

CREATE TABLE IF NOT EXISTS sharelinks (
    id UUID PRIMARY KEY,
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    token VARCHAR(255) UNIQUE NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE,
    is_active BOOLEAN NOT NULL DEFAULT true,
    download_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_sharelinks_token ON sharelinks(token);
CREATE INDEX IF NOT EXISTS idx_sharelinks_file_id ON sharelinks(file_id);
CREATE INDEX IF NOT EXISTS idx_sharelinks_expires_at ON sharelinks(expires_at);