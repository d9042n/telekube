// Package kube provides Kubernetes client factory utilities.
package kube

import (
	"fmt"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

// ClientConfig holds the configuration for creating a K8s client.
type ClientConfig struct {
	Kubeconfig string
	Context    string
	InCluster  bool
}

// Clients holds all K8s client types for a cluster.
type Clients struct {
	ClientSet     kubernetes.Interface
	MetricsClient metricsv.Interface
	DynamicClient dynamic.Interface
}

// NewClients creates all K8s clients from config.
func NewClients(cfg ClientConfig) (*Clients, error) {
	restCfg, err := buildRESTConfig(cfg)
	if err != nil {
		return nil, err
	}

	// Tune connection
	restCfg.Timeout = 10 * time.Second
	restCfg.QPS = 50
	restCfg.Burst = 100

	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes clientset: %w", err)
	}

	mc, err := metricsv.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("creating metrics client: %w", err)
	}

	dc, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("creating dynamic client: %w", err)
	}

	return &Clients{
		ClientSet:     cs,
		MetricsClient: mc,
		DynamicClient: dc,
	}, nil
}

func buildRESTConfig(cfg ClientConfig) (*rest.Config, error) {
	if cfg.InCluster {
		restCfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("building in-cluster config: %w", err)
		}
		return restCfg, nil
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if cfg.Kubeconfig != "" {
		loadingRules.ExplicitPath = cfg.Kubeconfig
	}

	overrides := &clientcmd.ConfigOverrides{}
	if cfg.Context != "" {
		overrides.CurrentContext = cfg.Context
	}

	restCfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("building kubeconfig: %w", err)
	}

	return restCfg, nil
}
