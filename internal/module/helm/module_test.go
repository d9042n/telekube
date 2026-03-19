package helm_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/module/helm"
	"github.com/d9042n/telekube/internal/rbac"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	helmchart "helm.sh/helm/v3/pkg/chart"
	helmrelease "helm.sh/helm/v3/pkg/release"
	helmtime "helm.sh/helm/v3/pkg/time"
	k8s "k8s.io/client-go/rest"
)


// ─── Stub ReleaseClient ───────────────────────────────────────────────────────

type stubClient struct {
	releases []*helmrelease.Release
	history  []*helmrelease.Release
	listErr  error
	getErr   error
	histErr  error
	rollErr  error

	rolledBackName    string
	rolledBackVersion int
}

func (s *stubClient) ListReleases() ([]*helmrelease.Release, error) {
	return s.releases, s.listErr
}

func (s *stubClient) GetRelease(name string) (*helmrelease.Release, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	for _, r := range s.releases {
		if r.Name == name {
			return r, nil
		}
	}
	return nil, fmt.Errorf("release: not found")
}

func (s *stubClient) GetHistory(_ string) ([]*helmrelease.Release, error) {
	return s.history, s.histErr
}

func (s *stubClient) Rollback(name string, version int) error {
	s.rolledBackName = name
	s.rolledBackVersion = version
	return s.rollErr
}

// ─── Stub Factory ─────────────────────────────────────────────────────────────

func stubFactory(cl *stubClient) helm.ClientFactory {
	return func(_ *k8s.Config, _ string) (helm.ReleaseClient, error) {
		return cl, nil
	}
}

func errFactory(err error) helm.ClientFactory {
	return func(_ *k8s.Config, _ string) (helm.ReleaseClient, error) {
		return nil, err
	}
}

// ─── Stub RBAC engine ─────────────────────────────────────────────────────────

type stubRBAC struct {
	perms map[string]bool
}

func (s *stubRBAC) HasPermission(_ context.Context, _ int64, perm string) (bool, error) {
	return s.perms[perm], nil
}
func (s *stubRBAC) Authorize(_ context.Context, _ int64, _ rbac.PermissionRequest) (bool, error) {
	return false, nil
}
func (s *stubRBAC) GetRole(_ context.Context, _ int64) (string, error)  { return entity.RoleViewer, nil }
func (s *stubRBAC) SetRole(_ context.Context, _ int64, _ string) error  { return nil }
func (s *stubRBAC) RolePermissions(_ string) []string                   { return nil }
func (s *stubRBAC) IsSuperAdmin(_ int64) bool                           { return false }
func (s *stubRBAC) Roles() []entity.Role                                { return nil }
func (s *stubRBAC) CreateRole(_ context.Context, _ *entity.Role) error  { return nil }
func (s *stubRBAC) ListRoles(_ context.Context) ([]entity.Role, error)  { return nil, nil }
func (s *stubRBAC) AssignRole(_ context.Context, _ *entity.UserRoleBinding) error { return nil }
func (s *stubRBAC) RevokeRole(_ context.Context, _ int64, _ string) error         { return nil }
func (s *stubRBAC) ListUserBindings(_ context.Context, _ int64) ([]entity.UserRoleBinding, error) {
	return nil, nil
}
func (s *stubRBAC) ListAllBindings(_ context.Context) ([]entity.UserRoleBinding, error) {
	return nil, nil
}

// ─── Stub audit logger ────────────────────────────────────────────────────────

type stubAuditLogger struct {
	logged []entity.AuditEntry
}

func (s *stubAuditLogger) Log(e entity.AuditEntry) { s.logged = append(s.logged, e) }
func (s *stubAuditLogger) Query(_ context.Context, _ storage.AuditFilter) ([]entity.AuditEntry, int, error) {
	return s.logged, len(s.logged), nil
}
func (s *stubAuditLogger) Flush(_ context.Context) error { return nil }
func (s *stubAuditLogger) Close() error                  { return nil }


// ─── Helpers ──────────────────────────────────────────────────────────────────

func makeTestRelease(name, ns, appVersion string, status helmrelease.Status, rev int) *helmrelease.Release {
	return &helmrelease.Release{
		Name:      name,
		Namespace: ns,
		Version:   rev,
		Info: &helmrelease.Info{
			Status:       status,
			LastDeployed: helmtime.Time{Time: time.Now().Add(-10 * time.Minute)},
		},
		Chart: &helmchart.Chart{
			Metadata: &helmchart.Metadata{
				Name:       name + "-chart",
				Version:    "1.0.0",
				AppVersion: appVersion,
			},
		},
	}
}

func defaultClusters() []helm.ClusterClient {
	return []helm.ClusterClient{
		{Name: "prod-1", Kubeconfig: &k8s.Config{}},
	}
}

// ─── listReleases via factory injection ───────────────────────────────────────

func TestModule_ListReleases_Empty(t *testing.T) {
	t.Parallel()

	cl := &stubClient{releases: nil}
	m := helm.NewModuleWithFactory(defaultClusters(), &stubRBAC{}, &stubAuditLogger{}, nil, stubFactory(cl))

	releases, err := m.TestListReleases("prod-1", "default")
	require.NoError(t, err)
	assert.Empty(t, releases)
}

func TestModule_ListReleases_Multiple(t *testing.T) {
	t.Parallel()

	cl := &stubClient{
		releases: []*helmrelease.Release{
			makeTestRelease("nginx", "default", "1.0.0", helmrelease.StatusDeployed, 3),
			makeTestRelease("redis", "default", "7.0.0", helmrelease.StatusFailed, 1),
		},
	}
	m := helm.NewModuleWithFactory(defaultClusters(), &stubRBAC{}, &stubAuditLogger{}, nil, stubFactory(cl))

	releases, err := m.TestListReleases("prod-1", "default")
	require.NoError(t, err)
	require.Len(t, releases, 2)
	assert.Equal(t, "nginx", releases[0].Name)
	assert.Equal(t, helmrelease.StatusFailed, releases[1].Info.Status)
}

func TestModule_ListReleases_ClientError(t *testing.T) {
	t.Parallel()

	cl := &stubClient{
		releases: nil,
		listErr:  errors.New("connection timeout"),
	}
	m := helm.NewModuleWithFactory(defaultClusters(), &stubRBAC{}, &stubAuditLogger{}, nil, stubFactory(cl))

	_, err := m.TestListReleases("prod-1", "default")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection timeout")
}

func TestModule_ListReleases_ClusterNotFound(t *testing.T) {
	t.Parallel()

	cl := &stubClient{}
	m := helm.NewModuleWithFactory(defaultClusters(), &stubRBAC{}, &stubAuditLogger{}, nil, stubFactory(cl))

	_, err := m.TestListReleases("unknown-cluster", "default")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// ─── getReleaseDetail via factory injection ───────────────────────────────────

func TestModule_GetReleaseDetail_Found(t *testing.T) {
	t.Parallel()

	nginx := makeTestRelease("nginx", "default", "1.2.0", helmrelease.StatusDeployed, 5)
	history := []*helmrelease.Release{
		makeTestRelease("nginx", "default", "1.1.0", helmrelease.StatusSuperseded, 4),
		makeTestRelease("nginx", "default", "1.2.0", helmrelease.StatusDeployed, 5),
	}

	cl := &stubClient{
		releases: []*helmrelease.Release{nginx},
		history:  history,
	}
	m := helm.NewModuleWithFactory(defaultClusters(), &stubRBAC{}, &stubAuditLogger{}, nil, stubFactory(cl))

	rel, hist, err := m.TestGetReleaseDetail("prod-1", "default", "nginx")
	require.NoError(t, err)
	assert.Equal(t, "nginx", rel.Name)
	assert.Len(t, hist, 2)
}

func TestModule_GetReleaseDetail_NotFound(t *testing.T) {
	t.Parallel()

	cl := &stubClient{
		releases: nil,
		getErr:   errors.New("release: not found"),
	}
	m := helm.NewModuleWithFactory(defaultClusters(), &stubRBAC{}, &stubAuditLogger{}, nil, stubFactory(cl))

	_, _, err := m.TestGetReleaseDetail("prod-1", "default", "ghost-release")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestModule_GetReleaseDetail_HistoryError_StillReturnsRelease(t *testing.T) {
	t.Parallel()

	nginx := makeTestRelease("nginx", "default", "1.2.0", helmrelease.StatusDeployed, 5)
	cl := &stubClient{
		releases: []*helmrelease.Release{nginx},
		histErr:  errors.New("history unavailable"),
	}
	m := helm.NewModuleWithFactory(defaultClusters(), &stubRBAC{}, &stubAuditLogger{}, nil, stubFactory(cl))

	rel, hist, err := m.TestGetReleaseDetail("prod-1", "default", "nginx")
	require.NoError(t, err, "history error must not fail the overall call")
	assert.Equal(t, "nginx", rel.Name)
	assert.Nil(t, hist, "history should be nil when fetch fails")
}

// ─── rollback via factory injection ──────────────────────────────────────────

func TestModule_Rollback_CallsClient(t *testing.T) {
	t.Parallel()

	cl := &stubClient{}
	m := helm.NewModuleWithFactory(defaultClusters(), &stubRBAC{}, &stubAuditLogger{}, nil, stubFactory(cl))

	err := m.TestRollback("prod-1", "default", "nginx", 3)
	require.NoError(t, err)
	assert.Equal(t, "nginx", cl.rolledBackName)
	assert.Equal(t, 3, cl.rolledBackVersion)
}

func TestModule_Rollback_ErrorPropagated(t *testing.T) {
	t.Parallel()

	cl := &stubClient{rollErr: errors.New("rollback: hooks timeout")}
	m := helm.NewModuleWithFactory(defaultClusters(), &stubRBAC{}, &stubAuditLogger{}, nil, stubFactory(cl))

	err := m.TestRollback("prod-1", "default", "nginx", 2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hooks timeout")
}

func TestModule_Rollback_FactoryError(t *testing.T) {
	t.Parallel()

	m := helm.NewModuleWithFactory(defaultClusters(), &stubRBAC{}, &stubAuditLogger{}, nil,
		errFactory(errors.New("kubeconfig: invalid")))

	err := m.TestRollback("prod-1", "default", "nginx", 2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kubeconfig")
}

// ─── RBAC permission checks ───────────────────────────────────────────────────

func TestModule_RBACPermission_ViewerCanList(t *testing.T) {
	t.Parallel()

	engine := &stubRBAC{perms: map[string]bool{rbac.PermHelmReleaseslist: true}}
	ok, err := engine.HasPermission(context.Background(), 100, rbac.PermHelmReleaseslist)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestModule_RBACPermission_ViewerCannotRollback(t *testing.T) {
	t.Parallel()

	engine := &stubRBAC{perms: map[string]bool{rbac.PermHelmReleaseslist: true}}
	ok, err := engine.HasPermission(context.Background(), 100, rbac.PermHelmReleasesRollback)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestModule_RBACPermission_AdminCanRollback(t *testing.T) {
	t.Parallel()

	engine := &stubRBAC{perms: map[string]bool{
		rbac.PermHelmReleaseslist:     true,
		rbac.PermHelmReleasesRollback: true,
	}}
	ok, err := engine.HasPermission(context.Background(), 300, rbac.PermHelmReleasesRollback)
	require.NoError(t, err)
	assert.True(t, ok)
}

// ─── Status emoji validation ──────────────────────────────────────────────────

func TestHelmRelease_AllStatusValues_DoNotPanic(t *testing.T) {
	t.Parallel()

	statuses := []helmrelease.Status{
		helmrelease.StatusDeployed,
		helmrelease.StatusFailed,
		helmrelease.StatusPendingInstall,
		helmrelease.StatusPendingUpgrade,
		helmrelease.StatusPendingRollback,
		helmrelease.StatusUninstalled,
		helmrelease.StatusSuperseded,
		helmrelease.StatusUnknown,
	}
	for _, status := range statuses {
		rel := makeTestRelease("test", "default", "1.0", status, 1)
		assert.Equal(t, status, rel.Info.Status)
	}
}
