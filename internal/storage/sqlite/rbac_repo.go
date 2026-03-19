package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/oklog/ulid/v2"
)

type rbacRepo struct {
	db *sql.DB
}

func (r *rbacRepo) GetUserRole(ctx context.Context, telegramID int64) (string, error) {
	var role string
	err := r.db.QueryRowContext(ctx,
		`SELECT role FROM users WHERE telegram_id = ?`, telegramID).Scan(&role)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", storage.ErrNotFound
		}
		return "", fmt.Errorf("getting user role: %w", err)
	}
	return role, nil
}

func (r *rbacRepo) SetUserRole(ctx context.Context, telegramID int64, role string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE users SET role = ?, updated_at = CURRENT_TIMESTAMP WHERE telegram_id = ?`,
		role, telegramID)
	if err != nil {
		return fmt.Errorf("setting user role: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return storage.ErrNotFound
	}
	return nil
}

func (r *rbacRepo) CreateRole(ctx context.Context, role *entity.Role) error {
	rulesJSON, err := json.Marshal(role.Rules)
	if err != nil {
		return fmt.Errorf("marshal rules: %w", err)
	}
	isBuiltIn := 0
	if role.IsBuiltin {
		isBuiltIn = 1
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO roles (name, display_name, description, rules, is_builtin, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE
		 SET display_name = excluded.display_name,
		     description  = excluded.description,
		     rules        = excluded.rules`,
		role.Name, role.DisplayName, role.Description, string(rulesJSON), isBuiltIn, role.CreatedAt.UTC().Format(time.RFC3339),
	)
	return err
}

func (r *rbacRepo) GetRole(ctx context.Context, name string) (*entity.Role, error) {
	var role entity.Role
	var rulesJSON string
	var isBuiltin int
	var createdAt string
	err := r.db.QueryRowContext(ctx,
		`SELECT name, display_name, description, rules, is_builtin, created_at FROM roles WHERE name = ?`,
		name,
	).Scan(&role.Name, &role.DisplayName, &role.Description, &rulesJSON, &isBuiltin, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("get role: %w", err)
	}
	role.IsBuiltin = isBuiltin == 1
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		role.CreatedAt = t
	}
	if err := json.Unmarshal([]byte(rulesJSON), &role.Rules); err != nil {
		return nil, fmt.Errorf("unmarshal rules: %w", err)
	}
	return &role, nil
}

func (r *rbacRepo) ListRoles(ctx context.Context) ([]entity.Role, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT name, display_name, description, rules, is_builtin, created_at FROM roles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entity.Role
	for rows.Next() {
		var role entity.Role
		var rulesJSON string
		var isBuiltin int
		var createdAt string
		if err := rows.Scan(&role.Name, &role.DisplayName, &role.Description, &rulesJSON, &isBuiltin, &createdAt); err != nil {
			return nil, err
		}
		role.IsBuiltin = isBuiltin == 1
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			role.CreatedAt = t
		}
		_ = json.Unmarshal([]byte(rulesJSON), &role.Rules)
		out = append(out, role)
	}
	return out, rows.Err()
}

func (r *rbacRepo) DeleteRole(ctx context.Context, name string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM roles WHERE name = ?`, name)
	return err
}

func (r *rbacRepo) CreateRoleBinding(ctx context.Context, b *entity.UserRoleBinding) error {
	if b.ID == "" {
		b.ID = ulid.Make().String()
	}
	var expiresAt *string
	if b.ExpiresAt != nil {
		s := b.ExpiresAt.UTC().Format(time.RFC3339)
		expiresAt = &s
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO user_role_bindings (id, user_id, role_name, expires_at) VALUES (?, ?, ?, ?)`,
		b.ID, b.UserID, b.RoleName, expiresAt,
	)
	return err
}

func (r *rbacRepo) GetUserRoleBindings(ctx context.Context, userID int64) ([]entity.UserRoleBinding, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, role_name, expires_at FROM user_role_bindings WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSQLiteBindings(rows)
}

func (r *rbacRepo) DeleteRoleBinding(ctx context.Context, userID int64, roleName string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM user_role_bindings WHERE user_id = ? AND role_name = ?`, userID, roleName)
	return err
}

func (r *rbacRepo) ListAllBindings(ctx context.Context) ([]entity.UserRoleBinding, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, role_name, expires_at FROM user_role_bindings ORDER BY user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSQLiteBindings(rows)
}

func scanSQLiteBindings(rows *sql.Rows) ([]entity.UserRoleBinding, error) {
	var out []entity.UserRoleBinding
	for rows.Next() {
		var b entity.UserRoleBinding
		var expiresAt *string
		if err := rows.Scan(&b.ID, &b.UserID, &b.RoleName, &expiresAt); err != nil {
			return nil, err
		}
		if expiresAt != nil {
			if t, err := time.Parse(time.RFC3339, *expiresAt); err == nil {
				b.ExpiresAt = &t
			}
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// approvalRepo implements storage.ApprovalRepository using SQLite.
type approvalRepo struct {
	db *sql.DB
}

func (r *approvalRepo) Create(ctx context.Context, req *entity.ApprovalRequest) error {
	detailsJSON, _ := json.Marshal(req.Details)
	approversJSON, _ := json.Marshal(req.Approvers)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO approval_requests
		 (id, requester_id, requester_name, action, resource, cluster, namespace,
		  details, status, approvers, required_approvals, chat_id, message_id,
		  created_at, expires_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		req.ID, req.RequesterID, req.RequesterName, req.Action, req.Resource,
		req.Cluster, req.Namespace, string(detailsJSON), string(req.Status), string(approversJSON),
		req.RequiredApprovals, req.ChatID, req.MessageID,
		req.CreatedAt.UTC().Format(time.RFC3339), req.ExpiresAt.UTC().Format(time.RFC3339),
	)
	return err
}

func (r *approvalRepo) GetByID(ctx context.Context, id string) (*entity.ApprovalRequest, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, requester_id, requester_name, action, resource, cluster, namespace,
		        details, status, approvers, required_approvals, chat_id, message_id,
		        created_at, expires_at, resolved_at, resolved_by
		 FROM approval_requests WHERE id = ?`, id)
	req, err := scanSQLiteApproval(row)
	if err == sql.ErrNoRows {
		return nil, storage.ErrNotFound
	}
	return req, err
}

func (r *approvalRepo) Update(ctx context.Context, req *entity.ApprovalRequest) error {
	approversJSON, _ := json.Marshal(req.Approvers)
	var resolvedAt *string
	if req.ResolvedAt != nil {
		s := req.ResolvedAt.UTC().Format(time.RFC3339)
		resolvedAt = &s
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE approval_requests
		 SET status = ?, approvers = ?, resolved_at = ?, resolved_by = ?, message_id = ?
		 WHERE id = ?`,
		string(req.Status), string(approversJSON), resolvedAt, req.ResolvedBy, req.MessageID, req.ID,
	)
	return err
}

func (r *approvalRepo) ListPending(ctx context.Context) ([]entity.ApprovalRequest, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, requester_id, requester_name, action, resource, cluster, namespace,
		        details, status, approvers, required_approvals, chat_id, message_id,
		        created_at, expires_at, resolved_at, resolved_by
		 FROM approval_requests WHERE status = 'pending' ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSQLiteApprovals(rows)
}

func (r *approvalRepo) ListByRequester(ctx context.Context, userID int64) ([]entity.ApprovalRequest, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, requester_id, requester_name, action, resource, cluster, namespace,
		        details, status, approvers, required_approvals, chat_id, message_id,
		        created_at, expires_at, resolved_at, resolved_by
		 FROM approval_requests WHERE requester_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSQLiteApprovals(rows)
}

func (r *approvalRepo) ExpireOld(ctx context.Context) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := r.db.ExecContext(ctx,
		`UPDATE approval_requests
		 SET status = 'expired', resolved_at = ?
		 WHERE status = 'pending' AND expires_at < ?`, now, now)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func scanSQLiteApproval(row *sql.Row) (*entity.ApprovalRequest, error) {
	var req entity.ApprovalRequest
	var detailsJSON, approversJSON, statusStr string
	var createdAt, expiresAt string
	var resolvedAt *string
	err := row.Scan(
		&req.ID, &req.RequesterID, &req.RequesterName,
		&req.Action, &req.Resource, &req.Cluster, &req.Namespace,
		&detailsJSON, &statusStr, &approversJSON,
		&req.RequiredApprovals, &req.ChatID, &req.MessageID,
		&createdAt, &expiresAt, &resolvedAt, &req.ResolvedBy,
	)
	if err != nil {
		return nil, err
	}
	req.Status = entity.ApprovalStatus(statusStr)
	_ = json.Unmarshal([]byte(detailsJSON), &req.Details)
	_ = json.Unmarshal([]byte(approversJSON), &req.Approvers)
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		req.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, expiresAt); err == nil {
		req.ExpiresAt = t
	}
	if resolvedAt != nil {
		if t, err := time.Parse(time.RFC3339, *resolvedAt); err == nil {
			req.ResolvedAt = &t
		}
	}
	return &req, nil
}

func scanSQLiteApprovals(rows *sql.Rows) ([]entity.ApprovalRequest, error) {
	var out []entity.ApprovalRequest
	for rows.Next() {
		var req entity.ApprovalRequest
		var detailsJSON, approversJSON, statusStr string
		var createdAt, expiresAt string
		var resolvedAt *string
		if err := rows.Scan(
			&req.ID, &req.RequesterID, &req.RequesterName,
			&req.Action, &req.Resource, &req.Cluster, &req.Namespace,
			&detailsJSON, &statusStr, &approversJSON,
			&req.RequiredApprovals, &req.ChatID, &req.MessageID,
			&createdAt, &expiresAt, &resolvedAt, &req.ResolvedBy,
		); err != nil {
			return nil, err
		}
		req.Status = entity.ApprovalStatus(statusStr)
		_ = json.Unmarshal([]byte(detailsJSON), &req.Details)
		_ = json.Unmarshal([]byte(approversJSON), &req.Approvers)
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			req.CreatedAt = t
		}
		if t, err := time.Parse(time.RFC3339, expiresAt); err == nil {
			req.ExpiresAt = t
		}
		if resolvedAt != nil {
			if t, err := time.Parse(time.RFC3339, *resolvedAt); err == nil {
				req.ResolvedAt = &t
			}
		}
		out = append(out, req)
	}
	return out, rows.Err()
}

var _ storage.RBACRepository = (*rbacRepo)(nil)
var _ storage.ApprovalRepository = (*approvalRepo)(nil)
