package kubernetes

import (
	"fmt"
	"testing"

	"github.com/d9042n/telekube/internal/bot/keyboard"
	"github.com/d9042n/telekube/internal/cluster"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ─── handleDeploysCommand tests ───────────────────────────────────────────────

func TestHandleDeploysCommand_PermissionDenied(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "viewer", entity.RoleViewer)
	m := buildTestModule(t, testutil.NewDenyAllRBAC())
	ctx := testutil.NewFakeTelebotContext(user, "/deploys")

	err := m.handleDeploysCommand(ctx)
	require.NoError(t, err)
	assert.Contains(t, ctx.LastMessage(), "don't have permission")
}

func TestHandleDeploysCommand_ClientSetError(t *testing.T) {
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
		kb:      keyboard.NewBuilder(),
		logger:  zap.NewNop(),
		nsCache: newNamespaceCache(),
	}

	ctx := testutil.NewFakeTelebotContext(user, "/deploys production")
	err := m.handleDeploysCommand(ctx)
	require.NoError(t, err)
	assert.Contains(t, ctx.LastMessage(), "Failed to connect to cluster")
}

func TestHandleDeploysCommand_WithNamespaceArg(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	replicas := int32(3)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api-server", Namespace: "production"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:     3,
			AvailableReplicas: 3,
		},
	}
	m := buildTestModuleWithK8s(t, testutil.NewAllowAllRBAC(), deploy)
	ctx := testutil.NewFakeTelebotContext(user, "/deploys production")

	err := m.handleDeploysCommand(ctx)
	require.NoError(t, err)

	msg := ctx.LastMessage()
	assert.Contains(t, msg, "api-server")
	assert.Contains(t, msg, "3/3")
}

func TestHandleDeploysCommand_EmptyNamespace(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	m := buildTestModuleWithK8s(t, testutil.NewAllowAllRBAC())
	ctx := testutil.NewFakeTelebotContext(user, "/deploys production")

	err := m.handleDeploysCommand(ctx)
	require.NoError(t, err)
	assert.Contains(t, ctx.LastMessage(), "No deployments found")
}

func TestHandleDeploysCommand_DegradedStatus(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	replicas := int32(3)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "broken-app", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:     0,
			AvailableReplicas: 0,
		},
	}
	m := buildTestModuleWithK8s(t, testutil.NewAllowAllRBAC(), deploy)
	ctx := testutil.NewFakeTelebotContext(user, "/deploys default")

	err := m.handleDeploysCommand(ctx)
	require.NoError(t, err)

	msg := ctx.LastMessage()
	assert.Contains(t, msg, "broken-app")
	assert.Contains(t, msg, "Degraded")
}

func TestHandleDeploysCommand_ProgressingStatus(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	replicas := int32(5)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "rolling", Namespace: "staging"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:     3,
			AvailableReplicas: 3,
		},
	}
	m := buildTestModuleWithK8s(t, testutil.NewAllowAllRBAC(), deploy)
	ctx := testutil.NewFakeTelebotContext(user, "/deploys staging")

	err := m.handleDeploysCommand(ctx)
	require.NoError(t, err)

	msg := ctx.LastMessage()
	assert.Contains(t, msg, "rolling")
	assert.Contains(t, msg, "Progressing")
	assert.Contains(t, msg, "3/5")
}

func TestHandleDeploysCommand_NoNamespaceArg_ShowsSelector(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	}
	m := buildTestModuleWithK8s(t, testutil.NewAllowAllRBAC(), ns)
	ctx := testutil.NewFakeTelebotContext(user, "/deploys")

	err := m.handleDeploysCommand(ctx)
	require.NoError(t, err)
	assert.Contains(t, ctx.LastMessage(), "Select namespace")
}
