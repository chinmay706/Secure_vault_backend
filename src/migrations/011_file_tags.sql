-- ============================================
-- Migration 011: AI-powered file tags
-- Creates file_tags and ai_tag_jobs tables,
-- migrates existing tags, and adds sync trigger
-- ============================================

-- 1. Create file_tags table (source of truth for all tags)
CREATE TABLE IF NOT EXISTS file_tags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    is_ai_generated BOOLEAN DEFAULT false,
    confidence FLOAT DEFAULT 1.0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(file_id, name)
);

CREATE INDEX IF NOT EXISTS idx_file_tags_file_id ON file_tags(file_id);
CREATE INDEX IF NOT EXISTS idx_file_tags_name ON file_tags(name);
CREATE INDEX IF NOT EXISTS idx_file_tags_ai ON file_tags(is_ai_generated);

-- 2. Create ai_tag_jobs table (tracks AI generation status)
CREATE TABLE IF NOT EXISTS ai_tag_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    status VARCHAR(20) DEFAULT 'pending',  -- 'pending', 'processing', 'completed', 'failed'
    suggested_tags TEXT[] DEFAULT '{}',
    confidence_scores FLOAT[] DEFAULT '{}',
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_ai_tag_jobs_file_id ON ai_tag_jobs(file_id);
CREATE INDEX IF NOT EXISTS idx_ai_tag_jobs_status ON ai_tag_jobs(status);

-- 3. Migrate existing tags from TEXT[] column to file_tags table
INSERT INTO file_tags (file_id, name, is_ai_generated)
SELECT f.id, UNNEST(f.tags), false
FROM files f
WHERE f.tags IS NOT NULL AND ARRAY_LENGTH(f.tags, 1) > 0
ON CONFLICT DO NOTHING;

-- 4. Create trigger function to keep files.tags in sync with file_tags table
CREATE OR REPLACE FUNCTION sync_file_tags_array() RETURNS TRIGGER AS $$
DECLARE
    target_file_id UUID;
BEGIN
    IF TG_OP = 'DELETE' THEN
        target_file_id := OLD.file_id;
    ELSE
        target_file_id := NEW.file_id;
    END IF;

    UPDATE files
    SET tags = COALESCE(
        (SELECT ARRAY_AGG(ft.name ORDER BY ft.created_at)
         FROM file_tags ft
         WHERE ft.file_id = target_file_id),
        '{}'
    )
    WHERE id = target_file_id;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    ELSE
        RETURN NEW;
    END IF;
END;
$$ LANGUAGE plpgsql;

-- 5. Create triggers on file_tags table
DROP TRIGGER IF EXISTS trg_sync_file_tags_insert ON file_tags;
CREATE TRIGGER trg_sync_file_tags_insert
    AFTER INSERT ON file_tags
    FOR EACH ROW EXECUTE FUNCTION sync_file_tags_array();

DROP TRIGGER IF EXISTS trg_sync_file_tags_delete ON file_tags;
CREATE TRIGGER trg_sync_file_tags_delete
    AFTER DELETE ON file_tags
    FOR EACH ROW EXECUTE FUNCTION sync_file_tags_array();

DROP TRIGGER IF EXISTS trg_sync_file_tags_update ON file_tags;
CREATE TRIGGER trg_sync_file_tags_update
    AFTER UPDATE ON file_tags
    FOR EACH ROW EXECUTE FUNCTION sync_file_tags_array();
