-- Notification preferences
CREATE TABLE IF NOT EXISTS notification_prefs (
    user_id           INTEGER PRIMARY KEY,
    min_severity      TEXT    NOT NULL DEFAULT 'info',
    muted_alerts      TEXT    NOT NULL DEFAULT '[]',
    muted_clusters    TEXT    NOT NULL DEFAULT '[]',
    quiet_hours_start TEXT,
    quiet_hours_end   TEXT,
    timezone          TEXT    NOT NULL DEFAULT 'UTC'
);
