//go:build e2e

// Package e2e provides the end-to-end test harness for Telekube.
//
// Usage:
//
//	func TestMyScenario(t *testing.T) {
//	    h := NewHarness(t, WithoutK3s())
//	    h.SendMessage(12345, "admin", "/start")
//	    msg, ok := h.WaitForMessageTo(12345, 5*time.Second, func(s string) bool {
//	        return strings.Contains(s, "Welcome")
//	    })
//	    require.True(t, ok)
//	}
package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/audit"
	"github.com/d9042n/telekube/internal/bot"
	"github.com/d9042n/telekube/internal/cluster"
	"github.com/d9042n/telekube/internal/config"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/module"
	helmmod "github.com/d9042n/telekube/internal/module/helm"
	kubemod "github.com/d9042n/telekube/internal/module/kubernetes"
	watchermod "github.com/d9042n/telekube/internal/module/watcher"
	"github.com/d9042n/telekube/internal/rbac"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/d9042n/telekube/internal/storage/sqlite"
	"go.uber.org/zap"
	helmchart "helm.sh/helm/v3/pkg/chart"
	helmrelease "helm.sh/helm/v3/pkg/release"
	helmtime "helm.sh/helm/v3/pkg/time"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	k8srest "k8s.io/client-go/rest"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

// defaultProcessDelay is how long we wait after injecting an update for the bot
// to process it before we check for responses.
const defaultProcessDelay = 400 * time.Millisecond

// Harness wires:
//   - FakeTelegramServer  — intercepts all Telegram API calls
//   - K3sCluster          — real k8s in Docker (skipped with WithoutK3s or E2E_SKIP_CLUSTER)
//   - SQLite :memory:     — clean storage per test
//   - Fully-initialised Telekube bot pointed at the fake server
type Harness struct {
	t        *testing.T
	Telegram *FakeTelegramServer
	K8s      *K3sCluster // nil when no k3s
	Storage  storage.Storage
	Registry *module.Registry
	logger   *zap.Logger
	store    storage.Storage
}

// HarnessOption is a functional option for Harness creation.
type HarnessOption func(*harnessConfig)

type harnessConfig struct {
	skipK3s  bool
	adminIDs []int64
}

// WithoutK3s skips starting the k3s container. Bot still boots with a noop cluster.
func WithoutK3s() HarnessOption { return func(c *harnessConfig) { c.skipK3s = true } }

// WithAdminIDs overrides which Telegram user IDs are auto-granted admin role.
func WithAdminIDs(ids ...int64) HarnessOption { return func(c *harnessConfig) { c.adminIDs = ids } }

// NewHarness creates and starts an E2E harness.
// All cleanup (k3s container, bot goroutine, storage) is registered with t.Cleanup.
func NewHarness(t *testing.T, opts ...HarnessOption) *Harness {
	t.Helper()

	cfg := &harnessConfig{adminIDs: []int64{999999}} // safe default test admin
	for _, o := range opts {
		o(cfg)
	}

	h := &Harness{t: t}

	// ── 1. Fake Telegram server ──────────────────────────────────────────────
	h.Telegram = NewFakeTelegramServer()
	t.Cleanup(h.Telegram.Close)

	// ── 2. In-memory SQLite storage ──────────────────────────────────────────
	store, err := sqlite.New(":memory:?cache=shared")
	if err != nil {
		t.Fatalf("creating in-memory sqlite: %v", err)
	}
	h.Storage = store
	h.store = store
	t.Cleanup(func() { _ = store.Close() })

	// ── 3. Logger (no-op keeps test output clean; swap for zap.NewExample() to debug) ──
	logger := zap.NewNop()
	h.logger = logger

	// ── 4. Cluster manager ───────────────────────────────────────────────────
	var clusterMgr cluster.Manager
	if !cfg.skipK3s && !skipCluster() {
		k3s := NewK3sCluster(t)
		h.K8s = k3s
		clusterMgr = cluster.NewManager([]config.ClusterConfig{{
			Name:       "e2e-cluster",
			Kubeconfig: k3s.KubeconfigPath(),
			Default:    true,
		}}, logger)
	} else {
		clusterMgr = newNoopClusterManager()
	}
	t.Cleanup(func() { _ = clusterMgr.Close() })

	// ── 5. Core services ─────────────────────────────────────────────────────
	rbacEngine := rbac.NewEngine("viewer", store.RBAC(), cfg.adminIDs)

	auditLogger := audit.NewLogger(store.Audit(), logger)
	t.Cleanup(func() { _ = auditLogger.Close() })

	// ── 6. Module registry ───────────────────────────────────────────────────
	registry := module.NewRegistry(logger)
	userCtx := cluster.NewUserContext(clusterMgr)
	if regErr := registry.Register(kubemod.NewModule(clusterMgr, userCtx, rbacEngine, auditLogger, logger)); regErr != nil {
		t.Logf("warning: registering kubernetes module: %v", regErr)
	}

	// Register Helm module with a fake client factory.
	helmClusters := []helmmod.ClusterClient{
		{Name: "test-cluster"},
	}
	helmModule := helmmod.NewModule(helmClusters, rbacEngine, auditLogger, logger)
	helmModule.SetClientFactory(fakeHelmClientFactory)
	if regErr := registry.Register(helmModule); regErr != nil {
		t.Logf("warning: registering helm module: %v", regErr)
	}

	// Register Watcher module — needs bot notifier (wired after bot creation).
	// We create the module here so it's registered for button handlers, but
	// defer StartWatchers() until after the bot starts.
	var watcherModule *watchermod.Module

	h.Registry = registry

	// ── 7. Bot (URL overridden to fake server) ───────────────────────────────
	telegramCfg := config.TelegramConfig{
		Token:     h.Telegram.Token(),
		AdminIDs:  cfg.adminIDs,
		RateLimit: 60,
	}

	// We modify the bot's internal URL by patching TelegramConfig.WebhookURL
	// trick: telebot reads Settings.URL for the API base URL.
	// We expose a WithURL option from the bot package.
	teleBot, err := bot.NewWithURL(telegramCfg, clusterMgr, store, rbacEngine, auditLogger, registry, logger, h.Telegram.URL())
	if err != nil {
		t.Fatalf("creating bot: %v", err)
	}

	// Now create the watcher module with the bot notifier.
	notifier := bot.NewNotifier(teleBot)
	watcherModule = watchermod.NewModule(clusterMgr, notifier, auditLogger, telegramCfg, logger)
	if regErr := registry.Register(watcherModule); regErr != nil {
		t.Logf("warning: registering watcher module: %v", regErr)
	}

	// ── 8. Start modules & bot ──────────────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	if startErr := registry.StartAll(ctx); startErr != nil {
		t.Logf("warning: starting modules: %v", startErr)
	}

	go teleBot.Start()

	// Start watchers now that the bot is running and can send messages.
	if h.K8s != nil && watcherModule != nil {
		watcherModule.StartWatchers(ctx)
	}

	t.Cleanup(func() {
		cancel()
		teleBot.Stop()
		registry.StopAll(context.Background())
	})

	return h
}

// ─── Interaction helpers ──────────────────────────────────────────────────────

// SendMessage injects a text command from userID and waits for bot processing.
func (h *Harness) SendMessage(userID int64, username, text string) {
	h.t.Helper()
	h.Telegram.InjectTextMessage(userID, username, text)
	time.Sleep(defaultProcessDelay)
}

// SendCallback injects an inline keyboard button callback.
func (h *Harness) SendCallback(userID int64, username, unique, data string) {
	h.t.Helper()
	h.Telegram.InjectCallback(userID, username, unique, data)
	time.Sleep(defaultProcessDelay)
}

// WaitForMessage polls until any sent message matches predicate (any chat).
func (h *Harness) WaitForMessage(timeout time.Duration, predicate func(string) bool) (string, bool) {
	return h.Telegram.WaitForMessage(timeout, predicate)
}

// WaitForMessageTo polls until a message to chatID matches predicate.
func (h *Harness) WaitForMessageTo(chatID int64, timeout time.Duration, predicate func(string) bool) (string, bool) {
	return h.Telegram.WaitForMessageTo(chatID, timeout, predicate)
}

// LastMessageTo returns the most recent message sent to chatID.
func (h *Harness) LastMessageTo(chatID int64) string {
	return h.Telegram.LastMessageTo(chatID)
}

// ClearMessages resets all recorded messages for test isolation.
func (h *Harness) ClearMessages() { h.Telegram.ClearMessages() }

// ─── Seed helpers ─────────────────────────────────────────────────────────────

// SeedUserRole directly writes a role into storage for the given Telegram user.
func (h *Harness) SeedUserRole(userID int64, role string) {
	h.t.Helper()
	if err := h.store.RBAC().SetUserRole(context.Background(), userID, role); err != nil {
		h.t.Fatalf("seeding user %d role=%s: %v", userID, role, err)
	}
}

// SeedUser upserts a user entity (needed before SetUserRole for FK constraints).
func (h *Harness) SeedUser(userID int64, username, role string) {
	h.t.Helper()
	ctx := context.Background()
	user := &entity.User{
		TelegramID:  userID,
		Username:    username,
		DisplayName: username,
		Role:        role,
		IsActive:    true,
	}
	if err := h.store.Users().Upsert(ctx, user); err != nil {
		h.t.Fatalf("seeding user %d: %v", userID, err)
	}
	if err := h.store.RBAC().SetUserRole(ctx, userID, role); err != nil {
		h.t.Fatalf("seeding user %d role: %v", userID, err)
	}
}

// ─── Assertion helpers ────────────────────────────────────────────────────────

// AssertMessageContains fails if none of the messages to chatID contain substr.
func (h *Harness) AssertMessageContains(chatID int64, substr string) {
	h.t.Helper()
	msgs := h.Telegram.MessagesTo(chatID)
	for _, m := range msgs {
		if strings.Contains(m.Text, substr) {
			return
		}
	}
	var texts []string
	for _, m := range msgs {
		texts = append(texts, fmt.Sprintf("%q", m.Text))
	}
	h.t.Errorf("no message to chat %d contains %q\n  got: [%s]", chatID, substr, strings.Join(texts, ", "))
}

// AssertNoMessageContains fails if any message to chatID contains substr.
func (h *Harness) AssertNoMessageContains(chatID int64, substr string) {
	h.t.Helper()
	for _, m := range h.Telegram.MessagesTo(chatID) {
		if strings.Contains(m.Text, substr) {
			h.t.Errorf("unexpected message to chat %d containing %q: %q", chatID, substr, m.Text)
		}
	}
}

// AssertBotReplied waits up to timeout for the bot to send any message to chatID.
// Returns the first message received.
func (h *Harness) AssertBotReplied(chatID int64, timeout time.Duration) string {
	h.t.Helper()
	msg, ok := h.WaitForMessageTo(chatID, timeout, func(s string) bool { return s != "" })
	if !ok {
		h.t.Errorf("bot did not reply to chat %d within %s", chatID, timeout)
	}
	return msg
}

// UniqueTestNamespace returns a k8s-safe namespace name scoped to the test.
func UniqueTestNamespace(t *testing.T) string {
	name := strings.ToLower(t.Name())
	name = strings.NewReplacer("/", "-", "_", "-", " ", "-").Replace(name)
	if len(name) > 60 {
		name = name[:60]
	}
	return "e2e-" + name
}

// ─── noopClusterManager ───────────────────────────────────────────────────────

// noopClusterManager satisfies cluster.Manager with no real cluster backing.
type noopClusterManager struct{}

func newNoopClusterManager() cluster.Manager { return &noopClusterManager{} }

func (n *noopClusterManager) List() []entity.ClusterInfo { return nil }
func (n *noopClusterManager) Get(_ string) (*entity.ClusterInfo, error) {
	return nil, fmt.Errorf("noop cluster manager: no clusters")
}
func (n *noopClusterManager) GetDefault() (*entity.ClusterInfo, error) {
	return nil, fmt.Errorf("noop cluster manager: no default cluster")
}
func (n *noopClusterManager) ClientSet(_ string) (kubernetes.Interface, error) {
	return nil, fmt.Errorf("noop cluster manager: no kubernetes client")
}
func (n *noopClusterManager) MetricsClient(_ string) (metricsv.Interface, error) {
	return nil, fmt.Errorf("noop cluster manager: no metrics client")
}
func (n *noopClusterManager) DynamicClient(_ string) (dynamic.Interface, error) {
	return nil, fmt.Errorf("noop cluster manager: no dynamic client")
}
func (n *noopClusterManager) HealthCheck(_ context.Context) map[string]entity.HealthStatus {
	return nil
}
func (n *noopClusterManager) Close() error { return nil }

// ─── fakeHelmClient for E2E ─────────────────────────────────────────────────

// fakeHelmClientFactory always returns a fakeReleaseClient.
func fakeHelmClientFactory(_ *k8srest.Config, _ string) (helmmod.ReleaseClient, error) {
	return &fakeReleaseClient{}, nil
}

type fakeReleaseClient struct{}

func (f *fakeReleaseClient) ListReleases() ([]*helmrelease.Release, error) {
	return []*helmrelease.Release{
		{
			Name:      "nginx-ingress",
			Namespace: "default",
			Version:   3,
			Info:      &helmrelease.Info{Status: helmrelease.StatusDeployed, LastDeployed: helmtime.Now()},
			Chart:     &helmchart.Chart{Metadata: &helmchart.Metadata{Name: "nginx-ingress", Version: "4.11.0", AppVersion: "1.11.0"}},
		},
	}, nil
}

func (f *fakeReleaseClient) GetRelease(name string) (*helmrelease.Release, error) {
	if name == "nginx-ingress" {
		rel, _ := f.ListReleases()
		return rel[0], nil
	}
	return nil, fmt.Errorf("release %q not found", name)
}

func (f *fakeReleaseClient) GetHistory(name string) ([]*helmrelease.Release, error) {
	if name == "nginx-ingress" {
		return []*helmrelease.Release{
			{Name: name, Version: 2, Info: &helmrelease.Info{Status: helmrelease.StatusSuperseded, LastDeployed: helmtime.Now()}, Chart: &helmchart.Chart{Metadata: &helmchart.Metadata{Name: "nginx-ingress", Version: "4.10.0", AppVersion: "1.10.0"}}},
			{Name: name, Version: 3, Info: &helmrelease.Info{Status: helmrelease.StatusDeployed, LastDeployed: helmtime.Now()}, Chart: &helmchart.Chart{Metadata: &helmchart.Metadata{Name: "nginx-ingress", Version: "4.11.0", AppVersion: "1.11.0"}}},
		}, nil
	}
	return nil, fmt.Errorf("release %q not found", name)
}

func (f *fakeReleaseClient) Rollback(_ string, _ int) error { return nil }
