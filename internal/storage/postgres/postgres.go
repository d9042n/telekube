// Package postgres provides a PostgreSQL storage backend.
package postgres

import (
	"context"
	"embed"
	"fmt"
	"time"

	"github.com/d9042n/telekube/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Config holds PostgreSQL connection settings.
type Config struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// Store implements the storage.Storage interface using PostgreSQL.
type Store struct {
	pool      *pgxpool.Pool
	users     *UserRepo
	audit     *AuditRepo
	rbac      *RBACRepo
	freeze    *FreezeRepo
	approval  *ApprovalRepo
	notifPref *NotificationPrefRepo
	logger    *zap.Logger
}

// New creates a new PostgreSQL store.
func New(cfg Config, logger *zap.Logger) (*Store, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parsing postgres DSN: %w", err)
	}

	if cfg.MaxOpenConns > 0 {
		poolConfig.MaxConns = int32(cfg.MaxOpenConns)
	} else {
		poolConfig.MaxConns = 50
	}
	if cfg.MaxIdleConns > 0 {
		poolConfig.MinConns = int32(cfg.MaxIdleConns)
	} else {
		poolConfig.MinConns = 25
	}
	if cfg.ConnMaxLifetime > 0 {
		poolConfig.MaxConnLifetime = cfg.ConnMaxLifetime
	} else {
		poolConfig.MaxConnLifetime = 30 * time.Minute
	}
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.HealthCheckPeriod = 30 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}

	s := &Store{
		pool:   pool,
		logger: logger,
	}

	if err := s.runMigrations(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	s.users = &UserRepo{pool: pool}
	s.audit = &AuditRepo{pool: pool}
	s.rbac = &RBACRepo{pool: pool}
	s.freeze = &FreezeRepo{pool: pool}
	s.approval = &ApprovalRepo{pool: pool}
	s.notifPref = &NotificationPrefRepo{pool: pool}

	logger.Info("postgresql storage initialized",
		zap.Int32("max_conns", poolConfig.MaxConns),
		zap.Int32("min_conns", poolConfig.MinConns),
	)

	return s, nil
}

// runMigrations applies embedded SQL migrations.
func (s *Store) runMigrations(ctx context.Context) error {
	migrationFiles := []string{
		"migrations/000001_initial.up.sql",
		"migrations/000002_argocd_freeze.up.sql",
		"migrations/000003_rbac_v2.up.sql",
		"migrations/000004_approval.up.sql",
		"migrations/000005_notification_prefs.up.sql",
	}
	for _, f := range migrationFiles {
		sql, err := migrations.ReadFile(f)
		if err != nil {
			return fmt.Errorf("reading migration file %s: %w", f, err)
		}
		if _, err := s.pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("executing migration %s: %w", f, err)
		}
	}
	s.logger.Info("migrations applied")
	return nil
}

func (s *Store) Users() storage.UserRepository     { return s.users }
func (s *Store) Audit() storage.AuditRepository    { return s.audit }
func (s *Store) RBAC() storage.RBACRepository      { return s.rbac }
func (s *Store) Freeze() storage.FreezeRepository  { return s.freeze }
func (s *Store) Approval() storage.ApprovalRepository { return s.approval }
func (s *Store) NotificationPrefs() storage.NotificationPrefRepository { return s.notifPref }

func (s *Store) Close() error {
	s.pool.Close()
	return nil
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}
