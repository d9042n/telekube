CREATE TABLE IF NOT EXISTS users (
    telegram_id INTEGER PRIMARY KEY,
    username TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT 'viewer',
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS audit_log (
    id TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL,
    username TEXT NOT NULL,
    action TEXT NOT NULL,
    resource TEXT NOT NULL DEFAULT '',
    cluster TEXT NOT NULL DEFAULT '',
    namespace TEXT NOT NULL DEFAULT '',
    chat_id INTEGER NOT NULL,
    chat_type TEXT NOT NULL DEFAULT 'private',
    status TEXT NOT NULL DEFAULT 'success',
    details TEXT DEFAULT '{}',
    error_msg TEXT DEFAULT '',
    occurred_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_user_id ON audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_log(action);
CREATE INDEX IF NOT EXISTS idx_audit_occurred_at ON audit_log(occurred_at);
CREATE INDEX IF NOT EXISTS idx_audit_cluster ON audit_log(cluster);
