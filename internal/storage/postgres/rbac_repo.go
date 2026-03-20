package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

// RBACRepo implements storage.RBACRepository using PostgreSQL.
type RBACRepo struct {
	pool *pgxpool.Pool
}

func (r *RBACRepo) GetUserRole(ctx context.Context, telegramID int64) (string, error) {
	var role string
	err := r.pool.QueryRow(ctx,
		`SELECT role FROM users WHERE telegram_id = $1`, telegramID,
	).Scan(&role)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", storage.ErrNotFound
		}
		return "", fmt.Errorf("get user role: %w", err)
	}
	return role, nil
}

func (r *RBACRepo) SetUserRole(ctx context.Context, telegramID int64, role string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET role = $2, updated_at = NOW() WHERE telegram_id = $1`,
		telegramID, role,
	)
	if err != nil {
		return fmt.Errorf("set user role: %w", err)
	}
	return nil
}

func (r *RBACRepo) CreateRole(ctx context.Context, role *entity.Role) error {
	rulesJSON, err := json.Marshal(role.Rules)
	if err != nil {
		return fmt.Errorf("marshal rules: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO roles (name, display_name, description, rules, is_builtin, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (name) DO UPDATE
		 SET display_name = EXCLUDED.display_name,
		     description  = EXCLUDED.description,
		     rules        = EXCLUDED.rules`,
		role.Name, role.DisplayName, role.Description, rulesJSON, role.IsBuiltin, role.CreatedAt,
	)
	return err
}

func (r *RBACRepo) GetRole(ctx context.Context, name string) (*entity.Role, error) {
	var role entity.Role
	var rulesJSON []byte
	err := r.pool.QueryRow(ctx,
		`SELECT name, display_name, description, rules, is_builtin, created_at FROM roles WHERE name = $1`,
		name,
	).Scan(&role.Name, &role.DisplayName, &role.Description, &rulesJSON, &role.IsBuiltin, &role.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("get role: %w", err)
	}
	if err := json.Unmarshal(rulesJSON, &role.Rules); err != nil {
		return nil, fmt.Errorf("unmarshal rules: %w", err)
	}
	return &role, nil
}

func (r *RBACRepo) ListRoles(ctx context.Context) ([]entity.Role, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT name, display_name, description, rules, is_builtin, created_at FROM roles ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()

	var roles []entity.Role
	for rows.Next() {
		var role entity.Role
		var rulesJSON []byte
		if err := rows.Scan(&role.Name, &role.DisplayName, &role.Description, &rulesJSON, &role.IsBuiltin, &role.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(rulesJSON, &role.Rules); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *RBACRepo) DeleteRole(ctx context.Context, name string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM roles WHERE name = $1`, name)
	return err
}

func (r *RBACRepo) CreateRoleBinding(ctx context.Context, b *entity.UserRoleBinding) error {
	if b.ID == "" {
		b.ID = ulid.Make().String()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO user_role_bindings (id, user_id, role_name, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, NOW())`,
		b.ID, b.UserID, b.RoleName, b.ExpiresAt,
	)
	return err
}

func (r *RBACRepo) GetUserRoleBindings(ctx context.Context, userID int64) ([]entity.UserRoleBinding, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, role_name, expires_at FROM user_role_bindings WHERE user_id = $1`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBindings(rows)
}

func (r *RBACRepo) DeleteRoleBinding(ctx context.Context, userID int64, roleName string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM user_role_bindings WHERE user_id = $1 AND role_name = $2`,
		userID, roleName,
	)
	return err
}

func (r *RBACRepo) ListAllBindings(ctx context.Context) ([]entity.UserRoleBinding, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, role_name, expires_at FROM user_role_bindings ORDER BY user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBindings(rows)
}

func scanBindings(rows pgx.Rows) ([]entity.UserRoleBinding, error) {
	var out []entity.UserRoleBinding
	for rows.Next() {
		var b entity.UserRoleBinding
		if err := rows.Scan(&b.ID, &b.UserID, &b.RoleName, &b.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ApprovalRepo implements storage.ApprovalRepository using PostgreSQL.
type ApprovalRepo struct {
	pool *pgxpool.Pool
}

func (r *ApprovalRepo) Create(ctx context.Context, req *entity.ApprovalRequest) error {
	detailsJSON, _ := json.Marshal(req.Details)
	approversJSON, _ := json.Marshal(req.Approvers)
	_, err := r.pool.Exec(ctx,
		`INSERT INTO approval_requests
		 (id, requester_id, requester_name, action, resource, cluster, namespace,
		  details, status, approvers, required_approvals, chat_id, message_id,
		  created_at, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		req.ID, req.RequesterID, req.RequesterName, req.Action, req.Resource,
		req.Cluster, req.Namespace, detailsJSON, req.Status, approversJSON,
		req.RequiredApprovals, req.ChatID, req.MessageID,
		req.CreatedAt, req.ExpiresAt,
	)
	return err
}

func (r *ApprovalRepo) GetByID(ctx context.Context, id string) (*entity.ApprovalRequest, error) {
	req, err := r.scanApprovalRow(r.pool.QueryRow(ctx,
		`SELECT id, requester_id, requester_name, action, resource, cluster, namespace,
		        details, status, approvers, required_approvals, chat_id, message_id,
		        created_at, expires_at, resolved_at, resolved_by
		 FROM approval_requests WHERE id = $1`, id))
	if err == pgx.ErrNoRows {
		return nil, storage.ErrNotFound
	}
	return req, err
}

func (r *ApprovalRepo) Update(ctx context.Context, req *entity.ApprovalRequest) error {
	approversJSON, _ := json.Marshal(req.Approvers)
	_, err := r.pool.Exec(ctx,
		`UPDATE approval_requests
		 SET status = $2, approvers = $3, resolved_at = $4, resolved_by = $5, message_id = $6
		 WHERE id = $1`,
		req.ID, req.Status, approversJSON, req.ResolvedAt, req.ResolvedBy, req.MessageID,
	)
	return err
}

func (r *ApprovalRepo) ListPending(ctx context.Context) ([]entity.ApprovalRequest, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, requester_id, requester_name, action, resource, cluster, namespace,
		        details, status, approvers, required_approvals, chat_id, message_id,
		        created_at, expires_at, resolved_at, resolved_by
		 FROM approval_requests WHERE status = 'pending' ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanApprovalRows(rows)
}

func (r *ApprovalRepo) ListByRequester(ctx context.Context, userID int64) ([]entity.ApprovalRequest, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, requester_id, requester_name, action, resource, cluster, namespace,
		        details, status, approvers, required_approvals, chat_id, message_id,
		        created_at, expires_at, resolved_at, resolved_by
		 FROM approval_requests WHERE requester_id = $1 ORDER BY created_at DESC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanApprovalRows(rows)
}

func (r *ApprovalRepo) ExpireOld(ctx context.Context) (int, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE approval_requests
		 SET status = 'expired', resolved_at = NOW()
		 WHERE status = 'pending' AND expires_at < NOW()`)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (r *ApprovalRepo) scanApprovalRow(row pgx.Row) (*entity.ApprovalRequest, error) {
	var req entity.ApprovalRequest
	var detailsJSON, approversJSON []byte
	err := row.Scan(
		&req.ID, &req.RequesterID, &req.RequesterName,
		&req.Action, &req.Resource, &req.Cluster, &req.Namespace,
		&detailsJSON, &req.Status, &approversJSON,
		&req.RequiredApprovals, &req.ChatID, &req.MessageID,
		&req.CreatedAt, &req.ExpiresAt, &req.ResolvedAt, &req.ResolvedBy,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(detailsJSON, &req.Details)
	_ = json.Unmarshal(approversJSON, &req.Approvers)
	return &req, nil
}

func (r *ApprovalRepo) scanApprovalRows(rows pgx.Rows) ([]entity.ApprovalRequest, error) {
	var out []entity.ApprovalRequest
	for rows.Next() {
		var req entity.ApprovalRequest
		var detailsJSON, approversJSON []byte
		if err := rows.Scan(
			&req.ID, &req.RequesterID, &req.RequesterName,
			&req.Action, &req.Resource, &req.Cluster, &req.Namespace,
			&detailsJSON, &req.Status, &approversJSON,
			&req.RequiredApprovals, &req.ChatID, &req.MessageID,
			&req.CreatedAt, &req.ExpiresAt, &req.ResolvedAt, &req.ResolvedBy,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(detailsJSON, &req.Details)
		_ = json.Unmarshal(approversJSON, &req.Approvers)
		out = append(out, req)
	}
	return out, rows.Err()
}

// noopApprovalRepo is used when PostgreSQL is not configured.
type noopApprovalRepo struct{}

func (noopApprovalRepo) Create(_ context.Context, _ *entity.ApprovalRequest) error { return nil }
func (noopApprovalRepo) GetByID(_ context.Context, _ string) (*entity.ApprovalRequest, error) {
	return nil, storage.ErrNotFound
}
func (noopApprovalRepo) Update(_ context.Context, _ *entity.ApprovalRequest) error { return nil }
func (noopApprovalRepo) ListPending(_ context.Context) ([]entity.ApprovalRequest, error) {
	return nil, nil
}
func (noopApprovalRepo) ListByRequester(_ context.Context, _ int64) ([]entity.ApprovalRequest, error) {
	return nil, nil
}
func (noopApprovalRepo) ExpireOld(_ context.Context) (int, error) { return 0, nil }

// ensure compile-time interface satisfaction
var _ storage.ApprovalRepository = (*ApprovalRepo)(nil)
var _ storage.ApprovalRepository = noopApprovalRepo{}


