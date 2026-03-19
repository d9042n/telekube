//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tclog "github.com/testcontainers/testcontainers-go/log"
	k3scontainer "github.com/testcontainers/testcontainers-go/modules/k3s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

// K3sCluster manages a k3s Kubernetes cluster running inside a Docker container.
// It wraps testcontainers-go's k3s module for test-lifecycle management.
type K3sCluster struct {
	container      *k3scontainer.K3sContainer
	kubeconfigPath string
	clientset      kubernetes.Interface
}

// NewK3sCluster starts a k3s container, writes the kubeconfig to t.TempDir(),
// and registers cleanup via t.Cleanup. It fails the test immediately if the
// cluster cannot be started.
func NewK3sCluster(t *testing.T) *K3sCluster {
	t.Helper()

	// Respect E2E_SKIP_CLUSTER env var to allow testing only Telegram logic.
	if skipCluster() {
		t.Skip("E2E_SKIP_CLUSTER=true — skipping k3s cluster tests")
	}

	ctx := context.Background()

	container, err := k3scontainer.Run(ctx, "rancher/k3s:v1.31.4-k3s1",
		testcontainers.WithLogger(tclog.TestLogger(t)),
	)
	if err != nil {
		t.Fatalf("starting k3s container: %v", err)
	}

	t.Cleanup(func() {
		if termErr := container.Terminate(context.Background()); termErr != nil {
			t.Logf("warning: failed to terminate k3s container: %v", termErr)
		}
	})

	// Export kubeconfig to a temp file.
	kubeconfigBytes, err := container.GetKubeConfig(ctx)
	if err != nil {
		t.Fatalf("getting k3s kubeconfig: %v", err)
	}

	kubeconfigPath := t.TempDir() + "/kubeconfig.yaml"
	if err := writeFile(kubeconfigPath, kubeconfigBytes); err != nil {
		t.Fatalf("writing kubeconfig: %v", err)
	}

	// Build a Kubernetes client from the kubeconfig.
	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		t.Fatalf("building rest config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		t.Fatalf("creating kubernetes client: %v", err)
	}

	cluster := &K3sCluster{
		container:      container,
		kubeconfigPath: kubeconfigPath,
		clientset:      clientset,
	}

	// Wait for the cluster to be ready before returning.
	cluster.waitReady(t, 120*time.Second)

	return cluster
}

// KubeconfigPath returns the path to the written kubeconfig file.
func (k *K3sCluster) KubeconfigPath() string {
	return k.kubeconfigPath
}

// ClientSet returns the kubernetes client connected to k3s.
func (k *K3sCluster) ClientSet() kubernetes.Interface {
	return k.clientset
}

// ApplyFixture reads a YAML file from yamlPath and applies it to the cluster
// using the Kubernetes API — no kubectl binary required.
func (k *K3sCluster) ApplyFixture(t *testing.T, yamlPath string) {
	t.Helper()
	data := readFile(t, yamlPath)
	k.applyYAML(t, data)
}

// ApplyRaw applies raw YAML content to the cluster.
func (k *K3sCluster) ApplyRaw(t *testing.T, rawYAML []byte) {
	t.Helper()
	k.applyYAML(t, rawYAML)
}

// WaitForPod polls until pod namespace/name is in Ready condition, or the
// timeout expires. It fails the test if the pod never becomes ready.
func (k *K3sCluster) WaitForPod(t *testing.T, namespace, name string, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		pod, err := k.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil // not yet created
		}
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("pod %s/%s never became Ready within %s: %v", namespace, name, timeout, err)
	}
}

// WaitForPodPhase polls until the pod reaches the specified phase (e.g., "Failed")
// or the timeout expires.
func (k *K3sCluster) WaitForPodPhase(t *testing.T, namespace, name string, phase corev1.PodPhase, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		pod, err := k.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		return pod.Status.Phase == phase, nil
	})
	if err != nil {
		t.Fatalf("pod %s/%s never reached phase %s within %s: %v", namespace, name, phase, timeout, err)
	}
}

// WaitForCrashLoopBackOff polls until at least one container in the pod is in CrashLoopBackOff.
func (k *K3sCluster) WaitForCrashLoopBackOff(t *testing.T, namespace, name string, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		pod, err := k.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("pod %s/%s never reached CrashLoopBackOff within %s: %v", namespace, name, timeout, err)
	}
}

// DeletePod removes the named pod (ignores not-found errors).
func (k *K3sCluster) DeletePod(t *testing.T, namespace, name string) {
	t.Helper()
	ctx := context.Background()
	err := k.clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !isNotFound(err) {
		t.Logf("warning: deleting pod %s/%s: %v", namespace, name, err)
	}
}

// CreateNamespace ensures a namespace exists.
func (k *K3sCluster) CreateNamespace(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	_, err := k.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !isAlreadyExists(err) {
		t.Fatalf("creating namespace %s: %v", name, err)
	}
}

// DeleteNamespace removes a namespace and all its resources (async, best-effort).
func (k *K3sCluster) DeleteNamespace(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	if err := k.clientset.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		t.Logf("warning: deleting namespace %s: %v", name, err)
	}
}

// ─── Private helpers ──────────────────────────────────────────────────────────

// waitReady blocks until at least one node reports Ready, or fails the test.
func (k *K3sCluster) waitReady(t *testing.T, timeout time.Duration) {
	t.Helper()
	t.Log("waiting for k3s cluster to be ready...")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		nodes, err := k.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		for _, node := range nodes.Items {
			for _, cond := range node.Status.Conditions {
				if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
					return true, nil
				}
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("k3s cluster never became ready within %s: %v", timeout, err)
	}
	t.Log("k3s cluster is ready")
}

// applyYAML applies multi-document YAML to the cluster by splitting on "---" and
// creating each resource via the appropriate typed API.
func (k *K3sCluster) applyYAML(t *testing.T, data []byte) {
	t.Helper()
	ctx := context.Background()
	docs := splitYAML(data)

	for _, doc := range docs {
		if strings.TrimSpace(string(doc)) == "" {
			continue
		}

		// Decode enough to route to the right API.
		var meta struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
			Metadata   struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
		}
		if err := yaml.Unmarshal(doc, &meta); err != nil {
			t.Fatalf("parsing YAML document: %v", err)
		}

		switch strings.ToLower(meta.Kind) {
		case "pod":
			var pod corev1.Pod
			if err := yaml.Unmarshal(doc, &pod); err != nil {
				t.Fatalf("unmarshaling Pod: %v", err)
			}
			ns := pod.Namespace
			if ns == "" {
				ns = "default"
			}
			_, err := k.clientset.CoreV1().Pods(ns).Create(ctx, &pod, metav1.CreateOptions{})
			if err != nil && !isAlreadyExists(err) {
				t.Fatalf("creating pod %s/%s: %v", ns, pod.Name, err)
			}

		case "deployment":
			var deploy corev1.Pod // placeholder; use unstructured approach below
			_ = deploy
			t.Logf("Deployment apply via applyYAML not implemented; use ApplyRaw with unstructured client")

		default:
			t.Logf("ApplyFixture: skipping unhandled kind %q (name=%s)", meta.Kind, meta.Metadata.Name)
		}
	}
}

// splitYAML splits a YAML byte slice by "---" document separators.
func splitYAML(data []byte) [][]byte {
	var docs [][]byte
	parts := strings.Split(string(data), "---")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			docs = append(docs, []byte(trimmed))
		}
	}
	return docs
}

// isNotFound returns true for Kubernetes "not found" API errors.
func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not found")
}

// isAlreadyExists returns true for Kubernetes "already exists" API errors.
func isAlreadyExists(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already exists")
}




// writeFile writes data to path, creating parent dirs as needed.
func writeFile(path string, data []byte) error {
	return writeFileWithPerm(path, data, 0600)
}

// readFile reads the contents of path, failing the test on error.
func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := readFileBytes(path)
	if err != nil {
		t.Fatalf("reading file %s: %v", path, err)
	}
	return data
}

// FixturePath returns the absolute path to a testdata fixture file.
func FixturePath(name string) string {
	return fmt.Sprintf("testdata/fixtures/%s", name)
}
