package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditRepo implements storage.AuditRepository using PostgreSQL.
type AuditRepo struct {
	pool *pgxpool.Pool
}

func (r *AuditRepo) Create(ctx context.Context, entry *entity.AuditEntry) error {
	detailsJSON, _ := json.Marshal(entry.Details)
	if detailsJSON == nil {
		detailsJSON = []byte("{}")
	}

	_, err := r.pool.Exec(ctx,
		`INSERT INTO audit_log (id, user_id, username, action, resource, cluster_name, namespace,
		 chat_id, chat_type, status, details, error_msg, occurred_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		entry.ID, entry.UserID, entry.Username, entry.Action, entry.Resource,
		entry.Cluster, entry.Namespace, entry.ChatID, entry.ChatType,
		entry.Status, detailsJSON, entry.Error, entry.OccurredAt,
	)
	if err != nil {
		return fmt.Errorf("create audit entry: %w", err)
	}
	return nil
}

func (r *AuditRepo) List(ctx context.Context, filter storage.AuditFilter) ([]entity.AuditEntry, int, error) {
	var conditions []string
	var args []interface{}
	argN := 1

	if filter.UserID != nil {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", argN))
		args = append(args, *filter.UserID)
		argN++
	}
	if filter.Action != nil {
		conditions = append(conditions, fmt.Sprintf("action LIKE $%d", argN))
		args = append(args, *filter.Action+"%")
		argN++
	}
	if filter.Cluster != nil {
		conditions = append(conditions, fmt.Sprintf("cluster_name = $%d", argN))
		args = append(args, *filter.Cluster)
		argN++
	}
	if filter.Namespace != nil {
		conditions = append(conditions, fmt.Sprintf("namespace = $%d", argN))
		args = append(args, *filter.Namespace)
		argN++
	}
	if filter.Status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argN))
		args = append(args, *filter.Status)
		argN++
	}
	if filter.From != nil {
		conditions = append(conditions, fmt.Sprintf("occurred_at >= $%d", argN))
		args = append(args, *filter.From)
		argN++
	}
	if filter.To != nil {
		conditions = append(conditions, fmt.Sprintf("occurred_at <= $%d", argN))
		args = append(args, *filter.To)
		argN++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count query
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_log %s", whereClause)
	var totalCount int
	err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("count audit entries: %w", err)
	}

	// Data query with pagination
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
	dataQuery := fmt.Sprintf(
		`SELECT id, user_id, username, action, resource, cluster_name, namespace,
		 chat_id, chat_type, status, details, error_msg, occurred_at
		 FROM audit_log %s
		 ORDER BY occurred_at DESC
		 LIMIT $%d OFFSET $%d`,
		whereClause, argN, argN+1,
	)
	args = append(args, pageSize, offset)

	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit entries: %w", err)
	}
	defer rows.Close()

	var entries []entity.AuditEntry
	for rows.Next() {
		var e entity.AuditEntry
		var detailsJSON []byte
		if err := rows.Scan(&e.ID, &e.UserID, &e.Username, &e.Action, &e.Resource,
			&e.Cluster, &e.Namespace, &e.ChatID, &e.ChatType,
			&e.Status, &detailsJSON, &e.Error, &e.OccurredAt); err != nil {
			return nil, 0, fmt.Errorf("scan audit entry: %w", err)
		}
		if len(detailsJSON) > 0 {
			_ = json.Unmarshal(detailsJSON, &e.Details)
		}
		entries = append(entries, e)
	}

	return entries, totalCount, rows.Err()
}
