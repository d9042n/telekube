CREATE TABLE IF NOT EXISTS deployment_freezes (
    id TEXT PRIMARY KEY,
    scope TEXT NOT NULL DEFAULT 'all',
    reason TEXT NOT NULL DEFAULT '',
    created_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    thawed_at TIMESTAMPTZ,
    thawed_by BIGINT
);

CREATE INDEX IF NOT EXISTS idx_freezes_expires_at ON deployment_freezes(expires_at);
CREATE INDEX IF NOT EXISTS idx_freezes_thawed_at ON deployment_freezes(thawed_at);
