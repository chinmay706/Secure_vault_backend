-- ============================================
-- Migration 013: AI Summaries
-- Adds ai_summaries table for AI-powered document summarization
-- with per-user summaries, refinement history, and rate limiting
-- ============================================

CREATE TABLE IF NOT EXISTS ai_summaries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    summary TEXT DEFAULT '',
    recommendations TEXT[] DEFAULT '{}',
    status VARCHAR(20) DEFAULT 'pending',
    error_message TEXT DEFAULT '',
    history JSONB DEFAULT '[]'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(file_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_ai_summaries_file_id ON ai_summaries(file_id);
CREATE INDEX IF NOT EXISTS idx_ai_summaries_user_id ON ai_summaries(user_id);
CREATE INDEX IF NOT EXISTS idx_ai_summaries_status ON ai_summaries(status);
