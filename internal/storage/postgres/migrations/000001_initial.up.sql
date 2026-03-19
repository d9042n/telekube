CREATE TABLE IF NOT EXISTS users (
    telegram_id BIGINT PRIMARY KEY,
    username TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT 'viewer',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS audit_log (
    id TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    username TEXT NOT NULL,
    action TEXT NOT NULL,
    resource TEXT NOT NULL DEFAULT '',
    cluster_name TEXT NOT NULL DEFAULT '',
    namespace TEXT NOT NULL DEFAULT '',
    chat_id BIGINT NOT NULL DEFAULT 0,
    chat_type TEXT NOT NULL DEFAULT 'private',
    status TEXT NOT NULL DEFAULT 'success',
    details JSONB DEFAULT '{}',
    error_msg TEXT DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_user_id ON audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_log(action);
CREATE INDEX IF NOT EXISTS idx_audit_occurred_at ON audit_log(occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_cluster ON audit_log(cluster_name);
CREATE INDEX IF NOT EXISTS idx_audit_status ON audit_log(status);
