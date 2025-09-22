-- Migration 005: Create folders table
-- Adds folder support for hierarchical file organization

-- Create folders table
CREATE TABLE IF NOT EXISTS folders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    parent_id UUID NULL REFERENCES folders(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    
    -- Constraints
    CONSTRAINT chk_no_self_reference CHECK (id IS DISTINCT FROM parent_id)
);

-- Create unique constraint for folder names per parent (case-insensitive)
-- This needs to be created separately to handle the COALESCE and LOWER functions
CREATE UNIQUE INDEX IF NOT EXISTS uq_folder_name_per_parent 
ON folders (owner_id, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'::UUID), LOWER(name));

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_folders_owner_parent ON folders(owner_id, parent_id);
CREATE INDEX IF NOT EXISTS idx_folders_owner ON folders(owner_id);
CREATE INDEX IF NOT EXISTS idx_folders_parent ON folders(parent_id);

-- Add comments for documentation
COMMENT ON TABLE folders IS 'Hierarchical folder structure for file organization';
COMMENT ON COLUMN folders.id IS 'Unique folder identifier';
COMMENT ON COLUMN folders.owner_id IS 'User who owns this folder';
COMMENT ON COLUMN folders.name IS 'Folder name (unique per parent)';
COMMENT ON COLUMN folders.parent_id IS 'Parent folder ID (NULL for root folders)';
COMMENT ON COLUMN folders.created_at IS 'Folder creation timestamp';
COMMENT ON COLUMN folders.updated_at IS 'Last modification timestamp';