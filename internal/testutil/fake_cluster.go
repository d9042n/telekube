// Package testutil provides shared test helpers for the Telekube test suite.
// All helpers are dependency-free (no Docker, no real cluster) and are intended
// for use in unit and handler-level tests only.
package testutil

import (
	"context"
	"fmt"

	"github.com/d9042n/telekube/internal/entity"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// FakeClusterManager is a test double for cluster.Manager.
// It holds one named cluster ("test-cluster") backed by a fake kubernetes clientset.
type FakeClusterManager struct {
	clientSet     kubernetes.Interface
	metricsClient metricsv.Interface
	dynamicClient dynamic.Interface
	clusters      []entity.ClusterInfo
}

// NewFakeClusterManager creates a fake cluster manager pre-seeded with the
// provided Kubernetes objects (pods, nodes, deployments, namespaces, etc.).
// The cluster is always named "test-cluster".
func NewFakeClusterManager(objects ...runtime.Object) *FakeClusterManager {
	return NewFakeClusterManagerNamed("test-cluster", objects...)
}

// NewFakeClusterManagerNamed creates a fake cluster manager with a custom name.
func NewFakeClusterManagerNamed(clusterName string, objects ...runtime.Object) *FakeClusterManager {
	cs := kubefake.NewSimpleClientset(objects...)
	ms := metricsfake.NewSimpleClientset()
	ds := fake.NewSimpleDynamicClient(scheme.Scheme)

	return &FakeClusterManager{
		clientSet:     cs,
		metricsClient: ms,
		dynamicClient: ds,
		clusters: []entity.ClusterInfo{
			{
				Name:        clusterName,
				DisplayName: clusterName,
				IsDefault:   true,
				Status:      entity.HealthStatusHealthy,
			},
		},
	}
}

// List implements cluster.Manager.
func (f *FakeClusterManager) List() []entity.ClusterInfo {
	return f.clusters
}

// Get implements cluster.Manager.
func (f *FakeClusterManager) Get(name string) (*entity.ClusterInfo, error) {
	for _, c := range f.clusters {
		if c.Name == name {
			info := c
			return &info, nil
		}
	}
	return nil, fmt.Errorf("cluster %q not found", name)
}

// GetDefault implements cluster.Manager.
func (f *FakeClusterManager) GetDefault() (*entity.ClusterInfo, error) {
	for _, c := range f.clusters {
		if c.IsDefault {
			info := c
			return &info, nil
		}
	}
	if len(f.clusters) > 0 {
		info := f.clusters[0]
		return &info, nil
	}
	return nil, fmt.Errorf("no default cluster configured")
}

// ClientSet implements cluster.Manager.
func (f *FakeClusterManager) ClientSet(_ string) (kubernetes.Interface, error) {
	return f.clientSet, nil
}

// MetricsClient implements cluster.Manager.
func (f *FakeClusterManager) MetricsClient(_ string) (metricsv.Interface, error) {
	return f.metricsClient, nil
}

// DynamicClient implements cluster.Manager.
func (f *FakeClusterManager) DynamicClient(_ string) (dynamic.Interface, error) {
	return f.dynamicClient, nil
}

// HealthCheck implements cluster.Manager.
func (f *FakeClusterManager) HealthCheck(_ context.Context) map[string]entity.HealthStatus {
	result := make(map[string]entity.HealthStatus)
	for _, c := range f.clusters {
		result[c.Name] = entity.HealthStatusHealthy
	}
	return result
}

// RESTConfig implements cluster.Manager.
func (f *FakeClusterManager) RESTConfig(_ string) (*rest.Config, error) {
	return &rest.Config{Host: "https://fake-cluster:6443"}, nil
}

// Close implements cluster.Manager.
func (f *FakeClusterManager) Close() error { return nil }

// FakeClusterManagerError is a cluster.Manager that always returns errors.
// Useful for testing error-handling code paths.
type FakeClusterManagerError struct {
	FakeClusterManager
	Err error
}

// NewFakeClusterManagerError creates a manager whose ClientSet always errors.
func NewFakeClusterManagerError(err error) *FakeClusterManagerError {
	return &FakeClusterManagerError{
		FakeClusterManager: *NewFakeClusterManager(),
		Err:                err,
	}
}

func (f *FakeClusterManagerError) ClientSet(_ string) (kubernetes.Interface, error) {
	return nil, f.Err
}

// DefaultNamespace creates a Namespace object for "default" — required by many tests.
func DefaultNamespace() *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: NewObjectMeta("default", ""),
	}
}
