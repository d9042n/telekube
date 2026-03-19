package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
)

type auditRepo struct {
	db *sql.DB
}

func (r *auditRepo) Create(ctx context.Context, entry *entity.AuditEntry) error {
	detailsJSON, err := json.Marshal(entry.Details)
	if err != nil {
		detailsJSON = []byte("{}")
	}

	_, err = r.db.ExecContext(ctx,
		`INSERT INTO audit_log (id, user_id, username, action, resource, cluster, namespace, chat_id, chat_type, status, details, error_msg, occurred_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.UserID, entry.Username, entry.Action, entry.Resource,
		entry.Cluster, entry.Namespace, entry.ChatID, entry.ChatType,
		entry.Status, string(detailsJSON), entry.Error, entry.OccurredAt)
	if err != nil {
		return fmt.Errorf("inserting audit entry: %w", err)
	}
	return nil
}

func (r *auditRepo) List(ctx context.Context, filter storage.AuditFilter) ([]entity.AuditEntry, int, error) {
	var conditions []string
	var args []interface{}

	if filter.UserID != nil {
		conditions = append(conditions, "user_id = ?")
		args = append(args, *filter.UserID)
	}
	if filter.Action != nil {
		conditions = append(conditions, "action = ?")
		args = append(args, *filter.Action)
	}
	if filter.Cluster != nil {
		conditions = append(conditions, "cluster = ?")
		args = append(args, *filter.Cluster)
	}
	if filter.Namespace != nil {
		conditions = append(conditions, "namespace = ?")
		args = append(args, *filter.Namespace)
	}
	if filter.Status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, *filter.Status)
	}
	if filter.From != nil {
		conditions = append(conditions, "occurred_at >= ?")
		args = append(args, *filter.From)
	}
	if filter.To != nil {
		conditions = append(conditions, "occurred_at <= ?")
		args = append(args, *filter.To)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_log %s", where)
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting audit entries: %w", err)
	}

	// Paginate
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	query := fmt.Sprintf(
		`SELECT id, user_id, username, action, resource, cluster, namespace, chat_id, chat_type, status, details, error_msg, occurred_at
		 FROM audit_log %s ORDER BY occurred_at DESC LIMIT ? OFFSET ?`, where)

	queryArgs := append(args, pageSize, offset)
	rows, err := r.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying audit entries: %w", err)
	}
	defer rows.Close()

	var entries []entity.AuditEntry
	for rows.Next() {
		var e entity.AuditEntry
		var detailsStr string
		if err := rows.Scan(
			&e.ID, &e.UserID, &e.Username, &e.Action, &e.Resource,
			&e.Cluster, &e.Namespace, &e.ChatID, &e.ChatType,
			&e.Status, &detailsStr, &e.Error, &e.OccurredAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning audit entry: %w", err)
		}
		if detailsStr != "" {
			_ = json.Unmarshal([]byte(detailsStr), &e.Details)
		}
		entries = append(entries, e)
	}

	return entries, total, rows.Err()
}
