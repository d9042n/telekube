package kubernetes

import (
	"fmt"
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/bot/keyboard"
	"github.com/d9042n/telekube/internal/cluster"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ─── handleCronJobsCommand tests ──────────────────────────────────────────────

func TestHandleCronJobsCommand_PermissionDenied(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "viewer", entity.RoleViewer)
	m := buildTestModule(t, testutil.NewDenyAllRBAC())
	ctx := testutil.NewFakeTelebotContext(user, "/cronjobs")

	err := m.handleCronJobsCommand(ctx)
	require.NoError(t, err)
	assert.Contains(t, ctx.LastMessage(), "don't have permission")
}

func TestHandleCronJobsCommand_ClientSetError(t *testing.T) {
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

	ctx := testutil.NewFakeTelebotContext(user, "/cronjobs production")
	err := m.handleCronJobsCommand(ctx)
	require.NoError(t, err)
	assert.Contains(t, ctx.LastMessage(), "Failed to connect to cluster")
}

func TestHandleCronJobsCommand_WithNamespaceArg(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	lastSchedule := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "daily-backup", Namespace: "production"},
		Spec: batchv1.CronJobSpec{
			Schedule: "0 2 * * *",
		},
		Status: batchv1.CronJobStatus{
			LastScheduleTime: &lastSchedule,
		},
	}
	m := buildTestModuleWithK8s(t, testutil.NewAllowAllRBAC(), cj)
	ctx := testutil.NewFakeTelebotContext(user, "/cronjobs production")

	err := m.handleCronJobsCommand(ctx)
	require.NoError(t, err)

	msg := ctx.LastMessage()
	assert.Contains(t, msg, "CronJobs in production")
	assert.Contains(t, msg, "daily-backup")
	assert.Contains(t, msg, "0 2 * * *")
}

func TestHandleCronJobsCommand_EmptyNamespace(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	m := buildTestModuleWithK8s(t, testutil.NewAllowAllRBAC())
	ctx := testutil.NewFakeTelebotContext(user, "/cronjobs production")

	err := m.handleCronJobsCommand(ctx)
	require.NoError(t, err)
	assert.Contains(t, ctx.LastMessage(), "No CronJobs found")
}

func TestHandleCronJobsCommand_SuspendedCronJob(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	suspended := true
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "paused-job", Namespace: "default"},
		Spec: batchv1.CronJobSpec{
			Schedule: "*/5 * * * *",
			Suspend:  &suspended,
		},
	}
	m := buildTestModuleWithK8s(t, testutil.NewAllowAllRBAC(), cj)
	ctx := testutil.NewFakeTelebotContext(user, "/cronjobs default")

	err := m.handleCronJobsCommand(ctx)
	require.NoError(t, err)

	msg := ctx.LastMessage()
	assert.Contains(t, msg, "paused-job")
	assert.Contains(t, msg, "Suspended")
}

func TestHandleCronJobsCommand_ActiveCronJob(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "frequent-job", Namespace: "default"},
		Spec: batchv1.CronJobSpec{
			Schedule: "* * * * *",
		},
		Status: batchv1.CronJobStatus{
			Active: []corev1.ObjectReference{{Name: "frequent-job-12345"}},
		},
	}
	m := buildTestModuleWithK8s(t, testutil.NewAllowAllRBAC(), cj)
	ctx := testutil.NewFakeTelebotContext(user, "/cronjobs default")

	err := m.handleCronJobsCommand(ctx)
	require.NoError(t, err)

	msg := ctx.LastMessage()
	assert.Contains(t, msg, "frequent-job")
	assert.Contains(t, msg, "Active Jobs: 1")
}

func TestHandleCronJobsCommand_NoNamespaceArg_ShowsSelector(t *testing.T) {
	t.Parallel()

	user := testutil.NewTestUser(123, "admin", entity.RoleAdmin)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	}
	m := buildTestModuleWithK8s(t, testutil.NewAllowAllRBAC(), ns)
	ctx := testutil.NewFakeTelebotContext(user, "/cronjobs")

	err := m.handleCronJobsCommand(ctx)
	require.NoError(t, err)
	assert.Contains(t, ctx.LastMessage(), "Select namespace")
}
