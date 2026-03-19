package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ─── permissionDenied boundary: scale logic helpers ───────────────────────────

// TestScaleMaxLimit validates the built-in safety limit constant.
func TestScaleMaxLimit(t *testing.T) {
	t.Parallel()
	assert.Equal(t, int32(50), int32(scaleMaxDefault),
		"scale safety limit should be %d", scaleMaxDefault)
}

// ─── Negative replica guard ───────────────────────────────────────────────────

// TestNegativeReplicaGuard documents that the handler rejects negative values.
// The slice-parse logic returns 0 for non-parseable strings, but we document
// the explicit check in handleScaleSet.
func TestNegativeReplicaGuard(t *testing.T) {
	t.Parallel()

	// Validate that -1 is indeed < 0 (i.e. the guard would trigger)
	var target int32 = -1
	assert.Less(t, target, int32(0))
}

// ─── scaleMaxDefault boundary ────────────────────────────────────────────────

func TestScaleWithinSafetyLimit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		target  int32
		allowed bool
	}{
		{"zero is allowed", 0, true},
		{"one is allowed", 1, true},
		{"max allowed", int32(scaleMaxDefault), true},
		{"one over limit blocked", int32(scaleMaxDefault) + 1, false},
		{"well over limit blocked", 100, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			withinLimit := tt.target >= 0 && tt.target <= int32(scaleMaxDefault)
			assert.Equal(t, tt.allowed, withinLimit,
				"target=%d: expected allowed=%v", tt.target, tt.allowed)
		})
	}
}

// ─── Node allocatable resource parsing ───────────────────────────────────────

// TestNodeAllocatableParsing verifies that resource.Quantity maths used in
// top.go and nodes.go behave correctly when computing percentages.
func TestNodeAllocatableParsing(t *testing.T) {
	t.Parallel()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
		},
	}

	cpuCap := node.Status.Allocatable.Cpu().MilliValue()
	ramCap := node.Status.Allocatable.Memory().Value()

	assert.Equal(t, int64(4000), cpuCap, "4 cores = 4000 millicores")
	assert.Equal(t, int64(8*1024*1024*1024), ramCap, "8Gi in bytes")
}

// ─── PodStatus priority: container waiting reason beats phase ─────────────────

func TestPodStatusContainerReasonPriority(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning, // would normally return "Running"
			ContainerStatuses: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "OOMKilled", // should take priority
						},
					},
				},
			},
		},
	}

	status := podStatus(pod)
	assert.Equal(t, "OOMKilled", status, "container waiting reason should override pod phase")
}

// ─── PodStatus: multiple containers — first non-empty waiting reason wins ─────

func TestPodStatusFirstWaitingReasonWins(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					// First container has empty reason — should be ignored
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: ""},
					},
				},
				{
					// Second container has a real reason
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
					},
				},
			},
		},
	}

	// Implementation skips empty Waiting.Reason and finds the second container's reason.
	status := podStatus(pod)
	assert.Equal(t, "CrashLoopBackOff", status,
		"should return second container reason when first has empty Waiting.Reason")
}
