-- Phase 4: Approval workflow

CREATE TABLE IF NOT EXISTS approval_requests (
    id                  TEXT    NOT NULL PRIMARY KEY,
    requester_id        INTEGER NOT NULL,
    requester_name      TEXT    NOT NULL DEFAULT '',
    action              TEXT    NOT NULL,
    resource            TEXT    NOT NULL DEFAULT '',
    cluster             TEXT    NOT NULL DEFAULT '',
    namespace           TEXT    NOT NULL DEFAULT '',
    details             TEXT    NOT NULL DEFAULT '{}',
    status              TEXT    NOT NULL DEFAULT 'pending',
    approvers           TEXT    NOT NULL DEFAULT '[]',
    required_approvals  INTEGER NOT NULL DEFAULT 1,
    chat_id             INTEGER NOT NULL DEFAULT 0,
    message_id          INTEGER NOT NULL DEFAULT 0,
    created_at          TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    expires_at          TEXT    NOT NULL,
    resolved_at         TEXT,
    resolved_by         INTEGER
);

CREATE INDEX IF NOT EXISTS idx_approval_status    ON approval_requests(status);
CREATE INDEX IF NOT EXISTS idx_approval_requester ON approval_requests(requester_id);
