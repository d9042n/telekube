package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
)

type userRepo struct {
	db *sql.DB
}

func (r *userRepo) GetByTelegramID(ctx context.Context, telegramID int64) (*entity.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT telegram_id, username, display_name, role, is_active, created_at, updated_at
		 FROM users WHERE telegram_id = ?`, telegramID)

	var u entity.User
	var isActive int
	err := row.Scan(&u.TelegramID, &u.Username, &u.DisplayName, &u.Role, &isActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("querying user: %w", err)
	}
	u.IsActive = isActive == 1
	return &u, nil
}

func (r *userRepo) Upsert(ctx context.Context, user *entity.User) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO users (telegram_id, username, display_name, role, is_active, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(telegram_id) DO UPDATE SET
		   username = excluded.username,
		   display_name = excluded.display_name,
		   updated_at = ?`,
		user.TelegramID, user.Username, user.DisplayName, user.Role,
		boolToInt(user.IsActive), now, now, now)
	if err != nil {
		return fmt.Errorf("upserting user: %w", err)
	}
	return nil
}

func (r *userRepo) List(ctx context.Context) ([]entity.User, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT telegram_id, username, display_name, role, is_active, created_at, updated_at
		 FROM users ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var users []entity.User
	for rows.Next() {
		var u entity.User
		var isActive int
		if err := rows.Scan(&u.TelegramID, &u.Username, &u.DisplayName, &u.Role, &isActive, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		u.IsActive = isActive == 1
		users = append(users, u)
	}
	return users, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
