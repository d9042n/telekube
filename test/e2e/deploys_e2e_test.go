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
	deploysAdminID  = int64(999999)
	deploysViewerID = int64(600002)
)

func TestE2E_Deploys_Smoke(t *testing.T) {
	h := newSmokeHarness(t, deploysAdminID)
	h.SeedUser(deploysAdminID, "testadmin", "admin")

	h.SendMessage(deploysAdminID, "testadmin", "/deploys")
	msg, ok := h.WaitForMessageTo(deploysAdminID, 5*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must reply to /deploys")
	assert.NotContains(t, strings.ToLower(msg), "unknown command")
}

func TestE2E_Deploys_PermissionDenied(t *testing.T) {
	h := newSmokeHarness(t, deploysAdminID)
	h.SeedUser(deploysViewerID, "viewer1", "viewer")

	h.SendMessage(deploysViewerID, "viewer1", "/deploys")
	msg, ok := h.WaitForMessageTo(deploysViewerID, 5*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must reply to /deploys from viewer")
	assert.NotEmpty(t, msg)
}

func TestE2E_Deploys_WithK3s_ListsDeployments(t *testing.T) {
	if e2e.IsSkipCluster() {
		t.Skip("E2E_SKIP_CLUSTER=true")
	}

	h := e2e.NewHarness(t, e2e.WithAdminIDs(deploysAdminID))
	ns := e2e.UniqueTestNamespace(t)
	h.K8s.CreateNamespace(t, ns)
	t.Cleanup(func() { h.K8s.DeleteNamespace(t, ns) })

	cs := h.K8s.ClientSet()
	replicas := int32(1)
	_, err := cs.AppsV1().Deployments(ns).Create(context.Background(), &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api-service", Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api-service"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api-service"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "bb", Image: "busybox:1.36", Command: []string{"sleep", "3600"}}}},
			},
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	time.Sleep(3 * time.Second)

	h.SeedUser(deploysAdminID, "testadmin", "admin")
	h.SendMessage(deploysAdminID, "testadmin", "/deploys "+ns)

	msg, ok := h.WaitForMessageTo(deploysAdminID, 10*time.Second, func(s string) bool {
		return strings.Contains(s, "api-service")
	})
	require.True(t, ok, "bot must list the deployment")
	assert.Contains(t, msg, "api-service")
}
