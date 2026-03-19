CREATE TABLE IF NOT EXISTS deployment_freezes (
    id TEXT PRIMARY KEY,
    scope TEXT NOT NULL DEFAULT 'all',
    reason TEXT NOT NULL DEFAULT '',
    created_by INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    thawed_at DATETIME,
    thawed_by INTEGER
);

CREATE INDEX IF NOT EXISTS idx_freezes_expires_at ON deployment_freezes(expires_at);
CREATE INDEX IF NOT EXISTS idx_freezes_thawed_at ON deployment_freezes(thawed_at);
