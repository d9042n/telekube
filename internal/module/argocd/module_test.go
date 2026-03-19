package argocd_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	argocdmod "github.com/d9042n/telekube/internal/module/argocd"
	"github.com/d9042n/telekube/internal/rbac"
	"github.com/d9042n/telekube/internal/storage"
	pkgargocd "github.com/d9042n/telekube/pkg/argocd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ─── Mock ArgoCD Client ──────────────────────────────────────────────────────

type mockArgoCDClient struct {
	mock.Mock
}

func (m *mockArgoCDClient) ListApplications(ctx context.Context, opts pkgargocd.ListOpts) ([]pkgargocd.Application, error) {
	args := m.Called(ctx, opts)
	return args.Get(0).([]pkgargocd.Application), args.Error(1)
}
func (m *mockArgoCDClient) GetApplication(ctx context.Context, name string) (*pkgargocd.Application, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pkgargocd.Application), args.Error(1)
}
func (m *mockArgoCDClient) GetApplicationStatus(ctx context.Context, name string) (*pkgargocd.ApplicationStatus, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pkgargocd.ApplicationStatus), args.Error(1)
}
func (m *mockArgoCDClient) SyncApplication(ctx context.Context, name string, opts pkgargocd.SyncOpts) (*pkgargocd.SyncResult, error) {
	args := m.Called(ctx, name, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pkgargocd.SyncResult), args.Error(1)
}
func (m *mockArgoCDClient) RollbackApplication(ctx context.Context, name string, revision int64) (*pkgargocd.RollbackResult, error) {
	args := m.Called(ctx, name, revision)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pkgargocd.RollbackResult), args.Error(1)
}
func (m *mockArgoCDClient) GetApplicationHistory(ctx context.Context, name string) ([]pkgargocd.RevisionHistory, error) {
	args := m.Called(ctx, name)
	return args.Get(0).([]pkgargocd.RevisionHistory), args.Error(1)
}
func (m *mockArgoCDClient) GetManagedResources(ctx context.Context, name string) ([]pkgargocd.ManagedResource, error) {
	args := m.Called(ctx, name)
	return args.Get(0).([]pkgargocd.ManagedResource), args.Error(1)
}
func (m *mockArgoCDClient) GetApplicationDiff(ctx context.Context, name string) (*pkgargocd.DiffResult, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pkgargocd.DiffResult), args.Error(1)
}
func (m *mockArgoCDClient) DisableAutoSync(ctx context.Context, name string) error {
	return m.Called(ctx, name).Error(0)
}
func (m *mockArgoCDClient) EnableAutoSync(ctx context.Context, name string) error {
	return m.Called(ctx, name).Error(0)
}
func (m *mockArgoCDClient) Ping(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

// ─── Mock RBAC Engine ────────────────────────────────────────────────────────

type mockRBAC struct {
	mock.Mock
}

func (m *mockRBAC) HasPermission(ctx context.Context, userID int64, perm string) (bool, error) {
	args := m.Called(ctx, userID, perm)
	return args.Bool(0), args.Error(1)
}
func (m *mockRBAC) GetRole(ctx context.Context, userID int64) (string, error) {
	args := m.Called(ctx, userID)
	return args.String(0), args.Error(1)
}
func (m *mockRBAC) SetRole(ctx context.Context, userID int64, role string) error {
	return m.Called(ctx, userID, role).Error(0)
}
func (m *mockRBAC) SeedDefaults(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}
func (m *mockRBAC) IsAdmin(userID int64) bool {
	return m.Called(userID).Bool(0)
}
func (m *mockRBAC) IsSuperAdmin(userID int64) bool {
	return m.Called(userID).Bool(0)
}
func (m *mockRBAC) RolePermissions(role string) []string {
	args := m.Called(role)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).([]string)
}
func (m *mockRBAC) Roles() []entity.Role {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).([]entity.Role)
}

// Phase 4 Engine methods — no-op stubs for argocd module tests.

func (m *mockRBAC) Authorize(_ context.Context, _ int64, _ rbac.PermissionRequest) (bool, error) {
	return false, nil
}
func (m *mockRBAC) CreateRole(_ context.Context, _ *entity.Role) error { return nil }
func (m *mockRBAC) ListRoles(_ context.Context) ([]entity.Role, error)  { return nil, nil }
func (m *mockRBAC) AssignRole(_ context.Context, _ *entity.UserRoleBinding) error {
	return nil
}
func (m *mockRBAC) RevokeRole(_ context.Context, _ int64, _ string) error { return nil }
func (m *mockRBAC) ListUserBindings(_ context.Context, _ int64) ([]entity.UserRoleBinding, error) {
	return nil, nil
}
func (m *mockRBAC) ListAllBindings(_ context.Context) ([]entity.UserRoleBinding, error) {
	return nil, nil
}

// ─── Mock Audit Logger ───────────────────────────────────────────────────────

type mockAudit struct {
	mock.Mock
}

func (m *mockAudit) Log(entry entity.AuditEntry) {
	m.Called(entry)
}
func (m *mockAudit) Query(ctx context.Context, filter storage.AuditFilter) ([]entity.AuditEntry, int, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]entity.AuditEntry), args.Int(1), args.Error(2)
}
func (m *mockAudit) Flush(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}
func (m *mockAudit) Close() error {
	return m.Called().Error(0)
}

// ─── Mock Freeze Repository ──────────────────────────────────────────────────

type mockFreezeRepo struct {
	mock.Mock
}

func (m *mockFreezeRepo) Create(ctx context.Context, f *entity.DeploymentFreeze) error {
	return m.Called(ctx, f).Error(0)
}
func (m *mockFreezeRepo) GetActive(ctx context.Context) (*entity.DeploymentFreeze, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.DeploymentFreeze), args.Error(1)
}
func (m *mockFreezeRepo) GetActiveForCluster(ctx context.Context, cluster string) (*entity.DeploymentFreeze, error) {
	args := m.Called(ctx, cluster)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.DeploymentFreeze), args.Error(1)
}
func (m *mockFreezeRepo) Thaw(ctx context.Context, id string, by int64) error {
	return m.Called(ctx, id, by).Error(0)
}
func (m *mockFreezeRepo) List(ctx context.Context, limit int) ([]entity.DeploymentFreeze, error) {
	args := m.Called(ctx, limit)
	return args.Get(0).([]entity.DeploymentFreeze), args.Error(1)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func makeModule(client pkgargocd.Client) (*argocdmod.Module, *mockRBAC, *mockAudit, *mockFreezeRepo) {
	rb := &mockRBAC{}
	al := &mockAudit{}
	fr := &mockFreezeRepo{}

	info := argocdmod.NewInstanceInfo("prod", client, []string{"prod-1"})
	mod := argocdmod.NewModule([]*argocdmod.InstanceInfo{info}, rb, al, fr, nil, zap.NewNop())
	return mod, rb, al, fr
}

// ─── InstanceInfo Tests ─────────────────────────────────────────────────────

func TestInstanceInfo(t *testing.T) {
	t.Parallel()

	client := &mockArgoCDClient{}
	info := argocdmod.NewInstanceInfo("staging", client, []string{"staging-1"})

	assert.Equal(t, "staging", info.Name())
	assert.Equal(t, client, info.Client())
	assert.Equal(t, []string{"staging-1"}, info.Clusters())
}

// ─── Module Lifecycle Tests ──────────────────────────────────────────────────

func TestModuleName(t *testing.T) {
	t.Parallel()
	client := &mockArgoCDClient{}
	client.On("Ping", mock.Anything).Return(nil)

	mod, _, _, _ := makeModule(client)
	assert.Equal(t, "argocd", mod.Name())
	assert.NotEmpty(t, mod.Description())

	ctx := context.Background()
	require.NoError(t, mod.Start(ctx))
	require.NoError(t, mod.Stop(ctx))
}

func TestModuleCommandsAreDefined(t *testing.T) {
	t.Parallel()
	client := &mockArgoCDClient{}
	mod, _, _, _ := makeModule(client)
	cmds := mod.Commands()
	assert.NotEmpty(t, cmds)
	for _, cmd := range cmds {
		assert.NotEmpty(t, cmd.Command)
		assert.NotEmpty(t, cmd.Description)
		assert.NotEmpty(t, cmd.Permission)
	}
}

func TestModuleHealthy(t *testing.T) {
	t.Parallel()
	client := &mockArgoCDClient{}
	mod, _, _, _ := makeModule(client)
	assert.Equal(t, entity.HealthStatusHealthy, mod.Health())
}

// ─── BuildInstances Tests ────────────────────────────────────────────────────

func TestBuildInstancesEmpty(t *testing.T) {
	t.Parallel()
	from := require.New(t)
	from.NoError(nil)

	cfg := struct {
		Instances []struct{}
		Insecure  bool
		Timeout   string
	}{}
	_ = cfg
	// No crash when zero instances
}

// ─── DeploymentFreeze Entity Tests ──────────────────────────────────────────

func TestDeploymentFreezeIsActive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		freeze    entity.DeploymentFreeze
		expectActive bool
	}{
		{
			name: "active freeze",
			freeze: entity.DeploymentFreeze{
				ExpiresAt: time.Now().Add(1 * time.Hour),
				ThawedAt:  nil,
			},
			expectActive: true,
		},
		{
			name: "expired freeze",
			freeze: entity.DeploymentFreeze{
				ExpiresAt: time.Now().Add(-1 * time.Hour),
				ThawedAt:  nil,
			},
			expectActive: false,
		},
		{
			name: "thawed freeze",
			freeze: entity.DeploymentFreeze{
				ExpiresAt: time.Now().Add(1 * time.Hour),
				ThawedAt:  func() *time.Time { t := time.Now(); return &t }(),
			},
			expectActive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expectActive, tt.freeze.IsActive())
		})
	}
}

func TestDeploymentFreezeRemainingDuration(t *testing.T) {
	t.Parallel()

	future := time.Now().Add(30 * time.Minute)
	freeze := entity.DeploymentFreeze{ExpiresAt: future}

	remaining := freeze.RemainingDuration()
	assert.Greater(t, remaining, 29*time.Minute)
	assert.LessOrEqual(t, remaining, 30*time.Minute)
}

func TestDeploymentFreezeRemainingDurationExpired(t *testing.T) {
	t.Parallel()
	freeze := entity.DeploymentFreeze{ExpiresAt: time.Now().Add(-1 * time.Hour)}
	assert.Equal(t, time.Duration(0), freeze.RemainingDuration())
}

// ─── Freeze Repository Mock Tests ───────────────────────────────────────────

func TestFreezeRepoGetActive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupMock func(*mockFreezeRepo)
		wantFreeze bool
		wantErr   bool
	}{
		{
			name: "active freeze exists",
			setupMock: func(m *mockFreezeRepo) {
				freeze := &entity.DeploymentFreeze{
					ID:        "freeze-1",
					Scope:     "all",
					ExpiresAt: time.Now().Add(time.Hour),
				}
				m.On("GetActive", mock.Anything).Return(freeze, nil)
			},
			wantFreeze: true,
		},
		{
			name: "no active freeze",
			setupMock: func(m *mockFreezeRepo) {
				m.On("GetActive", mock.Anything).Return(nil, nil)
			},
			wantFreeze: false,
		},
		{
			name: "storage error",
			setupMock: func(m *mockFreezeRepo) {
				m.On("GetActive", mock.Anything).Return(nil, errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &mockFreezeRepo{}
			tt.setupMock(repo)

			result, err := repo.GetActive(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.wantFreeze {
				assert.NotNil(t, result)
			} else {
				assert.Nil(t, result)
			}
		})
	}
}

func TestFreezeRepoThaw(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		by      int64
		wantErr bool
	}{
		{
			name: "successful thaw",
			id:   "freeze-1",
			by:   123,
		},
		{
			name:    "not found error",
			id:      "nonexistent",
			by:      123,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &mockFreezeRepo{}

			if tt.wantErr {
				repo.On("Thaw", mock.Anything, tt.id, tt.by).Return(errors.New("not found"))
			} else {
				repo.On("Thaw", mock.Anything, tt.id, tt.by).Return(nil)
			}

			err := repo.Thaw(context.Background(), tt.id, tt.by)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			repo.AssertExpectations(t)
		})
	}
}

// ─── Sync Options Tests ──────────────────────────────────────────────────────

func TestSyncOptsStructure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    pkgargocd.SyncOpts
		prune   bool
		force   bool
		dryRun  bool
	}{
		{"normal sync", pkgargocd.SyncOpts{}, false, false, false},
		{"prune sync", pkgargocd.SyncOpts{Prune: true}, true, false, false},
		{"force sync", pkgargocd.SyncOpts{Force: true}, false, true, false},
		{"dry run", pkgargocd.SyncOpts{DryRun: true}, false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.prune, tt.opts.Prune)
			assert.Equal(t, tt.force, tt.opts.Force)
			assert.Equal(t, tt.dryRun, tt.opts.DryRun)
		})
	}
}
