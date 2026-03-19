package rbac

import (
	"context"
	"testing"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRBACRepo implements storage.RBACRepository for testing.
type mockRBACRepo struct {
	roles    map[int64]string
	customs  map[string]*entity.Role
	bindings []entity.UserRoleBinding
}

func newMockRBACRepo() *mockRBACRepo {
	return &mockRBACRepo{
		roles:   make(map[int64]string),
		customs: make(map[string]*entity.Role),
	}
}

func (m *mockRBACRepo) GetUserRole(_ context.Context, telegramID int64) (string, error) {
	role, ok := m.roles[telegramID]
	if !ok {
		return "", storage.ErrNotFound
	}
	return role, nil
}

func (m *mockRBACRepo) SetUserRole(_ context.Context, telegramID int64, role string) error {
	m.roles[telegramID] = role
	return nil
}

func (m *mockRBACRepo) CreateRole(_ context.Context, role *entity.Role) error {
	m.customs[role.Name] = role
	return nil
}

func (m *mockRBACRepo) GetRole(_ context.Context, name string) (*entity.Role, error) {
	r, ok := m.customs[name]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return r, nil
}

func (m *mockRBACRepo) ListRoles(_ context.Context) ([]entity.Role, error) {
	out := make([]entity.Role, 0, len(m.customs))
	for _, r := range m.customs {
		out = append(out, *r)
	}
	return out, nil
}

func (m *mockRBACRepo) DeleteRole(_ context.Context, name string) error {
	delete(m.customs, name)
	return nil
}

func (m *mockRBACRepo) CreateRoleBinding(_ context.Context, b *entity.UserRoleBinding) error {
	m.bindings = append(m.bindings, *b)
	return nil
}

func (m *mockRBACRepo) GetUserRoleBindings(_ context.Context, userID int64) ([]entity.UserRoleBinding, error) {
	var out []entity.UserRoleBinding
	for _, b := range m.bindings {
		if b.UserID == userID {
			out = append(out, b)
		}
	}
	return out, nil
}

func (m *mockRBACRepo) DeleteRoleBinding(_ context.Context, userID int64, roleName string) error {
	filtered := m.bindings[:0]
	for _, b := range m.bindings {
		if !(b.UserID == userID && b.RoleName == roleName) {
			filtered = append(filtered, b)
		}
	}
	m.bindings = filtered
	return nil
}

func (m *mockRBACRepo) ListAllBindings(_ context.Context) ([]entity.UserRoleBinding, error) {
	return m.bindings, nil
}

func TestEngine_HasPermission(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		userID     int64
		role       string
		adminIDs   []int64
		permission string
		expected   bool
	}{
		{
			name:       "viewer can list pods",
			userID:     100,
			role:       entity.RoleViewer,
			permission: PermKubernetesPodsList,
			expected:   true,
		},
		{
			name:       "viewer cannot restart pods",
			userID:     100,
			role:       entity.RoleViewer,
			permission: PermKubernetesPodsRestart,
			expected:   false,
		},
		{
			name:       "operator can restart pods",
			userID:     200,
			role:       entity.RoleOperator,
			permission: PermKubernetesPodsRestart,
			expected:   true,
		},
		{
			name:       "operator cannot delete pods",
			userID:     200,
			role:       entity.RoleOperator,
			permission: PermKubernetesPodsDelete,
			expected:   false,
		},
		{
			name:       "admin can delete pods",
			userID:     300,
			role:       entity.RoleAdmin,
			permission: PermKubernetesPodsDelete,
			expected:   true,
		},
		{
			name:       "admin can manage RBAC",
			userID:     300,
			role:       entity.RoleAdmin,
			permission: PermAdminRBACManage,
			expected:   true,
		},
		{
			name:       "super admin has all permissions",
			userID:     999,
			adminIDs:   []int64{999},
			permission: PermKubernetesPodsDelete,
			expected:   true,
		},
		{
			name:       "super admin has admin-only permissions",
			userID:     999,
			adminIDs:   []int64{999},
			permission: PermAdminRBACManage,
			expected:   true,
		},
		{
			name:       "unknown user defaults to viewer",
			userID:     400,
			permission: PermKubernetesPodsList,
			expected:   true,
		},
		{
			name:       "unknown user cannot restart",
			userID:     400,
			permission: PermKubernetesPodsRestart,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := newMockRBACRepo()
			if tt.role != "" {
				repo.roles[tt.userID] = tt.role
			}

			eng := NewEngine(entity.RoleViewer, repo, tt.adminIDs)
			result, err := eng.HasPermission(context.Background(), tt.userID, tt.permission)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEngine_IsSuperAdmin(t *testing.T) {
	t.Parallel()

	eng := NewEngine(entity.RoleViewer, newMockRBACRepo(), []int64{100, 200})

	assert.True(t, eng.IsSuperAdmin(100))
	assert.True(t, eng.IsSuperAdmin(200))
	assert.False(t, eng.IsSuperAdmin(300))
}

func TestEngine_GetRole_SuperAdmin(t *testing.T) {
	t.Parallel()

	eng := NewEngine(entity.RoleViewer, newMockRBACRepo(), []int64{100})

	role, err := eng.GetRole(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, entity.RoleAdmin, role)
}

func TestEngine_GetRole_DefaultRole(t *testing.T) {
	t.Parallel()

	eng := NewEngine(entity.RoleViewer, newMockRBACRepo(), nil)

	role, err := eng.GetRole(context.Background(), 999)
	require.NoError(t, err)
	assert.Equal(t, entity.RoleViewer, role)
}

func TestEngine_SetRole(t *testing.T) {
	t.Parallel()

	repo := newMockRBACRepo()
	repo.roles[100] = entity.RoleViewer
	eng := NewEngine(entity.RoleViewer, repo, nil)

	err := eng.SetRole(context.Background(), 100, entity.RoleOperator)
	require.NoError(t, err)

	role, err := eng.GetRole(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, entity.RoleOperator, role)
}

func TestEngine_SetRole_InvalidRole(t *testing.T) {
	t.Parallel()

	eng := NewEngine(entity.RoleViewer, newMockRBACRepo(), nil)
	err := eng.SetRole(context.Background(), 100, "invalid")
	assert.Error(t, err)
}

func TestEngine_RolePermissions(t *testing.T) {
	t.Parallel()

	eng := NewEngine(entity.RoleViewer, newMockRBACRepo(), nil)

	perms := eng.RolePermissions(entity.RoleViewer)
	assert.Contains(t, perms, PermKubernetesPodsList)
	assert.NotContains(t, perms, PermKubernetesPodsRestart)

	perms = eng.RolePermissions(entity.RoleOperator)
	assert.Contains(t, perms, PermKubernetesPodsRestart)
	assert.NotContains(t, perms, PermKubernetesPodsDelete)

	perms = eng.RolePermissions(entity.RoleAdmin)
	assert.Contains(t, perms, PermKubernetesPodsDelete)
	assert.Contains(t, perms, PermAdminRBACManage)
}

func TestEngine_Authorize_PolicyRule(t *testing.T) {
	t.Parallel()

	repo := newMockRBACRepo()
	eng := NewEngine(entity.RoleViewer, repo, nil)

	// Create a custom role with a prod-only allow rule
	require.NoError(t, eng.CreateRole(context.Background(), &entity.Role{
		Name: "staging-deployer",
		Rules: []entity.PolicyRule{
			{
				Modules:    []string{"argocd"},
				Resources:  []string{"*"},
				Actions:    []string{"*"},
				Clusters:   []string{"staging"},
				Namespaces: []string{"*"},
				Effect:     "allow",
			},
		},
	}))

	// Bind user 500 to staging-deployer
	require.NoError(t, eng.AssignRole(context.Background(), &entity.UserRoleBinding{
		UserID:   500,
		RoleName: "staging-deployer",
	}))

	// User 500 can sync in staging
	ok, err := eng.Authorize(context.Background(), 500, PermissionRequest{
		Module: "argocd", Resource: "apps", Action: "sync",
		Cluster: "staging", Namespace: "default",
	})
	require.NoError(t, err)
	assert.True(t, ok)

	// User 500 cannot sync in prod
	ok, err = eng.Authorize(context.Background(), 500, PermissionRequest{
		Module: "argocd", Resource: "apps", Action: "sync",
		Cluster: "prod-1", Namespace: "default",
	})
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestEngine_Authorize_DenyOverridesAllow(t *testing.T) {
	t.Parallel()

	repo := newMockRBACRepo()
	eng := NewEngine(entity.RoleViewer, repo, nil)

	require.NoError(t, eng.CreateRole(context.Background(), &entity.Role{
		Name: "deny-prod",
		Rules: []entity.PolicyRule{
			{
				Modules:    []string{"*"},
				Resources:  []string{"*"},
				Actions:    []string{"*"},
				Clusters:   []string{"*"},
				Namespaces: []string{"*"},
				Effect:     "allow",
			},
			{
				Modules:    []string{"kubernetes"},
				Resources:  []string{"nodes"},
				Actions:    []string{"drain"},
				Clusters:   []string{"prod-1"},
				Namespaces: []string{"*"},
				Effect:     "deny",
			},
		},
	}))

	require.NoError(t, eng.AssignRole(context.Background(), &entity.UserRoleBinding{
		UserID:   600,
		RoleName: "deny-prod",
	}))

	// Deny rule matches first
	ok, err := eng.Authorize(context.Background(), 600, PermissionRequest{
		Module: "kubernetes", Resource: "nodes", Action: "drain",
		Cluster: "prod-1", Namespace: "*",
	})
	require.NoError(t, err)
	assert.False(t, ok)
}
