package watcher

import (
	"sync"
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ─── AlertSeverity edge cases ─────────────────────────────────────────────────

func TestAlertSeverityEmoji_AllVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		severity AlertSeverity
		expected string
	}{
		{name: "critical", severity: SeverityCritical, expected: "🔴"},
		{name: "warning", severity: SeverityWarning, expected: "🟡"},
		{name: "empty string", severity: AlertSeverity(""), expected: "⚪"},
		{name: "random string", severity: AlertSeverity("info"), expected: "⚪"},
		{name: "uppercase CRITICAL", severity: AlertSeverity("CRITICAL"), expected: "⚪"},
		{name: "mixed case Warning", severity: AlertSeverity("Warning"), expected: "⚪"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.severity.Emoji())
		})
	}
}

// ─── PodWatcher alert dedup ───────────────────────────────────────────────────

func TestPodWatcher_AlertDedup_CooldownRespected(t *testing.T) {
	t.Parallel()

	w := &PodWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
	}

	key := "prod/default/my-pod/CrashLoopBackOff"

	// First call should not be in cache.
	w.mu.RLock()
	_, exists := w.alertCache[key]
	w.mu.RUnlock()
	assert.False(t, exists)

	// Set alert into cache (simulate alert fired).
	w.mu.Lock()
	w.alertCache[key] = time.Now()
	w.mu.Unlock()

	// Should be in cache now and within cooldown.
	w.mu.RLock()
	cachedTime, exists := w.alertCache[key]
	w.mu.RUnlock()
	assert.True(t, exists)
	assert.True(t, time.Since(cachedTime) < w.cooldown, "cached entry should be within cooldown")
}

func TestPodWatcher_AlertDedup_ExpiredCacheEntry(t *testing.T) {
	t.Parallel()

	w := &PodWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
	}

	key := "prod/default/my-pod/OOMKilled"

	// Set an old alert (well past cooldown).
	w.mu.Lock()
	w.alertCache[key] = time.Now().Add(-10 * time.Minute)
	w.mu.Unlock()

	w.mu.RLock()
	lastAlert := w.alertCache[key]
	w.mu.RUnlock()

	// Should be treated as expired.
	assert.True(t, time.Since(lastAlert) > w.cooldown)
}

func TestPodWatcher_MuteAlert_ExtendsCooldown(t *testing.T) {
	t.Parallel()

	w := &PodWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
	}

	key := "prod/default/my-pod/OOMKilled"

	// Mute for 2 hours.
	w.muteAlert(key, 2*time.Hour)

	w.mu.RLock()
	cachedTime, exists := w.alertCache[key]
	w.mu.RUnlock()

	require.True(t, exists)
	// The muted time should be far in the future (at least 1 hour from now).
	assert.True(t, time.Until(cachedTime) > 1*time.Hour, "muted alert should extend into the future")
}

func TestPodWatcher_MuteAlert_Concurrent_NoRace(t *testing.T) {
	t.Parallel()

	w := &PodWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := "prod/default/pod/condition"
			w.muteAlert(key, time.Duration(idx)*time.Minute)
		}(i)
	}
	wg.Wait()

	w.mu.RLock()
	_, exists := w.alertCache["prod/default/pod/condition"]
	w.mu.RUnlock()
	assert.True(t, exists, "alert cache should have the key after concurrent writes")
}

// ─── NodeWatcher alert dedup ──────────────────────────────────────────────────

func TestNodeWatcher_MuteAlert_ExtendsCooldown(t *testing.T) {
	t.Parallel()

	w := &NodeWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
	}

	key := "prod/node-1/NotReady"
	w.muteAlert(key, 1*time.Hour)

	w.mu.RLock()
	cachedTime, exists := w.alertCache[key]
	w.mu.RUnlock()

	require.True(t, exists)
	assert.True(t, time.Until(cachedTime) > 30*time.Minute)
}

func TestNodeWatcher_MuteAlert_Concurrent_NoRace(t *testing.T) {
	t.Parallel()

	w := &NodeWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			w.muteAlert("prod/node/NotReady", time.Duration(idx)*time.Minute)
		}(i)
	}
	wg.Wait()
}

// ─── PodWatcher.checkPod edge cases ──────────────────────────────────────────

func TestPodWatcher_CheckPod_OOMKilled(t *testing.T) {
	t.Parallel()

	sent := make([]string, 0)
	var mu sync.Mutex

	w := &PodWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
		cfg: config.TelegramConfig{
			AdminIDs: []int64{111},
		},
		notifier: &fakeNotifier{
			sendFunc: func(_ int64, text string, _ interface{}) error {
				mu.Lock()
				sent = append(sent, text)
				mu.Unlock()
				return nil
			},
		},
		audit:  &nopAuditLogger{},
		logger: noopLogger(),
	}

	oldPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pod", Namespace: "default"},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "app", State: corev1.ContainerState{}},
			},
		},
	}
	newPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pod", Namespace: "default"},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "app",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled"},
					},
				},
			},
		},
	}

	w.checkPod("prod-cluster", oldPod, newPod)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0], "OOMKilled")
	assert.Contains(t, sent[0], "my-pod")
}

func TestPodWatcher_CheckPod_CrashLoopBackOff(t *testing.T) {
	t.Parallel()

	sent := make([]string, 0)
	var mu sync.Mutex

	w := &PodWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
		cfg:        config.TelegramConfig{AdminIDs: []int64{111}},
		notifier: &fakeNotifier{
			sendFunc: func(_ int64, text string, _ interface{}) error {
				mu.Lock()
				sent = append(sent, text)
				mu.Unlock()
				return nil
			},
		},
		audit:  &nopAuditLogger{},
		logger: noopLogger(),
	}

	newPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "crasher", Namespace: "kube-system"},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "main",
					RestartCount: 42,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
					},
				},
			},
		},
	}

	w.checkPod("staging", &corev1.Pod{ObjectMeta: newPod.ObjectMeta}, newPod)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0], "CrashLoopBackOff")
	assert.Contains(t, sent[0], "restarts: 42")
}

func TestPodWatcher_CheckPod_ImagePullBackOff(t *testing.T) {
	t.Parallel()

	sent := make([]string, 0)
	var mu sync.Mutex

	w := &PodWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
		cfg:        config.TelegramConfig{AdminIDs: []int64{111}},
		notifier: &fakeNotifier{
			sendFunc: func(_ int64, text string, _ interface{}) error {
				mu.Lock()
				sent = append(sent, text)
				mu.Unlock()
				return nil
			},
		},
		audit:  &nopAuditLogger{},
		logger: noopLogger(),
	}

	newPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "bad-image", Namespace: "default"},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "main",
					Image: "registry.example.com/app:v999",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"},
					},
				},
			},
		},
	}

	w.checkPod("prod", &corev1.Pod{ObjectMeta: newPod.ObjectMeta}, newPod)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0], "ImagePullBackOff")
	assert.Contains(t, sent[0], "registry.example.com/app:v999")
}

func TestPodWatcher_CheckPod_ErrImagePull(t *testing.T) {
	t.Parallel()

	sent := make([]string, 0)
	var mu sync.Mutex

	w := &PodWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
		cfg:        config.TelegramConfig{AdminIDs: []int64{111}},
		notifier: &fakeNotifier{
			sendFunc: func(_ int64, text string, _ interface{}) error {
				mu.Lock()
				sent = append(sent, text)
				mu.Unlock()
				return nil
			},
		},
		audit:  &nopAuditLogger{},
		logger: noopLogger(),
	}

	newPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "err-pull", Namespace: "default"},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "main",
					Image: "nonexistent:latest",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "ErrImagePull"},
					},
				},
			},
		},
	}

	w.checkPod("prod", &corev1.Pod{ObjectMeta: newPod.ObjectMeta}, newPod)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0], "ImagePullBackOff")
}

func TestPodWatcher_CheckPod_PendingTooLong(t *testing.T) {
	t.Parallel()

	sent := make([]string, 0)
	var mu sync.Mutex

	w := &PodWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
		cfg:        config.TelegramConfig{AdminIDs: []int64{111}},
		notifier: &fakeNotifier{
			sendFunc: func(_ int64, text string, _ interface{}) error {
				mu.Lock()
				sent = append(sent, text)
				mu.Unlock()
				return nil
			},
		},
		audit:  &nopAuditLogger{},
		logger: noopLogger(),
	}

	newPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "stuck-pod",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}

	w.checkPod("prod", &corev1.Pod{ObjectMeta: newPod.ObjectMeta}, newPod)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0], "PendingTooLong")
}

func TestPodWatcher_CheckPod_PendingShortDuration_NoAlert(t *testing.T) {
	t.Parallel()

	sent := make([]string, 0)
	var mu sync.Mutex

	w := &PodWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
		cfg:        config.TelegramConfig{AdminIDs: []int64{111}},
		notifier: &fakeNotifier{
			sendFunc: func(_ int64, text string, _ interface{}) error {
				mu.Lock()
				sent = append(sent, text)
				mu.Unlock()
				return nil
			},
		},
		audit:  &nopAuditLogger{},
		logger: noopLogger(),
	}

	// Pod created just now — should NOT trigger PendingTooLong.
	newPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "fresh-pod",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}

	w.checkPod("prod", &corev1.Pod{ObjectMeta: newPod.ObjectMeta}, newPod)

	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t, sent, "fresh pending pod should not trigger alert")
}

func TestPodWatcher_CheckPod_Evicted(t *testing.T) {
	t.Parallel()

	sent := make([]string, 0)
	var mu sync.Mutex

	w := &PodWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
		cfg:        config.TelegramConfig{AdminIDs: []int64{111}},
		notifier: &fakeNotifier{
			sendFunc: func(_ int64, text string, _ interface{}) error {
				mu.Lock()
				sent = append(sent, text)
				mu.Unlock()
				return nil
			},
		},
		audit:  &nopAuditLogger{},
		logger: noopLogger(),
	}

	oldPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "evicted-pod", Namespace: "default"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	newPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "evicted-pod", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase:   corev1.PodFailed,
			Reason:  "Evicted",
			Message: "The node was low on resource: memory.",
		},
	}

	w.checkPod("prod", oldPod, newPod)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0], "Evicted")
	assert.Contains(t, sent[0], "low on resource")
}

func TestPodWatcher_CheckPod_EvictedButAlreadyFailed_NoAlert(t *testing.T) {
	t.Parallel()

	sent := make([]string, 0)
	var mu sync.Mutex

	w := &PodWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
		cfg:        config.TelegramConfig{AdminIDs: []int64{111}},
		notifier: &fakeNotifier{
			sendFunc: func(_ int64, text string, _ interface{}) error {
				mu.Lock()
				sent = append(sent, text)
				mu.Unlock()
				return nil
			},
		},
		audit:  &nopAuditLogger{},
		logger: noopLogger(),
	}

	// Both old and new are Failed — should NOT re-alert.
	oldPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "evicted-pod", Namespace: "default"},
		Status:     corev1.PodStatus{Phase: corev1.PodFailed},
	}
	newPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "evicted-pod", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase:  corev1.PodFailed,
			Reason: "Evicted",
		},
	}

	w.checkPod("prod", oldPod, newPod)

	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t, sent, "already-failed pod should not re-alert on eviction")
}

func TestPodWatcher_CheckPod_HighRestarts(t *testing.T) {
	t.Parallel()

	sent := make([]string, 0)
	var mu sync.Mutex

	w := &PodWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
		cfg:        config.TelegramConfig{AdminIDs: []int64{111}},
		notifier: &fakeNotifier{
			sendFunc: func(_ int64, text string, _ interface{}) error {
				mu.Lock()
				sent = append(sent, text)
				mu.Unlock()
				return nil
			},
		},
		audit:  &nopAuditLogger{},
		logger: noopLogger(),
	}

	oldPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "flaky-pod", Namespace: "default"},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "app", RestartCount: 4, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
			},
		},
	}
	newPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "flaky-pod", Namespace: "default"},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "app", RestartCount: 6, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
			},
		},
	}

	w.checkPod("prod", oldPod, newPod)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0], "HighRestarts")
	assert.Contains(t, sent[0], "restart count: 6")
}

func TestPodWatcher_CheckPod_HighRestarts_SameCount_NoAlert(t *testing.T) {
	t.Parallel()

	sent := make([]string, 0)
	var mu sync.Mutex

	w := &PodWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
		cfg:        config.TelegramConfig{AdminIDs: []int64{111}},
		notifier: &fakeNotifier{
			sendFunc: func(_ int64, text string, _ interface{}) error {
				mu.Lock()
				sent = append(sent, text)
				mu.Unlock()
				return nil
			},
		},
		audit:  &nopAuditLogger{},
		logger: noopLogger(),
	}

	// Same restart count — should NOT alert.
	oldPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "flaky-pod", Namespace: "default"},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "app", RestartCount: 6, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
			},
		},
	}

	w.checkPod("prod", oldPod, oldPod)

	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t, sent, "same restart count should not trigger alert")
}

func TestPodWatcher_CheckPod_NoContainerStatuses_NoAlert(t *testing.T) {
	t.Parallel()

	sent := make([]string, 0)
	var mu sync.Mutex

	w := &PodWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
		cfg:        config.TelegramConfig{AdminIDs: []int64{111}},
		notifier: &fakeNotifier{
			sendFunc: func(_ int64, text string, _ interface{}) error {
				mu.Lock()
				sent = append(sent, text)
				mu.Unlock()
				return nil
			},
		},
		audit:  &nopAuditLogger{},
		logger: noopLogger(),
	}

	empty := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "init-pod", Namespace: "default"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}

	w.checkPod("prod", empty, empty)

	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t, sent, "pod with no container statuses should not alert")
}

func TestPodWatcher_CheckPod_Dedup_SecondCallSuppressed(t *testing.T) {
	t.Parallel()

	callCount := 0
	var mu sync.Mutex

	w := &PodWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
		cfg:        config.TelegramConfig{AdminIDs: []int64{111}},
		notifier: &fakeNotifier{
			sendFunc: func(_ int64, _ string, _ interface{}) error {
				mu.Lock()
				callCount++
				mu.Unlock()
				return nil
			},
		},
		audit:  &nopAuditLogger{},
		logger: noopLogger(),
	}

	newPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "crasher", Namespace: "default"},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "main",
					State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled"}},
				},
			},
		},
	}

	oldPod := &corev1.Pod{ObjectMeta: newPod.ObjectMeta}

	// First call — should send.
	w.checkPod("prod", oldPod, newPod)
	// Second call — should be deduped (within cooldown).
	w.checkPod("prod", oldPod, newPod)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, callCount, "second alert within cooldown should be suppressed")
}

// ─── NodeWatcher.checkNode edge cases ─────────────────────────────────────────

func TestNodeWatcher_CheckNode_ReadyToNotReady(t *testing.T) {
	t.Parallel()

	sent := make([]string, 0)
	var mu sync.Mutex

	w := &NodeWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
		cfg:        config.TelegramConfig{AdminIDs: []int64{111}},
		clusters:   &fakeClusterManager{},
		notifier: &fakeNotifier{
			sendFunc: func(_ int64, text string, _ interface{}) error {
				mu.Lock()
				sent = append(sent, text)
				mu.Unlock()
				return nil
			},
		},
		audit:  &nopAuditLogger{},
		logger: noopLogger(),
	}

	oldNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}
	newNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:               corev1.NodeReady,
					Status:             corev1.ConditionFalse,
					Reason:             "NodeNotReady",
					Message:            "kubelet stopped posting status",
					LastTransitionTime: metav1.NewTime(time.Now()),
				},
			},
		},
	}

	w.checkNode("prod-cluster", oldNode, newNode)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0], "NotReady")
	assert.Contains(t, sent[0], "Critical")
}

func TestNodeWatcher_CheckNode_ReadyToUnknown(t *testing.T) {
	t.Parallel()

	sent := make([]string, 0)
	var mu sync.Mutex

	w := &NodeWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
		cfg:        config.TelegramConfig{AdminIDs: []int64{111}},
		clusters:   &fakeClusterManager{},
		notifier: &fakeNotifier{
			sendFunc: func(_ int64, text string, _ interface{}) error {
				mu.Lock()
				sent = append(sent, text)
				mu.Unlock()
				return nil
			},
		},
		audit:  &nopAuditLogger{},
		logger: noopLogger(),
	}

	oldNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}
	newNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionUnknown, Reason: "NodeStatusUnknown"},
			},
		},
	}

	w.checkNode("prod", oldNode, newNode)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0], "Unreachable")
}

func TestNodeWatcher_CheckNode_DiskPressure(t *testing.T) {
	t.Parallel()

	sent := make([]string, 0)
	var mu sync.Mutex

	w := &NodeWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
		cfg:        config.TelegramConfig{AdminIDs: []int64{111}},
		clusters:   &fakeClusterManager{},
		notifier: &fakeNotifier{
			sendFunc: func(_ int64, text string, _ interface{}) error {
				mu.Lock()
				sent = append(sent, text)
				mu.Unlock()
				return nil
			},
		},
		audit:  &nopAuditLogger{},
		logger: noopLogger(),
	}

	oldNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-3"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
			},
		},
	}
	newNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-3"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue, Reason: "KubeletHasDiskPressure"},
			},
		},
	}

	w.checkNode("prod", oldNode, newNode)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0], "DiskPressure")
	assert.Contains(t, sent[0], "Warning")
}

func TestNodeWatcher_CheckNode_MemoryPressure(t *testing.T) {
	t.Parallel()

	sent := make([]string, 0)
	var mu sync.Mutex

	w := &NodeWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
		cfg:        config.TelegramConfig{AdminIDs: []int64{111}},
		clusters:   &fakeClusterManager{},
		notifier: &fakeNotifier{
			sendFunc: func(_ int64, text string, _ interface{}) error {
				mu.Lock()
				sent = append(sent, text)
				mu.Unlock()
				return nil
			},
		},
		audit:  &nopAuditLogger{},
		logger: noopLogger(),
	}

	oldNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-4"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
			},
		},
	}
	newNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-4"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue, Reason: "KubeletHasMemPressure"},
			},
		},
	}

	w.checkNode("prod", oldNode, newNode)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0], "MemoryPressure")
}

func TestNodeWatcher_CheckNode_PIDPressure(t *testing.T) {
	t.Parallel()

	sent := make([]string, 0)
	var mu sync.Mutex

	w := &NodeWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
		cfg:        config.TelegramConfig{AdminIDs: []int64{111}},
		clusters:   &fakeClusterManager{},
		notifier: &fakeNotifier{
			sendFunc: func(_ int64, text string, _ interface{}) error {
				mu.Lock()
				sent = append(sent, text)
				mu.Unlock()
				return nil
			},
		},
		audit:  &nopAuditLogger{},
		logger: noopLogger(),
	}

	oldNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-5"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse},
			},
		},
	}
	newNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-5"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionTrue, Reason: "KubeletHasPIDPressure"},
			},
		},
	}

	w.checkNode("prod", oldNode, newNode)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0], "PIDPressure")
}

func TestNodeWatcher_CheckNode_NoConditionChange_NoAlert(t *testing.T) {
	t.Parallel()

	sent := make([]string, 0)
	var mu sync.Mutex

	w := &NodeWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   5 * time.Minute,
		cfg:        config.TelegramConfig{AdminIDs: []int64{111}},
		clusters:   &fakeClusterManager{},
		notifier: &fakeNotifier{
			sendFunc: func(_ int64, text string, _ interface{}) error {
				mu.Lock()
				sent = append(sent, text)
				mu.Unlock()
				return nil
			},
		},
		audit:  &nopAuditLogger{},
		logger: noopLogger(),
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "healthy-node"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}

	w.checkNode("prod", node, node)

	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t, sent, "no condition change should not trigger alert")
}

// ─── CronJobWatcher edge cases ────────────────────────────────────────────────

func TestCronJobWatcher_isExcluded_EmptyList(t *testing.T) {
	t.Parallel()

	w := &CronJobWatcher{
		watcherCfg: CronJobWatcherConfig{ExcludeNamespaces: nil},
	}
	assert.False(t, w.isExcluded("kube-system"))
	assert.False(t, w.isExcluded(""))
}

func TestCronJobWatcher_isExcluded_CaseSensitive(t *testing.T) {
	t.Parallel()

	w := &CronJobWatcher{
		watcherCfg: CronJobWatcherConfig{ExcludeNamespaces: []string{"kube-system"}},
	}
	assert.True(t, w.isExcluded("kube-system"))
	assert.False(t, w.isExcluded("KUBE-SYSTEM"))
	assert.False(t, w.isExcluded("Kube-System"))
}

// ─── CertWatcher edge cases ──────────────────────────────────────────────────

func TestCertWatcher_isExcluded_EmptyList(t *testing.T) {
	t.Parallel()

	w := &CertWatcher{
		watcherCfg: CertWatcherConfig{ExcludeNamespaces: nil},
	}
	assert.False(t, w.isCertExcluded("kube-system"))
}

// ─── PVCWatcher edge cases ───────────────────────────────────────────────────

func TestPVCWatcher_isExcluded_EmptyList(t *testing.T) {
	t.Parallel()

	w := &PVCWatcher{
		watcherCfg: PVCWatcherConfig{ExcludeNamespaces: nil},
	}
	assert.False(t, w.isPVCExcluded("kube-system"))
}

func TestPVCWatcher_CustomThresholds(t *testing.T) {
	t.Parallel()

	w := NewPVCWatcher(nil, nil, nil, config.TelegramConfig{}, PVCWatcherConfig{
		WarningThreshold:  60.0,
		CriticalThreshold: 90.0,
		CheckInterval:     2 * time.Minute,
	}, nil)
	assert.Equal(t, 60.0, w.watcherCfg.WarningThreshold)
	assert.Equal(t, 90.0, w.watcherCfg.CriticalThreshold)
	assert.Equal(t, 2*time.Minute, w.watcherCfg.CheckInterval)
}

// ─── Module lifecycle ─────────────────────────────────────────────────────────

func TestModule_Name_And_Description(t *testing.T) {
	t.Parallel()

	m := &Module{}
	assert.Equal(t, "watcher", m.Name())
	assert.NotEmpty(t, m.Description())
}

func TestModule_Commands_IsNil(t *testing.T) {
	t.Parallel()

	m := &Module{}
	assert.Nil(t, m.Commands())
}

func TestModule_Health_DefaultHealthy(t *testing.T) {
	t.Parallel()

	m := &Module{healthy: true}
	assert.Equal(t, "healthy", string(m.Health()))
}

func TestModule_Health_Unhealthy(t *testing.T) {
	t.Parallel()

	m := &Module{healthy: false}
	assert.Equal(t, "unhealthy", string(m.Health()))
}

func TestModule_StopWatchers_NotRunning_NoOp(t *testing.T) {
	t.Parallel()

	m := &Module{running: false, logger: noopLogger()}
	// Should not panic.
	m.StopWatchers()
	assert.False(t, m.running)
}

func TestModule_StopWatchers_AlreadyStopped_Idempotent(t *testing.T) {
	t.Parallel()

	m := &Module{running: true, stopFunc: func() {}, logger: noopLogger()}
	m.StopWatchers()
	assert.False(t, m.running)
	// Second call should be a no-op.
	m.StopWatchers()
	assert.False(t, m.running)
}
