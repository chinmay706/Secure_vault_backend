-- Migration 008: Create folder_file_publicity_tracking table
-- Tracks which files were made public due to folder sharing vs originally public files

CREATE TABLE IF NOT EXISTS folder_file_publicity_tracking (
    folder_id UUID NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    was_originally_public BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (folder_id, file_id)
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_folder_file_publicity_tracking_folder_id ON folder_file_publicity_tracking(folder_id);
CREATE INDEX IF NOT EXISTS idx_folder_file_publicity_tracking_file_id ON folder_file_publicity_tracking(file_id);
CREATE INDEX IF NOT EXISTS idx_folder_file_publicity_tracking_created_at ON folder_file_publicity_tracking(created_at);