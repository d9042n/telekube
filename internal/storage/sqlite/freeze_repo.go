package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/d9042n/telekube/internal/entity"
)

type freezeRepo struct {
	db *sql.DB
}

func (r *freezeRepo) Create(ctx context.Context, freeze *entity.DeploymentFreeze) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO deployment_freezes (id, scope, reason, created_by, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		freeze.ID, freeze.Scope, freeze.Reason, freeze.CreatedBy, freeze.CreatedAt, freeze.ExpiresAt)
	if err != nil {
		return fmt.Errorf("inserting deployment freeze: %w", err)
	}
	return nil
}

func (r *freezeRepo) GetActive(ctx context.Context) (*entity.DeploymentFreeze, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, scope, reason, created_by, created_at, expires_at, thawed_at, thawed_by
		 FROM deployment_freezes
		 WHERE thawed_at IS NULL AND expires_at > ?
		 ORDER BY created_at DESC LIMIT 1`,
		time.Now().UTC())
	return scanFreeze(row)
}

func (r *freezeRepo) GetActiveForCluster(ctx context.Context, clusterName string) (*entity.DeploymentFreeze, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, scope, reason, created_by, created_at, expires_at, thawed_at, thawed_by
		 FROM deployment_freezes
		 WHERE thawed_at IS NULL AND expires_at > ? AND (scope = 'all' OR scope = ?)
		 ORDER BY created_at DESC LIMIT 1`,
		time.Now().UTC(), clusterName)
	return scanFreeze(row)
}

func (r *freezeRepo) Thaw(ctx context.Context, id string, thawedBy int64) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx,
		`UPDATE deployment_freezes SET thawed_at = ?, thawed_by = ? WHERE id = ?`,
		now, thawedBy, id)
	if err != nil {
		return fmt.Errorf("thawing freeze %s: %w", id, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("freeze %s not found", id)
	}
	return nil
}

func (r *freezeRepo) List(ctx context.Context, limit int) ([]entity.DeploymentFreeze, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, scope, reason, created_by, created_at, expires_at, thawed_at, thawed_by
		 FROM deployment_freezes
		 ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("listing deployment freezes: %w", err)
	}
	defer rows.Close()

	var freezes []entity.DeploymentFreeze
	for rows.Next() {
		f, err := scanFreezeRow(rows)
		if err != nil {
			return nil, err
		}
		freezes = append(freezes, *f)
	}
	return freezes, rows.Err()
}

// scanFreeze scans a single *sql.Row into a DeploymentFreeze.
func scanFreeze(row *sql.Row) (*entity.DeploymentFreeze, error) {
	var f entity.DeploymentFreeze
	var thawedAt sql.NullTime
	var thawedBy sql.NullInt64

	err := row.Scan(&f.ID, &f.Scope, &f.Reason, &f.CreatedBy,
		&f.CreatedAt, &f.ExpiresAt, &thawedAt, &thawedBy)
	if err == sql.ErrNoRows {
		return nil, nil //nolint:nilnil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning deployment freeze: %w", err)
	}
	if thawedAt.Valid {
		t := thawedAt.Time
		f.ThawedAt = &t
	}
	if thawedBy.Valid {
		v := thawedBy.Int64
		f.ThawedBy = &v
	}
	return &f, nil
}

// scanFreezeRow scans a *sql.Rows into a DeploymentFreeze.
func scanFreezeRow(rows *sql.Rows) (*entity.DeploymentFreeze, error) {
	var f entity.DeploymentFreeze
	var thawedAt sql.NullTime
	var thawedBy sql.NullInt64

	err := rows.Scan(&f.ID, &f.Scope, &f.Reason, &f.CreatedBy,
		&f.CreatedAt, &f.ExpiresAt, &thawedAt, &thawedBy)
	if err != nil {
		return nil, fmt.Errorf("scanning deployment freeze row: %w", err)
	}
	if thawedAt.Valid {
		t := thawedAt.Time
		f.ThawedAt = &t
	}
	if thawedBy.Valid {
		v := thawedBy.Int64
		f.ThawedBy = &v
	}
	return &f, nil
}
