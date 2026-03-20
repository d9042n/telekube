package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserRepo implements storage.UserRepository using PostgreSQL.
type UserRepo struct {
	pool *pgxpool.Pool
}

func (r *UserRepo) GetByTelegramID(ctx context.Context, telegramID int64) (*entity.User, error) {
	var u entity.User
	err := r.pool.QueryRow(ctx,
		`SELECT telegram_id, username, display_name, role, is_active, created_at, updated_at
		 FROM users WHERE telegram_id = $1`, telegramID,
	).Scan(&u.TelegramID, &u.Username, &u.DisplayName, &u.Role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("get user %d: %w", telegramID, err)
	}
	return &u, nil
}

func (r *UserRepo) Upsert(ctx context.Context, user *entity.User) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (telegram_id, username, display_name, role, is_active, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (telegram_id) DO UPDATE SET
		   username = EXCLUDED.username,
		   display_name = EXCLUDED.display_name,
		   updated_at = EXCLUDED.updated_at`,
		user.TelegramID, user.Username, user.DisplayName, user.Role, user.IsActive, now, now,
	)
	if err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}
	user.CreatedAt = now
	user.UpdatedAt = now
	return nil
}

func (r *UserRepo) List(ctx context.Context) ([]entity.User, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT telegram_id, username, display_name, role, is_active, created_at, updated_at
		 FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []entity.User
	for rows.Next() {
		var u entity.User
		if err := rows.Scan(&u.TelegramID, &u.Username, &u.DisplayName, &u.Role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
