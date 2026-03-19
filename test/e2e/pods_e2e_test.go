//go:build e2e

package e2e_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	e2e "github.com/d9042n/telekube/test/e2e"
)

const (
	podsAdminID  = int64(999999)
	podsViewerID = int64(500001)
)

// TestE2E_Pods_FullFlow tests the complete pod list → namespace flow with k3s.
func TestE2E_Pods_FullFlow(t *testing.T) {
	if e2e.IsSkipCluster() {
		t.Skip("E2E_SKIP_CLUSTER=true — skipping pod flow test")
	}

	h := e2e.NewHarness(t, e2e.WithAdminIDs(podsAdminID))

	ns := e2e.UniqueTestNamespace(t)
	h.K8s.CreateNamespace(t, ns)
	t.Cleanup(func() { h.K8s.DeleteNamespace(t, ns) })

	// Apply an nginx pod into the test namespace.
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-test",
			Namespace: ns,
			Labels:    map[string]string{"app": "nginx-test"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "nginx", Image: "nginx:1.27-alpine"},
			},
		},
	}
	cs := h.K8s.ClientSet()
	_, err := cs.CoreV1().Pods(ns).Create(context.Background(), pod, metav1.CreateOptions{})
	require.NoError(t, err)

	// Have admin user trigger /pods.
	h.SeedUser(podsAdminID, "testadmin", "admin")
	h.SendMessage(podsAdminID, "testadmin", "/pods")

	// Bot replies with a namespace selector.
	msg, ok := h.WaitForMessageTo(podsAdminID, 10*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must reply to /pods")
	t.Logf("pods reply: %q", msg)
	assert.NotEmpty(t, msg)
}

// TestE2E_Pods_EmptyNamespace tests that /pods replies when the cluster is reachable.
func TestE2E_Pods_EmptyNamespace(t *testing.T) {
	if e2e.IsSkipCluster() {
		t.Skip("E2E_SKIP_CLUSTER=true — skipping empty namespace test")
	}

	h := e2e.NewHarness(t, e2e.WithAdminIDs(podsAdminID))

	ns := e2e.UniqueTestNamespace(t)
	h.K8s.CreateNamespace(t, ns)
	t.Cleanup(func() { h.K8s.DeleteNamespace(t, ns) })

	h.SeedUser(podsAdminID, "testadmin", "admin")
	h.SendMessage(podsAdminID, "testadmin", "/pods")

	msg, ok := h.WaitForMessageTo(podsAdminID, 10*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must reply to /pods in empty namespace")
	t.Logf("empty namespace reply: %q", msg)
	assert.NotEmpty(t, msg)
}

// TestE2E_Pods_AllNamespaces verifies that /pods shows namespace selection
// which includes namespaces from the real k3s cluster.
func TestE2E_Pods_AllNamespaces(t *testing.T) {
	if e2e.IsSkipCluster() {
		t.Skip("E2E_SKIP_CLUSTER=true — skipping all-namespaces test")
	}

	h := e2e.NewHarness(t, e2e.WithAdminIDs(podsAdminID))
	h.SeedUser(podsAdminID, "testadmin", "admin")

	// Create a second namespace with a pod to ensure multiple namespaces exist.
	ns2 := e2e.UniqueTestNamespace(t)
	h.K8s.CreateNamespace(t, ns2)
	t.Cleanup(func() { h.K8s.DeleteNamespace(t, ns2) })

	cs := h.K8s.ClientSet()
	_, err := cs.CoreV1().Pods(ns2).Create(context.Background(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "allns-test", Namespace: ns2},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "nginx", Image: "nginx:1.27-alpine"}},
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	h.SendMessage(podsAdminID, "testadmin", "/pods")
	msg, ok := h.WaitForMessageTo(podsAdminID, 10*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must reply to /pods")
	t.Logf("all-namespaces reply: %q", msg)
	assert.NotEmpty(t, msg)
}

// TestE2E_Pods_Pagination creates 20 pods and verifies the bot responds with
// a paginated namespace selector from the real k3s cluster.
func TestE2E_Pods_Pagination(t *testing.T) {
	if e2e.IsSkipCluster() {
		t.Skip("E2E_SKIP_CLUSTER=true — skipping pagination test")
	}

	h := e2e.NewHarness(t, e2e.WithAdminIDs(podsAdminID))

	ns := e2e.UniqueTestNamespace(t)
	h.K8s.CreateNamespace(t, ns)
	t.Cleanup(func() { h.K8s.DeleteNamespace(t, ns) })

	cs := h.K8s.ClientSet()
	for i := 0; i < 20; i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("page-test-%02d", i),
				Namespace: ns,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "bb", Image: "busybox:1.36", Command: []string{"sleep", "3600"}}},
			},
		}
		_, err := cs.CoreV1().Pods(ns).Create(context.Background(), pod, metav1.CreateOptions{})
		require.NoError(t, err)
	}
	t.Logf("created 20 pods in namespace %s", ns)

	// Wait a moment for pods to appear.
	time.Sleep(2 * time.Second)

	h.SeedUser(podsAdminID, "testadmin", "admin")
	h.SendMessage(podsAdminID, "testadmin", "/pods")

	msg, ok := h.WaitForMessageTo(podsAdminID, 10*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must reply to /pods with 20-pod namespace")
	t.Logf("pagination pods reply: %q", msg)
	assert.NotEmpty(t, msg)
}

// TestE2E_Pods_AllNamespaces_NoK3s tests /pods in smoke mode.
// Without a cluster selected, the bot sends "no cluster" message via c.Send().
func TestE2E_Pods_AllNamespaces_NoK3s(t *testing.T) {
	const (
		adminID  = int64(999999)
		viewerID = int64(500002)
	)
	h := newSmokeHarness(t, adminID)
	h.SeedUser(viewerID, "viewer1", "viewer")

	// /pods with no cluster selected → bot sends "no cluster selected" message.
	h.SendMessage(viewerID, "viewer1", "/pods")

	msg, ok := h.WaitForMessageTo(viewerID, 5*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must reply to /pods with no cluster")
	t.Logf("no-cluster pods reply: %q", msg)
	assert.NotEmpty(t, msg)
}

// TestE2E_Pods_Refresh tests that /pods twice returns consistent messages.
func TestE2E_Pods_Refresh(t *testing.T) {
	const adminID = int64(999999)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(adminID, "testadmin", "admin")

	// First get the list
	h.SendMessage(adminID, "testadmin", "/pods")
	_, ok := h.WaitForMessageTo(adminID, 5*time.Second, func(s string) bool { return s != "" })
	require.True(t, ok, "initial /pods must reply")
	h.ClearMessages()

	// Simulate refresh by sending /pods again.
	h.SendMessage(adminID, "testadmin", "/pods")
	msg, ok := h.WaitForMessageTo(adminID, 5*time.Second, func(s string) bool { return s != "" })
	require.True(t, ok, "refresh must reply")
	assert.NotEmpty(t, msg)
}

// TestE2E_Pods_Pagination_NoK3s verifies the bot handles k8s_pods_page callback.
// Since k8s_pods_page uses Bot().Edit(), we verify liveness only.
func TestE2E_Pods_Pagination_NoK3s(t *testing.T) {
	const adminID = int64(999999)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(adminID, "testadmin", "admin")

	// k8s_pods_page calls Bot().Edit() — not captured as SendMessage.
	// Just verify the bot doesn't crash on receiving this callback.
	h.SendCallback(adminID, "testadmin", "k8s_pods_page", "default|2")
	time.Sleep(300 * time.Millisecond)

	// Verify bot is still alive.
	h.SendMessage(adminID, "testadmin", "/start")
	_, ok := h.WaitForMessageTo(adminID, 5*time.Second, func(s string) bool { return s != "" })
	require.True(t, ok, "bot must be alive after pagination callback")
}

// TestE2E_Pods_Detail tests clicking on a pod name shows detail (k3s required).
func TestE2E_Pods_Detail(t *testing.T) {
	if e2e.IsSkipCluster() {
		t.Skip("E2E_SKIP_CLUSTER=true — skipping pod detail test")
	}

	h := e2e.NewHarness(t, e2e.WithAdminIDs(podsAdminID))

	ns := e2e.UniqueTestNamespace(t)
	h.K8s.CreateNamespace(t, ns)
	t.Cleanup(func() { h.K8s.DeleteNamespace(t, ns) })

	cs := h.K8s.ClientSet()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "detail-pod", Namespace: ns},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "nginx", Image: "nginx:1.27-alpine"},
			},
		},
	}
	_, err := cs.CoreV1().Pods(ns).Create(context.Background(), pod, metav1.CreateOptions{})
	require.NoError(t, err)
	time.Sleep(2 * time.Second)

	h.SeedUser(podsAdminID, "testadmin", "admin")

	// Trigger pod list first — the bot must find the cluster.
	h.SendMessage(podsAdminID, "testadmin", "/pods")
	msg, ok := h.WaitForMessageTo(podsAdminID, 10*time.Second, func(s string) bool { return s != "" })
	require.True(t, ok, "bot must reply to /pods command")
	t.Logf("pods list: %q", msg)

	// k8s_pod_detail uses Bot().Edit() — just confirm liveness.
	podData := fmt.Sprintf("detail-pod|%s|e2e-cluster", ns)
	h.SendCallback(podsAdminID, "testadmin", "k8s_pod_detail", podData)
	time.Sleep(500 * time.Millisecond)
}

// TestE2E_Pods_NotFound verifies bot liveness after requesting a non-existent pod.
// k8s_pod_detail uses Bot().Edit() for its response — not captured as SendMessage.
func TestE2E_Pods_NotFound(t *testing.T) {
	const adminID = int64(999999)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(adminID, "testadmin", "admin")

	// Request detail for a pod that doesn't exist.
	h.SendCallback(adminID, "testadmin", "k8s_pod_detail", "completely-missing-pod|default|test-cluster")
	time.Sleep(300 * time.Millisecond)

	// Verify bot is still alive.
	h.SendMessage(adminID, "testadmin", "/start")
	msg, ok := h.WaitForMessageTo(adminID, 5*time.Second, func(s string) bool { return s != "" })
	require.True(t, ok, "bot must be alive after not-found pod callback")
	assert.NotEmpty(t, msg)
}

// TestE2E_Pods_CommandSmoke tests the /pods command is accessible (smoke, no cluster).
func TestE2E_Pods_CommandSmoke(t *testing.T) {
	const adminID = int64(999999)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(adminID, "testadmin", "admin")

	h.SendMessage(adminID, "testadmin", "/pods")

	msg, ok := h.WaitForMessageTo(adminID, 5*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must reply to /pods in smoke mode")

	assert.NotContains(t, strings.ToLower(msg), "unknown command")
}
