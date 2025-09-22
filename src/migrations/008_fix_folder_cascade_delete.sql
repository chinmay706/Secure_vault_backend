-- Migration 008: Fix folder deletion to cascade to files
-- Changes the foreign key constraint to CASCADE delete instead of SET NULL
-- This ensures that when a folder is deleted, all files in it are also deleted

-- Drop the existing foreign key constraint
ALTER TABLE files DROP CONSTRAINT IF EXISTS files_folder_id_fkey;

-- Re-add the foreign key constraint with CASCADE delete
ALTER TABLE files 
    ADD CONSTRAINT files_folder_id_fkey 
    FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE CASCADE;

-- Update the comment to reflect the new behavior
COMMENT ON COLUMN files.folder_id IS 'Parent folder ID (NULL for root level files). Files are deleted when parent folder is deleted.';