-- Migration 007: Extend sharelinks to support folders
-- Allows folder-level share links using Option A approach (extend existing table)

-- Add folder_id column to sharelinks table
ALTER TABLE sharelinks
    ADD COLUMN IF NOT EXISTS folder_id UUID NULL REFERENCES folders(id) ON DELETE CASCADE;

-- Add constraint to ensure either file_id OR folder_id is set (mutually exclusive)
-- Drop constraint if it exists first, then recreate
ALTER TABLE sharelinks
    DROP CONSTRAINT IF EXISTS chk_sharelink_target;

ALTER TABLE sharelinks
    ADD CONSTRAINT chk_sharelink_target CHECK (
        (file_id IS NOT NULL AND folder_id IS NULL) OR
        (file_id IS NULL AND folder_id IS NOT NULL)
    );

-- Create indexes for folder share links
CREATE INDEX IF NOT EXISTS idx_sharelinks_folder ON sharelinks(folder_id);
CREATE INDEX IF NOT EXISTS idx_sharelinks_folder_token ON sharelinks(folder_id, token);

-- Update existing index to include folder_id for combined queries
DROP INDEX IF EXISTS idx_sharelinks_file_token;
CREATE INDEX IF NOT EXISTS idx_sharelinks_file_token ON sharelinks(file_id, token) WHERE file_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sharelinks_folder_token_active ON sharelinks(folder_id, token) WHERE folder_id IS NOT NULL;

-- Add comments for documentation
COMMENT ON COLUMN sharelinks.folder_id IS 'Folder ID for folder share links (mutually exclusive with file_id)';