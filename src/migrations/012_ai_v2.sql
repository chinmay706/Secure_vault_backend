-- ============================================
-- Migration 012: AI V2 - descriptions, folder suggestions, bulk tagging
-- Adds ai_description + suggested_folder to ai_tag_jobs,
-- enforces UNIQUE(file_id), creates file_descriptions table
-- ============================================

-- 1. Add new columns to ai_tag_jobs
ALTER TABLE ai_tag_jobs ADD COLUMN IF NOT EXISTS ai_description TEXT DEFAULT '';
ALTER TABLE ai_tag_jobs ADD COLUMN IF NOT EXISTS suggested_folder VARCHAR(255) DEFAULT '';

-- 2. Deduplicate ai_tag_jobs (keep latest per file) before adding UNIQUE constraint
DELETE FROM ai_tag_jobs
WHERE id NOT IN (
    SELECT DISTINCT ON (file_id) id
    FROM ai_tag_jobs
    ORDER BY file_id, created_at DESC
);

-- 3. Add UNIQUE constraint on file_id (one job per file, re-run overwrites)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'uq_ai_tag_jobs_file_id'
    ) THEN
        ALTER TABLE ai_tag_jobs ADD CONSTRAINT uq_ai_tag_jobs_file_id UNIQUE (file_id);
    END IF;
END $$;

-- 4. Create file_descriptions table for standalone descriptions
CREATE TABLE IF NOT EXISTS file_descriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    description TEXT NOT NULL,
    generated_by VARCHAR(20) DEFAULT 'ai',  -- 'ai' or 'user'
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(file_id)
);

CREATE INDEX IF NOT EXISTS idx_file_descriptions_file_id ON file_descriptions(file_id);
