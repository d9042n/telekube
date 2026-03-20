package kubernetes

import (
	"fmt"
	"testing"

	"github.com/d9042n/telekube/internal/bot/keyboard"
	"github.com/d9042n/telekube/internal/cluster"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/rbac"
	"github.com/d9042n/telekube/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ─── handleRestartCommand tests ───────────────────────────────────────────────

func TestHandleRestartCommand_PermissionDenied(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "viewer", entity.RoleViewer)
	m := buildTestModule(t, testutil.NewDenyAllRBAC())
	ctx := testutil.NewFakeTelebotContext(user, "/restart nginx")

	err := m.handleRestartCommand(ctx)
	require.NoError(t, err)
	assert.Contains(t, ctx.LastMessage(), "don't have permission")
}

func TestHandleRestartCommand_MissingArgs(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	m := buildTestModule(t, testutil.NewAllowAllRBAC())
	ctx := testutil.NewFakeTelebotContext(user, "/restart")

	err := m.handleRestartCommand(ctx)
	require.NoError(t, err)
	assert.Contains(t, ctx.LastMessage(), "Usage:")
}

func TestHandleRestartCommand_ClientSetError(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	errMgr := testutil.NewFakeClusterManagerError(fmt.Errorf("connection refused"))
	userCtx := cluster.NewUserContext(errMgr)
	userCtx.SetCluster(123, "test-cluster")

	m := &Module{
		cluster: errMgr,
		userCtx: userCtx,
		rbac:    testutil.NewAllowAllRBAC(),
		audit:   testutil.NewFakeAuditLogger(),
		kb:      keyboard.NewBuilder(),
		logger:  zap.NewNop(),
		nsCache: newNamespaceCache(),
	}

	ctx := testutil.NewFakeTelebotContext(user, "/restart nginx")
	err := m.handleRestartCommand(ctx)
	require.NoError(t, err)
	// ClientSet returns error — handler surfaces error to user
	assert.Contains(t, ctx.LastMessage(), "Failed to connect to cluster")
}

func TestHandleRestartCommand_PodNotFound(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	m := buildTestModuleWithK8s(t, testutil.NewAllowAllRBAC())
	ctx := testutil.NewFakeTelebotContext(user, "/restart nonexistent default")

	err := m.handleRestartCommand(ctx)
	require.NoError(t, err)
	assert.Contains(t, ctx.LastMessage(), "not found")
}

func TestHandleRestartCommand_StandalonePod(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "standalone", Namespace: "default"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	m := buildTestModuleWithK8s(t, testutil.NewAllowAllRBAC(), pod)
	ctx := testutil.NewFakeTelebotContext(user, "/restart standalone default")

	err := m.handleRestartCommand(ctx)
	require.NoError(t, err)
	assert.Contains(t, ctx.LastMessage(), "Cannot restart standalone")
}

func TestHandleRestartCommand_ShowsConfirmation(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-abc123",
			Namespace: "production",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "nginx-rs", APIVersion: "apps/v1"},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	m := buildTestModuleWithK8s(t, testutil.NewAllowAllRBAC(), pod)
	ctx := testutil.NewFakeTelebotContext(user, "/restart nginx-abc123 production")

	err := m.handleRestartCommand(ctx)
	require.NoError(t, err)
	msg := ctx.LastMessage()
	assert.Contains(t, msg, "Confirm restart")
	assert.Contains(t, msg, "nginx-abc123")
	assert.Contains(t, msg, "production")
}

func TestHandleRestartCommand_DefaultNamespace(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "worker",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "worker-rs", APIVersion: "apps/v1"},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	m := buildTestModuleWithK8s(t, testutil.NewAllowAllRBAC(), pod)
	ctx := testutil.NewFakeTelebotContext(user, "/restart worker")

	err := m.handleRestartCommand(ctx)
	require.NoError(t, err)
	assert.Contains(t, ctx.LastMessage(), "Confirm restart")
}

func TestHandleRestartCommand_AuditDenied(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "viewer", entity.RoleViewer)
	auditLog := testutil.NewFakeAuditLogger()
	clusterMgr := testutil.NewFakeClusterManager()
	userCtx := cluster.NewUserContext(clusterMgr)
	userCtx.SetCluster(user.TelegramID, "test-cluster")

	m := &Module{
		cluster: clusterMgr,
		userCtx: userCtx,
		rbac:    testutil.NewDenyAllRBAC(),
		audit:   auditLog,
		kb:      keyboard.NewBuilder(),
		logger:  zap.NewNop(),
		nsCache: newNamespaceCache(),
	}

	ctx := testutil.NewFakeTelebotContext(user, "/restart nginx")
	_ = m.handleRestartCommand(ctx)

	entries := auditLog.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, entity.AuditStatusDenied, entries[0].Status)
	assert.Equal(t, "pod.restart", entries[0].Action)
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

// buildTestModule creates a Module with the given RBAC engine and a default
// cluster selected for user 123.
func buildTestModule(t *testing.T, rbacEngine rbac.Engine) *Module {
	t.Helper()
	clusterMgr := testutil.NewFakeClusterManager()
	userCtx := cluster.NewUserContext(clusterMgr)
	userCtx.SetCluster(123, "test-cluster")

	return &Module{
		cluster: clusterMgr,
		userCtx: userCtx,
		rbac:    rbacEngine,
		audit:   testutil.NewFakeAuditLogger(),
		kb:      keyboard.NewBuilder(),
		logger:  zap.NewNop(),
		nsCache: newNamespaceCache(),
	}
}

// buildTestModuleWithK8s creates a Module with a fake K8s client pre-seeded
// with the given objects, and user 123 pointed at "test-cluster".
func buildTestModuleWithK8s(t *testing.T, rbacEngine rbac.Engine, objects ...runtime.Object) *Module {
	t.Helper()
	clusterMgr := testutil.NewFakeClusterManager(objects...)
	userCtx := cluster.NewUserContext(clusterMgr)
	userCtx.SetCluster(123, "test-cluster")

	return &Module{
		cluster: clusterMgr,
		userCtx: userCtx,
		rbac:    rbacEngine,
		audit:   testutil.NewFakeAuditLogger(),
		kb:      keyboard.NewBuilder(),
		logger:  zap.NewNop(),
		nsCache: newNamespaceCache(),
	}
}
