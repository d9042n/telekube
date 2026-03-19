//go:build e2e

package e2e_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	e2e "github.com/d9042n/telekube/test/e2e"
)

const cronjobsAdminID = int64(999999)

func TestE2E_CronJobs_Smoke(t *testing.T) {
	h := newSmokeHarness(t, cronjobsAdminID)
	h.SeedUser(cronjobsAdminID, "testadmin", "admin")

	h.SendMessage(cronjobsAdminID, "testadmin", "/cronjobs")
	msg, ok := h.WaitForMessageTo(cronjobsAdminID, 5*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must reply to /cronjobs")
	assert.NotContains(t, strings.ToLower(msg), "unknown command")
}

func TestE2E_CronJobs_WithK3s(t *testing.T) {
	if e2e.IsSkipCluster() {
		t.Skip("E2E_SKIP_CLUSTER=true")
	}

	h := e2e.NewHarness(t, e2e.WithAdminIDs(cronjobsAdminID))
	ns := e2e.UniqueTestNamespace(t)
	h.K8s.CreateNamespace(t, ns)
	t.Cleanup(func() { h.K8s.DeleteNamespace(t, ns) })

	cs := h.K8s.ClientSet()
	_, err := cs.BatchV1().CronJobs(ns).Create(context.Background(), &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test-backup", Namespace: ns},
		Spec: batchv1.CronJobSpec{
			Schedule: "0 2 * * *",
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers:    []corev1.Container{{Name: "bb", Image: "busybox:1.36", Command: []string{"echo", "backup"}}},
							RestartPolicy: corev1.RestartPolicyNever,
						},
					},
				},
			},
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	time.Sleep(2 * time.Second)

	h.SeedUser(cronjobsAdminID, "testadmin", "admin")
	h.SendMessage(cronjobsAdminID, "testadmin", "/cronjobs "+ns)

	msg, ok := h.WaitForMessageTo(cronjobsAdminID, 10*time.Second, func(s string) bool {
		return strings.Contains(s, "test-backup")
	})
	require.True(t, ok, "bot must list the cronjob")
	assert.Contains(t, msg, "test-backup")
	assert.Contains(t, msg, "0 2 * * *")
}
