// Package sqlite provides a SQLite-backed storage implementation.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/d9042n/telekube/internal/storage"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store implements storage.Storage using SQLite.
type Store struct {
	db *sql.DB
}

// New creates a new SQLite store and runs migrations.
func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite db: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("enabling WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return s, nil
}

func (s *Store) migrate() error {
	migrations := []string{
		"migrations/001_initial.sql",
		"migrations/002_argocd_freeze.sql",
		"migrations/003_rbac_v2.sql",
		"migrations/004_approval.sql",
		"migrations/005_notification_prefs.sql",
	}
	for _, name := range migrations {
		migrationSQL, err := migrationsFS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("reading migration file %s: %w", name, err)
		}
		if _, err := s.db.Exec(string(migrationSQL)); err != nil {
			return fmt.Errorf("executing migration %s: %w", name, err)
		}
	}
	return nil
}

// Users returns the user repository.
func (s *Store) Users() storage.UserRepository {
	return &userRepo{db: s.db}
}

// Audit returns the audit repository.
func (s *Store) Audit() storage.AuditRepository {
	return &auditRepo{db: s.db}
}

// Freeze returns the freeze repository.
func (s *Store) Freeze() storage.FreezeRepository {
	return &freezeRepo{db: s.db}
}

// RBAC returns the RBAC repository.
func (s *Store) RBAC() storage.RBACRepository {
	return &rbacRepo{db: s.db}
}

// Approval returns the approval repository.
func (s *Store) Approval() storage.ApprovalRepository {
	return &approvalRepo{db: s.db}
}

// NotificationPrefs returns the notification preferences repository.
func (s *Store) NotificationPrefs() storage.NotificationPrefRepository {
	return &notificationPrefRepo{db: s.db}
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Ping checks the database connection.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

var _ storage.Storage = (*Store)(nil)
