//go:build e2e

package e2e_test

import (
	"testing"
	"time"

	e2e "github.com/d9042n/telekube/test/e2e"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/stretchr/testify/require"
	"context"
)

// TestK3sCluster_StartStop verifies that the k3s container can start, accept
// fixture resources, and be cleaned up correctly.
//
// This is a slow test (~60s) — it should only run with E2E_SKIP_CLUSTER unset.
func TestK3sCluster_StartStop(t *testing.T) {
	if e2e.IsSkipCluster() {
		t.Skip("E2E_SKIP_CLUSTER=true — skipping k3s cluster test")
	}

	k3s := e2e.NewK3sCluster(t)

	// The cluster must have at least one ready node.
	cs := k3s.ClientSet()
	ctx := context.Background()

	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, nodes.Items, "expected at least one node in k3s cluster")

	readyCount := 0
	for _, node := range nodes.Items {
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				readyCount++
			}
		}
	}
	require.Positive(t, readyCount, "expected at least one Ready node")
	t.Logf("k3s cluster ready: %d node(s)", readyCount)

	// Apply the nginx fixture and wait for it to be Ready.
	ns := e2e.UniqueTestNamespace(t)
	k3s.CreateNamespace(t, ns)
	t.Cleanup(func() { k3s.DeleteNamespace(t, ns) })

	// Create nginx pod programmatically (avoid relying on ApplyFixture for this unit test).
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-smoke",
			Namespace: ns,
			Labels:    map[string]string{"app": "nginx-smoke"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.27-alpine",
				},
			},
		},
	}

	_, err = cs.CoreV1().Pods(ns).Create(ctx, pod, metav1.CreateOptions{})
	require.NoError(t, err)

	k3s.WaitForPod(t, ns, "nginx-smoke", 90*time.Second)
	t.Log("nginx-smoke pod is Ready")

	// Cleanup: delete the pod explicitly (namespace deletion will also clean it up).
	k3s.DeletePod(t, ns, "nginx-smoke")
}
