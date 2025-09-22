-- Migration 006: Add folder_id to files table
-- Allows files to be organized within folders

-- Add folder_id column to files table
ALTER TABLE files
    ADD COLUMN IF NOT EXISTS folder_id UUID NULL REFERENCES folders(id) ON DELETE SET NULL;

-- Create index for folder_id for performance
CREATE INDEX IF NOT EXISTS idx_files_folder ON files(folder_id);

-- Add composite index for owner and folder queries
CREATE INDEX IF NOT EXISTS idx_files_owner_folder ON files(owner_id, folder_id);

-- Add comments for documentation
COMMENT ON COLUMN files.folder_id IS 'Parent folder ID (NULL for root level files)';