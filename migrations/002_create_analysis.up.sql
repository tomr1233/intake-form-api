-- Create analysis_results table
CREATE TABLE IF NOT EXISTS analysis_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    submission_id UUID NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,

    executive_summary TEXT,
    client_psychology TEXT,
    operational_gap_analysis TEXT,
    red_flags JSONB DEFAULT '[]',
    green_flags JSONB DEFAULT '[]',
    strategic_questions JSONB DEFAULT '[]',
    closing_strategy TEXT,
    estimated_fit_score INTEGER,

    raw_response JSONB,
    error_message TEXT,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    CONSTRAINT chk_fit_score CHECK (estimated_fit_score IS NULL OR (estimated_fit_score >= 0 AND estimated_fit_score <= 100))
);

-- Index
CREATE INDEX IF NOT EXISTS idx_analysis_submission_id ON analysis_results(submission_id);
