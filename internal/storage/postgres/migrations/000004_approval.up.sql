-- Phase 4: Approval workflow table

CREATE TABLE IF NOT EXISTS approval_requests (
    id                  TEXT        PRIMARY KEY,
    requester_id        BIGINT      NOT NULL,
    requester_name      TEXT        NOT NULL DEFAULT '',
    action              TEXT        NOT NULL,
    resource            TEXT        NOT NULL DEFAULT '',
    cluster             TEXT        NOT NULL DEFAULT '',
    namespace           TEXT        NOT NULL DEFAULT '',
    details             JSONB       NOT NULL DEFAULT '{}',
    status              TEXT        NOT NULL DEFAULT 'pending',
    approvers           JSONB       NOT NULL DEFAULT '[]',
    required_approvals  INT         NOT NULL DEFAULT 1,
    chat_id             BIGINT      NOT NULL DEFAULT 0,
    message_id          INT         NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at          TIMESTAMPTZ NOT NULL,
    resolved_at         TIMESTAMPTZ,
    resolved_by         BIGINT
);

CREATE INDEX IF NOT EXISTS idx_approval_status    ON approval_requests(status);
CREATE INDEX IF NOT EXISTS idx_approval_requester ON approval_requests(requester_id);
CREATE INDEX IF NOT EXISTS idx_approval_expires   ON approval_requests(expires_at) WHERE status = 'pending';
