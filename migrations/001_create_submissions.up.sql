-- Create submissions table
CREATE TABLE IF NOT EXISTS submissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_token VARCHAR(64) UNIQUE NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',

    -- Contact info
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100),
    email VARCHAR(255) NOT NULL,
    website VARCHAR(255),
    company_name VARCHAR(255),

    -- Context
    reason_for_booking TEXT,
    how_did_you_hear VARCHAR(100),

    -- Current reality
    current_revenue VARCHAR(50),
    team_size VARCHAR(50),
    primary_service TEXT,
    average_deal_size VARCHAR(50),
    marketing_budget VARCHAR(50),
    biggest_bottleneck TEXT,
    is_decision_maker VARCHAR(20),
    previous_agency_experience TEXT,

    -- Process fields
    acquisition_source TEXT,
    sales_process TEXT,
    fulfillment_workflow TEXT,
    current_tech_stack TEXT,

    -- Dream future
    desired_outcome TEXT,
    desired_speed VARCHAR(50),
    ready_to_scale VARCHAR(20),

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_submissions_admin_token ON submissions(admin_token);
CREATE INDEX IF NOT EXISTS idx_submissions_status ON submissions(status);
CREATE INDEX IF NOT EXISTS idx_submissions_created_at ON submissions(created_at DESC);

-- Status constraint
ALTER TABLE submissions ADD CONSTRAINT chk_submission_status
    CHECK (status IN ('pending', 'processing', 'completed', 'failed'));
