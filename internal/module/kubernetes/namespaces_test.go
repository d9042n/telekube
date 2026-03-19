package kubernetes

import (
	"fmt"
	"testing"

	"github.com/d9042n/telekube/internal/cluster"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ─── handleNamespacesCommand tests ────────────────────────────────────────────

func TestHandleNamespacesCommand_PermissionDenied(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "viewer", entity.RoleViewer)
	m := buildTestModule(t, testutil.NewDenyAllRBAC())
	ctx := testutil.NewFakeTelebotContext(user, "/namespaces")

	err := m.handleNamespacesCommand(ctx)
	require.NoError(t, err)
	assert.Contains(t, ctx.LastMessage(), "don't have permission")
}

func TestHandleNamespacesCommand_ClientSetError(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	errMgr := testutil.NewFakeClusterManagerError(fmt.Errorf("cluster unavailable"))
	userCtx := cluster.NewUserContext(errMgr)
	userCtx.SetCluster(123, "test-cluster")

	m := &Module{
		cluster: errMgr,
		userCtx: userCtx,
		rbac:    testutil.NewAllowAllRBAC(),
		audit:   testutil.NewFakeAuditLogger(),
		logger:  zap.NewNop(),
		nsCache: newNamespaceCache(),
	}

	ctx := testutil.NewFakeTelebotContext(user, "/namespaces")
	err := m.handleNamespacesCommand(ctx)
	require.NoError(t, err)
	assert.Contains(t, ctx.LastMessage(), "Failed to connect to cluster")
}

func TestHandleNamespacesCommand_ListsNamespaces(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	ns1 := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	}
	ns2 := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "production"},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	}
	ns3 := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "old-ns"},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceTerminating},
	}

	m := buildTestModuleWithK8s(t, testutil.NewAllowAllRBAC(), ns1, ns2, ns3)
	ctx := testutil.NewFakeTelebotContext(user, "/namespaces")

	err := m.handleNamespacesCommand(ctx)
	require.NoError(t, err)

	msg := ctx.LastMessage()
	assert.Contains(t, msg, "Namespaces")
	assert.Contains(t, msg, "default")
	assert.Contains(t, msg, "production")
	assert.Contains(t, msg, "old-ns")
	assert.Contains(t, msg, "Terminating")
}

func TestHandleNamespacesCommand_EmptyCluster(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	m := buildTestModuleWithK8s(t, testutil.NewAllowAllRBAC())
	ctx := testutil.NewFakeTelebotContext(user, "/namespaces")

	err := m.handleNamespacesCommand(ctx)
	require.NoError(t, err)
	assert.Contains(t, ctx.LastMessage(), "No namespaces found")
}

func TestHandleNamespacesCommand_SingleNamespace(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	}
	m := buildTestModuleWithK8s(t, testutil.NewAllowAllRBAC(), ns)
	ctx := testutil.NewFakeTelebotContext(user, "/namespaces")

	err := m.handleNamespacesCommand(ctx)
	require.NoError(t, err)
	assert.Contains(t, ctx.LastMessage(), "default")
	assert.Contains(t, ctx.LastMessage(), "Active")
}
