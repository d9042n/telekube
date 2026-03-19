package testutil

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewObjectMeta returns a metav1.ObjectMeta with the given name and namespace.
func NewObjectMeta(name, namespace string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
	}
}

// NewRunningPod creates a simple pod in Running phase for testing.
func NewRunningPod(name, namespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: NewObjectMeta(name, namespace),
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
}

// NewCrashLoopPod creates a pod that appears to be in CrashLoopBackOff.
func NewCrashLoopPod(name, namespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: NewObjectMeta(name, namespace),
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CrashLoopBackOff",
						},
					},
				},
			},
		},
	}
}

// NewReadyNode creates a schedulable, Ready node for testing.
func NewReadyNode(name string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: NewObjectMeta(name, ""),
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
}

// NewCordonedNode creates a node that is marked unschedulable.
func NewCordonedNode(name string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: NewObjectMeta(name, ""),
		Spec: corev1.NodeSpec{
			Unschedulable: true,
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
}

// NewNotReadyNode creates a node with NotReady condition.
func NewNotReadyNode(name string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: NewObjectMeta(name, ""),
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}
}
