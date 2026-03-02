CREATE TABLE IF NOT EXISTS conversion_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    original_filename VARCHAR(500) NOT NULL DEFAULT '',
    source_format VARCHAR(20) NOT NULL,
    target_format VARCHAR(20) NOT NULL,
    status VARCHAR(20) DEFAULT 'pending',
    error_message TEXT DEFAULT '',
    result_path TEXT DEFAULT '',
    result_size_bytes BIGINT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_conversion_jobs_user ON conversion_jobs(user_id, created_at DESC);
