-- Phase 4: Full RBAC v2 — custom roles with policy rules and role bindings

CREATE TABLE IF NOT EXISTS roles (
    name         TEXT        PRIMARY KEY,
    display_name TEXT        NOT NULL DEFAULT '',
    description  TEXT        NOT NULL DEFAULT '',
    rules        JSONB       NOT NULL DEFAULT '[]',
    is_builtin   BOOLEAN     NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_role_bindings (
    id         TEXT        PRIMARY KEY,
    user_id    BIGINT      NOT NULL,
    role_name  TEXT        NOT NULL REFERENCES roles(name) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_role_bindings_user_id  ON user_role_bindings(user_id);
CREATE INDEX IF NOT EXISTS idx_role_bindings_role     ON user_role_bindings(role_name);
CREATE INDEX IF NOT EXISTS idx_role_bindings_expires  ON user_role_bindings(expires_at) WHERE expires_at IS NOT NULL;
