package kubernetes

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ─── podStatus tests ──────────────────────────────────────────────────────────

func TestPodStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected string
	}{
		{
			name: "running pod",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			},
			expected: "Running",
		},
		{
			name: "pending pod",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{Phase: corev1.PodPending},
			},
			expected: "Pending",
		},
		{
			name: "CrashLoopBackOff detected via container status",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason: "CrashLoopBackOff",
								},
							},
						},
					},
				},
			},
			expected: "CrashLoopBackOff",
		},
		{
			name: "ImagePullBackOff detected via container status",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason: "ImagePullBackOff",
								},
							},
						},
					},
				},
			},
			expected: "ImagePullBackOff",
		},
		{
			name: "container with empty waiting reason uses Phase",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{Reason: ""},
							},
						},
					},
				},
			},
			expected: "Pending",
		},
		{
			name: "terminating pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
				},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			},
			expected: "Terminating",
		},
		{
			name:     "unknown phase",
			pod:      &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodUnknown}},
			expected: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, podStatus(tt.pod))
		})
	}
}

// ─── formatPodLine tests ──────────────────────────────────────────────────────

func TestFormatPodLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		pod             *corev1.Pod
		wantSubstrings  []string
	}{
		{
			name: "running pod no restarts",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "nginx",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
				},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			},
			wantSubstrings: []string{"nginx", "Running", "10m"},
		},
		{
			name: "crashed pod shows restart count",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "worker",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "app", RestartCount: 3,
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
							},
						},
					},
				},
			},
			wantSubstrings: []string{"worker", "CrashLoopBackOff", "restarts: 3"},
		},
		{
			name: "pod age in hours",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "db",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-5 * time.Hour)},
				},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			},
			wantSubstrings: []string{"5h"},
		},
		{
			name: "pod age in days",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "db",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-48 * time.Hour)},
				},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			},
			wantSubstrings: []string{"2d"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			line := formatPodLine(tt.pod)
			for _, sub := range tt.wantSubstrings {
				assert.True(t, strings.Contains(line, sub),
					"expected %q in line %q", sub, line)
			}
		})
	}
}

// ─── formatPodDetail tests ────────────────────────────────────────────────────

func TestFormatPodDetail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		pod            *corev1.Pod
		clusterName    string
		wantSubstrings []string
	}{
		{
			name: "running pod with IP and node",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "nginx",
					Namespace:         "default",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
				},
				Spec: corev1.PodSpec{
					NodeName:   "node-1",
					Containers: []corev1.Container{{Image: "nginx:latest"}},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					PodIP: "10.0.0.5",
				},
			},
			clusterName:    "prod",
			wantSubstrings: []string{"nginx", "default", "node-1", "10.0.0.5", "prod", "nginx:latest"},
		},
		{
			name: "CrashLoopBackOff container shows 🔴",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "bad-pod", Namespace: "default"},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name:         "app",
							RestartCount: 5,
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
							},
						},
					},
				},
			},
			clusterName:    "dev",
			wantSubstrings: []string{"🔴", "CrashLoopBackOff", "restarts: 5"},
		},
		{
			name: "pod without optional fields",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "minimal", Namespace: "kube-system"},
			},
			clusterName:    "test",
			wantSubstrings: []string{"minimal", "kube-system", "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			detail := formatPodDetail(tt.pod, tt.clusterName)
			for _, sub := range tt.wantSubstrings {
				assert.True(t, strings.Contains(detail, sub),
					"expected %q in detail:\n%s", sub, detail)
			}
		})
	}
}

// ─── formatDuration tests ─────────────────────────────────────────────────────

func TestFormatDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		d        time.Duration
		expected string
	}{
		{"seconds", 45 * time.Second, "45s"},
		{"minutes", 90 * time.Second, "1m"},
		{"hours", 90 * time.Minute, "1h"},
		{"days", 72 * time.Hour, "3d"},
		{"zero", 0, "0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, formatDuration(tt.d))
		})
	}
}

// ─── PodStatus: multiple containers — first non-empty reason wins ─────────────

func TestPodStatusMultiContainerFirstNonEmptyReasonWins(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					// First container has empty Waiting.Reason — implementation skips it
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: ""},
					},
				},
				{
					// Second container has a real reason — should be returned
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
					},
				},
			},
		},
	}

	// Implementation skips containers with empty Waiting.Reason and
	// returns the first non-empty reason (CrashLoopBackOff from container 2).
	assert.Equal(t, "CrashLoopBackOff", podStatus(pod),
		"should return second container reason when first has empty Waiting.Reason")
}

// ─── formatPodDetail branch tests ─────────────────────────────────────────────

func TestFormatPodDetail_WithNodeAndIP(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "mypod",
			Namespace:         "default",
			CreationTimestamp: metav1.Now(),
		},
		Spec: corev1.PodSpec{
			NodeName:   "node-1",
			Containers: []corev1.Container{{Name: "app", Image: "nginx:latest"}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "10.0.0.1",
		},
	}

	result := formatPodDetail(pod, "prod")
	assert.Contains(t, result, "node-1", "should show node name")
	assert.Contains(t, result, "10.0.0.1", "should show pod IP")
	assert.Contains(t, result, "nginx:latest")
	assert.Contains(t, result, "prod")
}

func TestFormatPodDetail_TerminatedContainer_ShowsExitCode(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mypod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "nginx"}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodFailed,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 3,
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 1,
							Reason:   "", // empty → fallback to exit code
						},
					},
				},
			},
		},
	}

	result := formatPodDetail(pod, "prod")
	assert.Contains(t, result, "exit code 1", "empty terminated reason should show exit code")
	assert.Contains(t, result, "restarts: 3", "restart count should appear")
}

func TestFormatPodDetail_TerminatedContainer_ShowsReason(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mypod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "nginx"}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "app",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 0,
							Reason:   "Completed",
						},
					},
				},
			},
		},
	}

	result := formatPodDetail(pod, "prod")
	assert.Contains(t, result, "Completed", "terminated reason should appear")
}

func TestFormatPodDetail_InitContainer_CompletedAndWaiting(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mypod",
			Namespace: "ns",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "myimage"}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			InitContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "init-complete",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{ExitCode: 0},
					},
				},
				{
					Name: "init-waiting",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"},
					},
				},
			},
		},
	}

	result := formatPodDetail(pod, "prod")
	assert.Contains(t, result, "init-complete")
	assert.Contains(t, result, "Completed", "completed init should show 'Completed'")
	assert.Contains(t, result, "init-waiting")
	assert.Contains(t, result, "PodInitializing")
}
