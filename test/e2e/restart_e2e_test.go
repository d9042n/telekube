//go:build e2e

package e2e_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	e2e "github.com/d9042n/telekube/test/e2e"
)

const (
	restartAdminID  = int64(999999)
	restartViewerID = int64(600001)
)

func TestE2E_Restart_Smoke(t *testing.T) {
	h := newSmokeHarness(t, restartAdminID)
	h.SeedUser(restartAdminID, "testadmin", "admin")

	h.SendMessage(restartAdminID, "testadmin", "/restart")
	msg, ok := h.WaitForMessageTo(restartAdminID, 5*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must reply to /restart")
	assert.Contains(t, msg, "Usage:")
}

func TestE2E_Restart_PermissionDenied(t *testing.T) {
	h := newSmokeHarness(t, restartAdminID)
	h.SeedUser(restartViewerID, "viewer1", "viewer")

	h.SendMessage(restartViewerID, "viewer1", "/restart some-pod")
	msg, ok := h.WaitForMessageTo(restartViewerID, 5*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must reply to /restart permission denied")
	assert.Contains(t, msg, "don't have permission")
}

func TestE2E_Restart_CommandNotUnknown(t *testing.T) {
	h := newSmokeHarness(t, restartAdminID)
	h.SeedUser(restartAdminID, "testadmin", "admin")

	h.SendMessage(restartAdminID, "testadmin", "/restart")
	msg, ok := h.WaitForMessageTo(restartAdminID, 5*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must reply to /restart")
	assert.NotContains(t, strings.ToLower(msg), "unknown command")
}

func TestE2E_Restart_WithK3s_StandalonePod(t *testing.T) {
	if e2e.IsSkipCluster() {
		t.Skip("E2E_SKIP_CLUSTER=true")
	}

	h := e2e.NewHarness(t, e2e.WithAdminIDs(restartAdminID))
	ns := e2e.UniqueTestNamespace(t)
	h.K8s.CreateNamespace(t, ns)
	t.Cleanup(func() { h.K8s.DeleteNamespace(t, ns) })

	cs := h.K8s.ClientSet()
	_, err := cs.CoreV1().Pods(ns).Create(context.Background(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "standalone-pod", Namespace: ns},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "bb", Image: "busybox:1.36", Command: []string{"sleep", "3600"}}}},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	time.Sleep(2 * time.Second)

	h.SeedUser(restartAdminID, "testadmin", "admin")
	h.SendMessage(restartAdminID, "testadmin", "/restart standalone-pod "+ns)

	msg, ok := h.WaitForMessageTo(restartAdminID, 10*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must reply to /restart standalone")
	assert.Contains(t, msg, "Cannot restart standalone")
}

func TestE2E_Restart_WithK3s_ManagedPod(t *testing.T) {
	if e2e.IsSkipCluster() {
		t.Skip("E2E_SKIP_CLUSTER=true")
	}

	h := e2e.NewHarness(t, e2e.WithAdminIDs(restartAdminID))
	ns := e2e.UniqueTestNamespace(t)
	h.K8s.CreateNamespace(t, ns)
	t.Cleanup(func() { h.K8s.DeleteNamespace(t, ns) })

	cs := h.K8s.ClientSet()
	replicas := int32(1)
	_, err := cs.AppsV1().Deployments(ns).Create(context.Background(), &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "restart-test", Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "restart-test"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "restart-test"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "bb", Image: "busybox:1.36", Command: []string{"sleep", "3600"}}}},
			},
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	var podName string
	require.Eventually(t, func() bool {
		pods, _ := cs.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{LabelSelector: "app=restart-test"})
		if len(pods.Items) > 0 {
			podName = pods.Items[0].Name
			return true
		}
		return false
	}, 30*time.Second, 1*time.Second, "deployment pod must appear")

	h.SeedUser(restartAdminID, "testadmin", "admin")
	h.SendMessage(restartAdminID, "testadmin", "/restart "+podName+" "+ns)

	msg, ok := h.WaitForMessageTo(restartAdminID, 10*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must reply with restart confirmation")
	assert.Contains(t, msg, "Confirm restart")
}
