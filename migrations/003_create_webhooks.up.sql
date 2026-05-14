CREATE TABLE IF NOT EXISTS webhooks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    url TEXT NOT NULL,
    secret VARCHAR(64) NOT NULL,
    secret_prefix VARCHAR(8) NOT NULL,
    events TEXT[] NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    CONSTRAINT chk_webhook_events_nonempty CHECK (array_length(events, 1) > 0),
    CONSTRAINT chk_webhook_url_scheme CHECK (url ~* '^https?://')
);

CREATE INDEX IF NOT EXISTS idx_webhooks_active ON webhooks(active);
