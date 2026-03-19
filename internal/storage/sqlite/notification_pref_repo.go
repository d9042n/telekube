package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
)

type notificationPrefRepo struct {
	db *sql.DB
}

func (r *notificationPrefRepo) Get(ctx context.Context, userID int64) (*entity.NotificationPreference, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT user_id, min_severity, muted_alerts, muted_clusters,
		        quiet_hours_start, quiet_hours_end, timezone
		 FROM notification_prefs WHERE user_id = ?`, userID)

	var pref entity.NotificationPreference
	var mutedAlertsJSON, mutedClustersJSON string
	err := row.Scan(
		&pref.UserID, &pref.MinSeverity,
		&mutedAlertsJSON, &mutedClustersJSON,
		&pref.QuietHoursStart, &pref.QuietHoursEnd,
		&pref.Timezone,
	)
	if err == sql.ErrNoRows {
		return nil, storage.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal([]byte(mutedAlertsJSON), &pref.MutedAlerts)
	_ = json.Unmarshal([]byte(mutedClustersJSON), &pref.MutedClusters)

	return &pref, nil
}

func (r *notificationPrefRepo) Upsert(ctx context.Context, pref *entity.NotificationPreference) error {
	mutedAlerts, _ := json.Marshal(pref.MutedAlerts)
	mutedClusters, _ := json.Marshal(pref.MutedClusters)

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO notification_prefs
		        (user_id, min_severity, muted_alerts, muted_clusters,
		         quiet_hours_start, quiet_hours_end, timezone)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (user_id) DO UPDATE SET
		        min_severity = excluded.min_severity,
		        muted_alerts = excluded.muted_alerts,
		        muted_clusters = excluded.muted_clusters,
		        quiet_hours_start = excluded.quiet_hours_start,
		        quiet_hours_end = excluded.quiet_hours_end,
		        timezone = excluded.timezone`,
		pref.UserID, pref.MinSeverity,
		string(mutedAlerts), string(mutedClusters),
		pref.QuietHoursStart, pref.QuietHoursEnd,
		pref.Timezone,
	)
	return err
}

func (r *notificationPrefRepo) Delete(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM notification_prefs WHERE user_id = ?`, userID)
	return err
}
