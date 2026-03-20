// Package rbacmod provides the `/rbac` Telegram command for managing
// roles and role-bindings via inline keyboards.
package rbacmod

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/d9042n/telekube/internal/audit"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/module"
	"github.com/d9042n/telekube/internal/rbac"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/oklog/ulid/v2"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

// Module implements the RBAC management module.
type Module struct {
	rbac    rbac.Engine
	users   storage.UserRepository
	audit   audit.Logger
	logger  *zap.Logger
	healthy bool
}

// NewModule creates a new RBAC management module.
func NewModule(
	rbacEngine rbac.Engine,
	users storage.UserRepository,
	auditLogger audit.Logger,
	logger *zap.Logger,
) *Module {
	return &Module{
		rbac:    rbacEngine,
		users:   users,
		audit:   auditLogger,
		logger:  logger,
		healthy: true,
	}
}

func (m *Module) Name() string        { return "rbac" }
func (m *Module) Description() string { return "RBAC role management via Telegram" }

func (m *Module) Register(bot *telebot.Bot, _ *telebot.Group) {
	bot.Handle("/rbac", m.handleRBAC)

	// Callbacks
	bot.Handle(&telebot.Btn{Unique: "rbac_users"}, m.handleListUsers)
	bot.Handle(&telebot.Btn{Unique: "rbac_roles"}, m.handleListRoles)
	bot.Handle(&telebot.Btn{Unique: "rbac_assign"}, m.handleAssignStart)
	bot.Handle(&telebot.Btn{Unique: "rbac_assign_user"}, m.handleAssignSelectUser)
	bot.Handle(&telebot.Btn{Unique: "rbac_assign_role"}, m.handleAssignConfirm)
	bot.Handle(&telebot.Btn{Unique: "rbac_revoke"}, m.handleRevokeStart)
	bot.Handle(&telebot.Btn{Unique: "rbac_revoke_user"}, m.handleRevokeSelectUser)
	bot.Handle(&telebot.Btn{Unique: "rbac_revoke_confirm"}, m.handleRevokeConfirm)
	bot.Handle(&telebot.Btn{Unique: "rbac_user_detail"}, m.handleUserDetail)
	bot.Handle(&telebot.Btn{Unique: "rbac_back"}, m.handleBack)
}

func (m *Module) Start(_ context.Context) error {
	m.logger.Info("rbac management module started")
	return nil
}

func (m *Module) Stop(_ context.Context) error {
	m.logger.Info("rbac management module stopped")
	return nil
}

func (m *Module) Health() entity.HealthStatus {
	if m.healthy {
		return entity.HealthStatusHealthy
	}
	return entity.HealthStatusUnhealthy
}

func (m *Module) Commands() []module.CommandInfo {
	return []module.CommandInfo{
		{
			Command:     "/rbac",
			Description: "Manage user roles and permissions",
			Permission:  rbac.PermAdminRBACManage,
			ChatType:    "all",
		},
	}
}

// ---------- Handlers ----------

// handleRBAC shows the main RBAC management menu.
func (m *Module) handleRBAC(c telebot.Context) error {
	ctx := context.Background()
	allowed, err := m.rbac.HasPermission(ctx, c.Sender().ID, rbac.PermAdminRBACManage)
	if err != nil || !allowed {
		return c.Send("❌ You don't have permission to manage RBAC.")
	}

	menu := &telebot.ReplyMarkup{}
	btnUsers := menu.Data("👥 List Users", "rbac_users")
	btnRoles := menu.Data("🛡 List Roles", "rbac_roles")
	btnAssign := menu.Data("➕ Assign Role", "rbac_assign")
	btnRevoke := menu.Data("➖ Revoke Role", "rbac_revoke")
	menu.Inline(
		menu.Row(btnUsers, btnRoles),
		menu.Row(btnAssign, btnRevoke),
	)

	return c.Send("🔐 *RBAC Management*\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nSelect an action:", menu, telebot.ModeMarkdown)
}

// handleListUsers shows all users with their roles.
func (m *Module) handleListUsers(c telebot.Context) error {
	ctx := context.Background()
	allowed, err := m.rbac.HasPermission(ctx, c.Sender().ID, rbac.PermAdminRBACManage)
	if err != nil || !allowed {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Permission denied"})
	}

	users, err := m.users.List(ctx)
	if err != nil {
		m.logger.Error("failed to list users", zap.Error(err))
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Failed to load users"})
	}

	if len(users) == 0 {
		menu := &telebot.ReplyMarkup{}
		btnBack := menu.Data("⬅ Back", "rbac_back")
		menu.Inline(menu.Row(btnBack))
		return c.Edit("👥 *Users & Roles*\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nNo users registered yet.", menu, telebot.ModeMarkdown)
	}

	var sb strings.Builder
	sb.WriteString("👥 *Users & Roles*\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	for _, u := range users {
		role, _ := m.rbac.GetRole(ctx, u.TelegramID)
		superTag := ""
		if m.rbac.IsSuperAdmin(u.TelegramID) {
			superTag = " 👑"
		}

		username := u.Username
		if username == "" {
			username = fmt.Sprintf("ID:%d", u.TelegramID)
		}

		fmt.Fprintf(&sb, "• @%s — `%s`%s\n", username, role, superTag)
	}

	// User detail buttons (up to 8 to stay within Telegram limits)
	menu := &telebot.ReplyMarkup{}
	var rows []telebot.Row
	var currentRow []telebot.Btn
	limit := min(len(users), 8)
	for i := 0; i < limit; i++ {
		u := users[i]
		label := u.Username
		if label == "" {
			label = fmt.Sprintf("%d", u.TelegramID)
		}
		btn := menu.Data(label, "rbac_user_detail", fmt.Sprintf("%d", u.TelegramID))
		currentRow = append(currentRow, btn)
		if len(currentRow) == 3 || i == limit-1 {
			rows = append(rows, menu.Row(currentRow...))
			currentRow = nil
		}
	}
	btnBack := menu.Data("⬅ Back", "rbac_back")
	rows = append(rows, menu.Row(btnBack))
	menu.Inline(rows...)

	return c.Edit(sb.String(), menu, telebot.ModeMarkdown)
}

// handleUserDetail shows bindings for a specific user.
func (m *Module) handleUserDetail(c telebot.Context) error {
	ctx := context.Background()
	allowed, err := m.rbac.HasPermission(ctx, c.Sender().ID, rbac.PermAdminRBACManage)
	if err != nil || !allowed {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Permission denied"})
	}

	data := c.Callback().Data
	var userID int64
	if _, parseErr := fmt.Sscanf(data, "%d", &userID); parseErr != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Invalid user ID"})
	}

	// Flat role
	flatRole, _ := m.rbac.GetRole(ctx, userID)

	// Dynamic bindings
	bindings, _ := m.rbac.ListUserBindings(ctx, userID)

	var sb strings.Builder
	fmt.Fprintf(&sb, "👤 *User %d*\n", userID)
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	fmt.Fprintf(&sb, "Legacy role: `%s`\n", flatRole)

	if m.rbac.IsSuperAdmin(userID) {
		sb.WriteString("Status: 👑 Super Admin (config)\n")
	}

	if len(bindings) > 0 {
		sb.WriteString("\n*Dynamic Bindings:*\n")
		for _, b := range bindings {
			expiry := "permanent"
			if b.ExpiresAt != nil {
				if b.IsExpired() {
					expiry = "expired"
				} else {
					expiry = fmt.Sprintf("expires %s", b.ExpiresAt.Format("Jan 02 15:04 UTC"))
				}
			}
			fmt.Fprintf(&sb, "  • `%s` (%s)\n", b.RoleName, expiry)
		}
	} else {
		sb.WriteString("\nNo dynamic role bindings.\n")
	}

	menu := &telebot.ReplyMarkup{}
	btnBack := menu.Data("⬅ Back", "rbac_users")
	menu.Inline(menu.Row(btnBack))

	return c.Edit(sb.String(), menu, telebot.ModeMarkdown)
}

// handleListRoles shows all built-in and custom roles.
func (m *Module) handleListRoles(c telebot.Context) error {
	ctx := context.Background()
	allowed, err := m.rbac.HasPermission(ctx, c.Sender().ID, rbac.PermAdminRBACManage)
	if err != nil || !allowed {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Permission denied"})
	}

	builtinRoles := m.rbac.Roles()
	customRoles, _ := m.rbac.ListRoles(ctx)

	var sb strings.Builder
	sb.WriteString("🛡 *Roles*\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	sb.WriteString("*Built-in:*\n")
	for _, r := range builtinRoles {
		fmt.Fprintf(&sb, "  • `%s` — %s\n", r.Name, r.Description)
	}

	if len(customRoles) > 0 {
		sb.WriteString("\n*Custom:*\n")
		for _, r := range customRoles {
			if r.IsBuiltin {
				continue
			}
			ruleCount := len(r.Rules)
			fmt.Fprintf(&sb, "  • `%s` — %s (%d rules)\n", r.Name, r.Description, ruleCount)
		}
	}

	menu := &telebot.ReplyMarkup{}
	btnBack := menu.Data("⬅ Back", "rbac_back")
	menu.Inline(menu.Row(btnBack))

	return c.Edit(sb.String(), menu, telebot.ModeMarkdown)
}

// handleAssignStart shows user list for role assignment.
func (m *Module) handleAssignStart(c telebot.Context) error {
	ctx := context.Background()
	allowed, err := m.rbac.HasPermission(ctx, c.Sender().ID, rbac.PermAdminRBACManage)
	if err != nil || !allowed {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Permission denied"})
	}

	users, err := m.users.List(ctx)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Failed to load users"})
	}

	if len(users) == 0 {
		return c.Respond(&telebot.CallbackResponse{Text: "No users registered."})
	}

	var sb strings.Builder
	sb.WriteString("➕ *Assign Role*\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	sb.WriteString("Select a user to assign a role to:")

	menu := &telebot.ReplyMarkup{}
	var rows []telebot.Row
	var currentRow []telebot.Btn
	limit := min(len(users), 12)
	for i := 0; i < limit; i++ {
		u := users[i]
		label := u.Username
		if label == "" {
			label = fmt.Sprintf("%d", u.TelegramID)
		}
		btn := menu.Data(label, "rbac_assign_user", fmt.Sprintf("%d", u.TelegramID))
		currentRow = append(currentRow, btn)
		if len(currentRow) == 3 || i == limit-1 {
			rows = append(rows, menu.Row(currentRow...))
			currentRow = nil
		}
	}
	btnBack := menu.Data("⬅ Cancel", "rbac_back")
	rows = append(rows, menu.Row(btnBack))
	menu.Inline(rows...)

	return c.Edit(sb.String(), menu, telebot.ModeMarkdown)
}

// handleAssignSelectUser shows role selection for chosen user.
func (m *Module) handleAssignSelectUser(c telebot.Context) error {
	ctx := context.Background()
	allowed, err := m.rbac.HasPermission(ctx, c.Sender().ID, rbac.PermAdminRBACManage)
	if err != nil || !allowed {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Permission denied"})
	}

	data := c.Callback().Data
	var userID int64
	if _, parseErr := fmt.Sscanf(data, "%d", &userID); parseErr != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Invalid user ID"})
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "➕ *Assign Role to User %d*\n", userID)
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	sb.WriteString("Select a role:")

	validRoles := entity.ValidRoles()
	menu := &telebot.ReplyMarkup{}
	var rows []telebot.Row
	for _, r := range validRoles {
		btn := menu.Data(r, "rbac_assign_role", fmt.Sprintf("%d|%s", userID, r))
		rows = append(rows, menu.Row(btn))
	}
	btnCancel := menu.Data("⬅ Cancel", "rbac_assign")
	rows = append(rows, menu.Row(btnCancel))
	menu.Inline(rows...)

	return c.Edit(sb.String(), menu, telebot.ModeMarkdown)
}

// handleAssignConfirm executes the role assignment.
func (m *Module) handleAssignConfirm(c telebot.Context) error {
	ctx := context.Background()
	allowed, err := m.rbac.HasPermission(ctx, c.Sender().ID, rbac.PermAdminRBACManage)
	if err != nil || !allowed {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Permission denied"})
	}

	data := c.Callback().Data
	parts := strings.SplitN(data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Invalid data"})
	}

	var userID int64
	if _, parseErr := fmt.Sscanf(parts[0], "%d", &userID); parseErr != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Invalid user ID"})
	}
	roleName := parts[1]

	// Set legacy flat role
	if err := m.rbac.SetRole(ctx, userID, roleName); err != nil {
		m.logger.Error("failed to set role", zap.Error(err))
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Failed to assign role"})
	}

	// Also create a dynamic binding for Phase 4 compatibility
	binding := &entity.UserRoleBinding{
		ID:       ulid.Make().String(),
		UserID:   userID,
		RoleName: roleName,
	}
	if assignErr := m.rbac.AssignRole(ctx, binding); assignErr != nil {
		m.logger.Warn("failed to create dynamic binding (non-critical)", zap.Error(assignErr))
	}

	// Audit
	m.audit.Log(entity.AuditEntry{
		ID:        ulid.Make().String(),
		UserID:    c.Sender().ID,
		Username:  c.Sender().Username,
		Action:    "rbac.role.assign",
		Resource:  fmt.Sprintf("user/%d", userID),
		Status:    entity.AuditStatusSuccess,
		Details: map[string]interface{}{
			"role":    roleName,
			"target":  userID,
		},
		OccurredAt: time.Now().UTC(),
	})

	var sb strings.Builder
	sb.WriteString("✅ *Role Assigned*\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	fmt.Fprintf(&sb, "User: `%d`\n", userID)
	fmt.Fprintf(&sb, "Role: `%s`\n", roleName)
	fmt.Fprintf(&sb, "Assigned by: @%s\n", c.Sender().Username)

	menu := &telebot.ReplyMarkup{}
	btnBack := menu.Data("⬅ Back to RBAC", "rbac_back")
	menu.Inline(menu.Row(btnBack))

	return c.Edit(sb.String(), menu, telebot.ModeMarkdown)
}

// handleRevokeStart shows user list for role revocation.
func (m *Module) handleRevokeStart(c telebot.Context) error {
	ctx := context.Background()
	allowed, err := m.rbac.HasPermission(ctx, c.Sender().ID, rbac.PermAdminRBACManage)
	if err != nil || !allowed {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Permission denied"})
	}

	users, err := m.users.List(ctx)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Failed to load users"})
	}

	if len(users) == 0 {
		return c.Respond(&telebot.CallbackResponse{Text: "No users registered."})
	}

	var sb strings.Builder
	sb.WriteString("➖ *Revoke Role*\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	sb.WriteString("Select a user to revoke a role from:")

	menu := &telebot.ReplyMarkup{}
	var rows []telebot.Row
	var currentRow []telebot.Btn
	limit := min(len(users), 12)
	for i := 0; i < limit; i++ {
		u := users[i]
		label := u.Username
		if label == "" {
			label = fmt.Sprintf("%d", u.TelegramID)
		}
		btn := menu.Data(label, "rbac_revoke_user", fmt.Sprintf("%d", u.TelegramID))
		currentRow = append(currentRow, btn)
		if len(currentRow) == 3 || i == limit-1 {
			rows = append(rows, menu.Row(currentRow...))
			currentRow = nil
		}
	}
	btnBack := menu.Data("⬅ Cancel", "rbac_back")
	rows = append(rows, menu.Row(btnBack))
	menu.Inline(rows...)

	return c.Edit(sb.String(), menu, telebot.ModeMarkdown)
}

// handleRevokeSelectUser shows current bindings for chosen user to revoke.
func (m *Module) handleRevokeSelectUser(c telebot.Context) error {
	ctx := context.Background()
	allowed, err := m.rbac.HasPermission(ctx, c.Sender().ID, rbac.PermAdminRBACManage)
	if err != nil || !allowed {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Permission denied"})
	}

	data := c.Callback().Data
	var userID int64
	if _, parseErr := fmt.Sscanf(data, "%d", &userID); parseErr != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Invalid user ID"})
	}

	bindings, _ := m.rbac.ListUserBindings(ctx, userID)
	flatRole, _ := m.rbac.GetRole(ctx, userID)

	var sb strings.Builder
	fmt.Fprintf(&sb, "➖ *Revoke Role from User %d*\n", userID)
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	fmt.Fprintf(&sb, "Current role: `%s`\n", flatRole)

	menu := &telebot.ReplyMarkup{}
	var rows []telebot.Row

	// Dynamic bindings — can be revoked individually
	if len(bindings) > 0 {
		sb.WriteString("\nSelect a binding to revoke:\n")
		for _, b := range bindings {
			btn := menu.Data(
				fmt.Sprintf("🗑 %s", b.RoleName),
				"rbac_revoke_confirm",
				fmt.Sprintf("%d|%s", userID, b.RoleName),
			)
			rows = append(rows, menu.Row(btn))
		}
	}

	// Also allow resetting to viewer
	if flatRole != entity.RoleViewer {
		btnReset := menu.Data(
			"🔄 Reset to viewer",
			"rbac_revoke_confirm",
			fmt.Sprintf("%d|__reset__", userID),
		)
		rows = append(rows, menu.Row(btnReset))
	}

	btnBack := menu.Data("⬅ Cancel", "rbac_revoke")
	rows = append(rows, menu.Row(btnBack))
	menu.Inline(rows...)

	return c.Edit(sb.String(), menu, telebot.ModeMarkdown)
}

// handleRevokeConfirm executes the revocation.
func (m *Module) handleRevokeConfirm(c telebot.Context) error {
	ctx := context.Background()
	allowed, err := m.rbac.HasPermission(ctx, c.Sender().ID, rbac.PermAdminRBACManage)
	if err != nil || !allowed {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Permission denied"})
	}

	data := c.Callback().Data
	parts := strings.SplitN(data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Invalid data"})
	}

	var userID int64
	if _, parseErr := fmt.Sscanf(parts[0], "%d", &userID); parseErr != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Invalid user ID"})
	}
	roleName := parts[1]

	if roleName == "__reset__" {
		// Reset to viewer
		if err := m.rbac.SetRole(ctx, userID, entity.RoleViewer); err != nil {
			return c.Respond(&telebot.CallbackResponse{Text: "❌ Failed to reset role"})
		}
		roleName = "all → viewer"
	} else {
		// Revoke specific dynamic binding
		if err := m.rbac.RevokeRole(ctx, userID, roleName); err != nil {
			m.logger.Error("failed to revoke role", zap.Error(err))
			return c.Respond(&telebot.CallbackResponse{Text: "❌ Failed to revoke role"})
		}
	}

	// Audit
	m.audit.Log(entity.AuditEntry{
		ID:        ulid.Make().String(),
		UserID:    c.Sender().ID,
		Username:  c.Sender().Username,
		Action:    "rbac.role.revoke",
		Resource:  fmt.Sprintf("user/%d", userID),
		Status:    entity.AuditStatusSuccess,
		Details: map[string]interface{}{
			"revoked_role": roleName,
			"target":       userID,
		},
		OccurredAt: time.Now().UTC(),
	})

	var sb strings.Builder
	sb.WriteString("✅ *Role Revoked*\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	fmt.Fprintf(&sb, "User: `%d`\n", userID)
	fmt.Fprintf(&sb, "Revoked: `%s`\n", roleName)
	fmt.Fprintf(&sb, "By: @%s\n", c.Sender().Username)

	menu := &telebot.ReplyMarkup{}
	btnBack := menu.Data("⬅ Back to RBAC", "rbac_back")
	menu.Inline(menu.Row(btnBack))

	return c.Edit(sb.String(), menu, telebot.ModeMarkdown)
}

// handleBack returns to the main RBAC menu.
func (m *Module) handleBack(c telebot.Context) error {
	// Re-render the main menu
	menu := &telebot.ReplyMarkup{}
	btnUsers := menu.Data("👥 List Users", "rbac_users")
	btnRoles := menu.Data("🛡 List Roles", "rbac_roles")
	btnAssign := menu.Data("➕ Assign Role", "rbac_assign")
	btnRevoke := menu.Data("➖ Revoke Role", "rbac_revoke")
	menu.Inline(
		menu.Row(btnUsers, btnRoles),
		menu.Row(btnAssign, btnRevoke),
	)

	return c.Edit("🔐 *RBAC Management*\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nSelect an action:", menu, telebot.ModeMarkdown)
}
