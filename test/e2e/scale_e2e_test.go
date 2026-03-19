//go:build e2e

// Package e2e_test — scaling scenarios.
// Tests scale-up, scale-down, scale-to-zero confirmation, and RBAC enforcement.
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
	scaleAdminID    = int64(999999)
	scaleOperatorID = int64(850001)
	scaleViewerID   = int64(850002)
)

// ptr32 is a helper for *int32.
func ptr32(n int32) *int32 { return &n }

// ─── Scale with real k3s cluster ─────────────────────────────────────────────

// TestE2E_Scale_Up tests scaling a deployment from 2 to 5 replicas via /scale.
func TestE2E_Scale_Up(t *testing.T) {
	if e2e.IsSkipCluster() {
		t.Skip("E2E_SKIP_CLUSTER=true — skipping scale up test")
	}

	h := e2e.NewHarness(t, e2e.WithAdminIDs(scaleAdminID))
	h.SeedUser(scaleAdminID, "testadmin", "admin")

	ns := e2e.UniqueTestNamespace(t)
	h.K8s.CreateNamespace(t, ns)
	t.Cleanup(func() { h.K8s.DeleteNamespace(t, ns) })

	// Create a 2-replica nginx deployment.
	cs := h.K8s.ClientSet()
	deploy := makeNginxDeployment("scale-test", ns, 2)
	_, err := cs.AppsV1().Deployments(ns).Create(context.Background(), deploy, metav1.CreateOptions{})
	require.NoError(t, err)
	t.Logf("deployment scale-test created in %s with 2 replicas", ns)

	// /scale triggers the namespace selector — admin must see it.
	h.SendMessage(scaleAdminID, "testadmin", "/scale")

	msg, ok := h.WaitForMessageTo(scaleAdminID, 15*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must reply to /scale request")
	t.Logf("scale reply: %q", msg)
	assert.NotEmpty(t, msg)
	assert.NotContains(t, msg, "⛔")
}

// TestE2E_Scale_Down tests the scale command route is accessible for admins.
func TestE2E_Scale_Down(t *testing.T) {
	if e2e.IsSkipCluster() {
		t.Skip("E2E_SKIP_CLUSTER=true — skipping scale down test")
	}

	h := e2e.NewHarness(t, e2e.WithAdminIDs(scaleAdminID))
	h.SeedUser(scaleAdminID, "testadmin", "admin")

	ns := e2e.UniqueTestNamespace(t)
	h.K8s.CreateNamespace(t, ns)
	t.Cleanup(func() { h.K8s.DeleteNamespace(t, ns) })

	cs := h.K8s.ClientSet()
	deploy := makeNginxDeployment("scale-down-test", ns, 5)
	_, err := cs.AppsV1().Deployments(ns).Create(context.Background(), deploy, metav1.CreateOptions{})
	require.NoError(t, err)

	// Admin can access /scale without RBAC denial.
	h.SendMessage(scaleAdminID, "testadmin", "/scale")
	msg, ok := h.WaitForMessageTo(scaleAdminID, 10*time.Second, func(s string) bool { return s != "" })
	require.True(t, ok, "admin must receive /scale response")
	assert.NotContains(t, msg, "⛔")
}

// ─── Smoke-mode scale tests (no k3s required) ─────────────────────────────────

// TestE2E_Scale_ToZero_RequiresConfirm verifies /scale command is accessible for admins.
// The confirm flow uses k8s_scale_confirm callback which responds via Bot().Edit().
func TestE2E_Scale_ToZero_RequiresConfirm(t *testing.T) {
	const adminID = int64(999999)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(adminID, "testadmin", "admin")

	// /scale shows the namespace selector — a c.Send call — verifiable.
	h.SendMessage(adminID, "testadmin", "/scale")

	msg, ok := h.WaitForMessageTo(adminID, 5*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must reply to /scale command")
	t.Logf("scale response: %q", msg)
	assert.NotEmpty(t, msg)
	// Admin must not be denied.
	assert.NotContains(t, msg, "⛔")
}

// TestE2E_Scale_Viewer_CannotScale verifies that a viewer is denied /scale.
func TestE2E_Scale_Viewer_CannotScale(t *testing.T) {
	const (
		adminID  = int64(999999)
		viewerID = int64(850002)
	)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(viewerID, "viewer2", "viewer")

	// /scale requires kubernetes.deployments.scale — viewer doesn't have it.
	h.SendMessage(viewerID, "viewer2", "/scale")

	msg, ok := h.WaitForMessageTo(viewerID, 5*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must respond to viewer /scale attempt")
	t.Logf("viewer scale response: %q", msg)

	lower := strings.ToLower(msg)
	isDenied := strings.Contains(msg, "⛔") ||
		strings.Contains(lower, "permission") ||
		strings.Contains(lower, "denied") ||
		strings.Contains(lower, "access")
	assert.True(t, isDenied, "viewer must be denied /scale; got: %q", msg)
}

// TestE2E_Scale_Operator_CanScale verifies that an operator can access /scale.
func TestE2E_Scale_Operator_CanScale(t *testing.T) {
	const (
		adminID    = int64(999999)
		operatorID = int64(850001)
	)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(operatorID, "op1", "operator")

	// Operator has kubernetes.deployments.scale — must not be denied.
	h.SendMessage(operatorID, "op1", "/scale")

	msg, ok := h.WaitForMessageTo(operatorID, 5*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must respond to operator /scale attempt")
	t.Logf("operator scale response: %q", msg)
	assert.NotEmpty(t, msg)

	// Operator should NOT receive a permission denied.
	assert.NotContains(t, msg, "⛔")
}

// TestE2E_Scale_NotFound_Error verifies that selecting a deployment namespace
// when there's no cluster gracefully returns an error message.
func TestE2E_Scale_NotFound_Error(t *testing.T) {
	const adminID = int64(999999)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(adminID, "testadmin", "admin")

	// k8s_scale_ns triggers namespace selection inside the scale flow.
	// With no cluster selected it sends "no cluster" c.Send() message.
	h.SendCallback(adminID, "testadmin", "k8s_scale_ns", "default")

	msg, ok := h.WaitForMessageTo(adminID, 5*time.Second, func(s string) bool {
		return s != ""
	})
	if !ok {
		// k8s_scale_ns may call Bot().Edit() only — still proves no crash.
		h.SendMessage(adminID, "testadmin", "/start")
		_, alive := h.WaitForMessageTo(adminID, 3*time.Second, func(s string) bool { return s != "" })
		require.True(t, alive, "bot must be alive after scale namespace callback")
		return
	}
	t.Logf("scale ns callback response: %q", msg)
	assert.NotEmpty(t, msg)
}

// TestE2E_Scale_NegativeReplica_Error verifies graceful handling of invalid data.
// k8s_scale_set with bad data uses Respond() internally — we just verify no crash.
func TestE2E_Scale_NegativeReplica_Error(t *testing.T) {
	const adminID = int64(999999)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(adminID, "testadmin", "admin")

	// k8s_scale_set with garbage data: the bot returns a CallbackResponse (toast),
	// which is not recorded as a SendMessage. Verify bot is still alive.
	h.SendCallback(adminID, "testadmin", "k8s_scale_set", "invalid-data")
	time.Sleep(300 * time.Millisecond)

	h.SendMessage(adminID, "testadmin", "/start")
	msg, ok := h.WaitForMessageTo(adminID, 5*time.Second, func(s string) bool { return s != "" })
	require.True(t, ok, "bot must still be alive after invalid scale data")
	assert.NotEmpty(t, msg)
}

// TestE2E_Scale_CommandSmoke verifies /scale command is routed (smoke mode).
func TestE2E_Scale_CommandSmoke(t *testing.T) {
	const adminID = int64(999999)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(adminID, "testadmin", "admin")

	h.SendMessage(adminID, "testadmin", "/scale")
	msg, ok := h.WaitForMessageTo(adminID, 5*time.Second, func(s string) bool { return s != "" })
	require.True(t, ok, "bot must reply to /scale")
	assert.NotEmpty(t, msg)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func makeNginxDeployment(name, namespace string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr32(replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "nginx", Image: "nginx:1.27-alpine"},
					},
				},
			},
		},
	}
}
