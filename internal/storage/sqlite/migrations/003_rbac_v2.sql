-- Phase 4: RBAC v2

CREATE TABLE IF NOT EXISTS roles (
    name         TEXT    NOT NULL PRIMARY KEY,
    display_name TEXT    NOT NULL DEFAULT '',
    description  TEXT    NOT NULL DEFAULT '',
    rules        TEXT    NOT NULL DEFAULT '[]',
    is_builtin   INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS user_role_bindings (
    id         TEXT    NOT NULL PRIMARY KEY,
    user_id    INTEGER NOT NULL,
    role_name  TEXT    NOT NULL REFERENCES roles(name) ON DELETE CASCADE,
    expires_at TEXT,
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_role_bindings_user_id ON user_role_bindings(user_id);
