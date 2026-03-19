package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNodeConditionDisplay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		node          *corev1.Node
		expectedEmoji string
		expectedText  string
	}{
		{
			name: "unschedulable node",
			node: &corev1.Node{
				Spec: corev1.NodeSpec{
					Unschedulable: true,
				},
			},
			expectedEmoji: "🟡",
			expectedText:  "SchedulingDisabled",
		},
		{
			name: "ready node",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expectedEmoji: "🟢",
			expectedText:  "Ready",
		},
		{
			name: "not ready node",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			expectedEmoji: "🔴",
			expectedText:  "NotReady",
		},
		{
			name: "unknown condition node",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionUnknown,
						},
					},
				},
			},
			expectedEmoji: "🔴",
			expectedText:  "Unknown",
		},
		{
			name:          "no conditions",
			node:          &corev1.Node{},
			expectedEmoji: "⚪",
			expectedText:  "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			emoji, text := nodeConditionDisplay(tt.node)
			assert.Equal(t, tt.expectedEmoji, emoji)
			assert.Equal(t, tt.expectedText, text)
		})
	}
}

func TestIsDaemonSetPod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name: "daemonset pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "DaemonSet", Name: "fluentd"},
					},
				},
			},
			expected: true,
		},
		{
			name: "replicaset pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "ReplicaSet", Name: "nginx-rs"},
					},
				},
			},
			expected: false,
		},
		{
			name:     "no owner reference",
			pod:      &corev1.Pod{},
			expected: false,
		},
		{
			name: "multiple owners including daemonset",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "ReplicaSet", Name: "nginx"},
						{Kind: "DaemonSet", Name: "fluentd"},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := isDaemonSetPod(tt.pod)
			assert.Equal(t, tt.expected, result)
		})
	}
}
