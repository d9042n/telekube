package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FreezeRepo implements storage.FreezeRepository on PostgreSQL.
type FreezeRepo struct {
	pool *pgxpool.Pool
}

func (r *FreezeRepo) Create(ctx context.Context, freeze *entity.DeploymentFreeze) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO deployment_freezes (id, scope, reason, created_by, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		freeze.ID, freeze.Scope, freeze.Reason, freeze.CreatedBy, freeze.CreatedAt, freeze.ExpiresAt)
	if err != nil {
		return fmt.Errorf("inserting deployment freeze: %w", err)
	}
	return nil
}

func (r *FreezeRepo) GetActive(ctx context.Context) (*entity.DeploymentFreeze, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, scope, reason, created_by, created_at, expires_at, thawed_at, thawed_by
		 FROM deployment_freezes
		 WHERE thawed_at IS NULL AND expires_at > $1
		 ORDER BY created_at DESC LIMIT 1`,
		time.Now().UTC())
	return scanPgFreeze(row)
}

func (r *FreezeRepo) GetActiveForCluster(ctx context.Context, clusterName string) (*entity.DeploymentFreeze, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, scope, reason, created_by, created_at, expires_at, thawed_at, thawed_by
		 FROM deployment_freezes
		 WHERE thawed_at IS NULL AND expires_at > $1 AND (scope = 'all' OR scope = $2)
		 ORDER BY created_at DESC LIMIT 1`,
		time.Now().UTC(), clusterName)
	return scanPgFreeze(row)
}

func (r *FreezeRepo) Thaw(ctx context.Context, id string, thawedBy int64) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE deployment_freezes SET thawed_at = $1, thawed_by = $2 WHERE id = $3`,
		time.Now().UTC(), thawedBy, id)
	if err != nil {
		return fmt.Errorf("thawing freeze %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("freeze %s not found", id)
	}
	return nil
}

func (r *FreezeRepo) List(ctx context.Context, limit int) ([]entity.DeploymentFreeze, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, scope, reason, created_by, created_at, expires_at, thawed_at, thawed_by
		 FROM deployment_freezes
		 ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("listing deployment freezes: %w", err)
	}
	defer rows.Close()

	var freezes []entity.DeploymentFreeze
	for rows.Next() {
		f, err := scanPgFreezeRow(rows)
		if err != nil {
			return nil, err
		}
		freezes = append(freezes, *f)
	}
	return freezes, rows.Err()
}

func scanPgFreeze(row pgx.Row) (*entity.DeploymentFreeze, error) {
	var f entity.DeploymentFreeze
	var thawedAt *time.Time
	var thawedBy *int64

	err := row.Scan(&f.ID, &f.Scope, &f.Reason, &f.CreatedBy,
		&f.CreatedAt, &f.ExpiresAt, &thawedAt, &thawedBy)
	if err == pgx.ErrNoRows {
		return nil, nil //nolint:nilnil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning deployment freeze: %w", err)
	}
	f.ThawedAt = thawedAt
	f.ThawedBy = thawedBy
	return &f, nil
}

func scanPgFreezeRow(rows pgx.Rows) (*entity.DeploymentFreeze, error) {
	var f entity.DeploymentFreeze
	var thawedAt *time.Time
	var thawedBy *int64

	err := rows.Scan(&f.ID, &f.Scope, &f.Reason, &f.CreatedBy,
		&f.CreatedAt, &f.ExpiresAt, &thawedAt, &thawedBy)
	if err != nil {
		return nil, fmt.Errorf("scanning deployment freeze row: %w", err)
	}
	f.ThawedAt = thawedAt
	f.ThawedBy = thawedBy
	return &f, nil
}

// compile-time check
var _ storage.FreezeRepository = (*FreezeRepo)(nil)
