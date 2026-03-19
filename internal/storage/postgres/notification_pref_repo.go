package postgres

import (
	"context"
	"encoding/json"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NotificationPrefRepo implements storage.NotificationPrefRepository for PostgreSQL.
type NotificationPrefRepo struct {
	pool *pgxpool.Pool
}

func (r *NotificationPrefRepo) Get(ctx context.Context, userID int64) (*entity.NotificationPreference, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT user_id, min_severity, muted_alerts, muted_clusters,
		        quiet_hours_start, quiet_hours_end, timezone
		 FROM notification_prefs WHERE user_id = $1`, userID)

	var pref entity.NotificationPreference
	var mutedAlertsJSON, mutedClustersJSON []byte
	err := row.Scan(
		&pref.UserID, &pref.MinSeverity,
		&mutedAlertsJSON, &mutedClustersJSON,
		&pref.QuietHoursStart, &pref.QuietHoursEnd,
		&pref.Timezone,
	)
	if err == pgx.ErrNoRows {
		return nil, storage.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal(mutedAlertsJSON, &pref.MutedAlerts)
	_ = json.Unmarshal(mutedClustersJSON, &pref.MutedClusters)

	return &pref, nil
}

func (r *NotificationPrefRepo) Upsert(ctx context.Context, pref *entity.NotificationPreference) error {
	mutedAlerts, _ := json.Marshal(pref.MutedAlerts)
	mutedClusters, _ := json.Marshal(pref.MutedClusters)

	_, err := r.pool.Exec(ctx,
		`INSERT INTO notification_prefs
		        (user_id, min_severity, muted_alerts, muted_clusters,
		         quiet_hours_start, quiet_hours_end, timezone)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (user_id) DO UPDATE SET
		        min_severity = EXCLUDED.min_severity,
		        muted_alerts = EXCLUDED.muted_alerts,
		        muted_clusters = EXCLUDED.muted_clusters,
		        quiet_hours_start = EXCLUDED.quiet_hours_start,
		        quiet_hours_end = EXCLUDED.quiet_hours_end,
		        timezone = EXCLUDED.timezone`,
		pref.UserID, pref.MinSeverity,
		mutedAlerts, mutedClusters,
		pref.QuietHoursStart, pref.QuietHoursEnd,
		pref.Timezone,
	)
	return err
}

func (r *NotificationPrefRepo) Delete(ctx context.Context, userID int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM notification_prefs WHERE user_id = $1`, userID)
	return err
}
