package alertmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

// mockNotifier captures alerts sent through the notifier.
type mockNotifier struct {
	alerts []sentAlert
}

type sentAlert struct {
	chatID int64
	text   string
	markup *telebot.ReplyMarkup
}

func (m *mockNotifier) SendAlert(chatID int64, text string, markup *telebot.ReplyMarkup) error {
	m.alerts = append(m.alerts, sentAlert{chatID: chatID, text: text, markup: markup})
	return nil
}

func newTestModule(t *testing.T) (*Module, *mockNotifier) {
	t.Helper()
	notifier := &mockNotifier{}
	logger := zap.NewNop()
	m := NewModule("test-token", []int64{123, 456}, notifier, nil, logger)
	return m, notifier
}

// --- WebhookHandler tests ---

func TestWebhookHandler_ValidPayload(t *testing.T) {
	m, notifier := newTestModule(t)

	payload := AlertManagerPayload{
		Version: "4",
		Status:  "firing",
		Alerts: []Alert{
			{
				Status:      "firing",
				Labels:      map[string]string{"alertname": "HighMemory", "severity": "warning", "namespace": "production"},
				Annotations: map[string]string{"summary": "Memory usage above 90%"},
				StartsAt:    time.Now().Add(-5 * time.Minute),
				Fingerprint: "abc123",
			},
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()

	m.WebhookHandler().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Len(t, notifier.alerts, 2) // 2 chats (123, 456)
	assert.Contains(t, notifier.alerts[0].text, "HighMemory")
	assert.Contains(t, notifier.alerts[0].text, "ALERT FIRING")
}

func TestWebhookHandler_InvalidToken(t *testing.T) {
	m, notifier := newTestModule(t)

	payload := AlertManagerPayload{Alerts: []Alert{{Status: "firing", Labels: map[string]string{"alertname": "Test"}}}}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer wrong-token")
	rr := httptest.NewRecorder()

	m.WebhookHandler().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Empty(t, notifier.alerts)
}

func TestWebhookHandler_NoToken(t *testing.T) {
	// Module with no token should accept all requests
	notifier := &mockNotifier{}
	m := NewModule("", []int64{100}, notifier, nil, zap.NewNop())

	payload := AlertManagerPayload{Alerts: []Alert{{Status: "firing", Labels: map[string]string{"alertname": "Test", "severity": "critical"}}}}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.WebhookHandler().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Len(t, notifier.alerts, 1)
}

func TestWebhookHandler_InvalidJSON(t *testing.T) {
	m, _ := newTestModule(t)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()

	m.WebhookHandler().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestWebhookHandler_EmptyAlerts(t *testing.T) {
	m, notifier := newTestModule(t)

	payload := AlertManagerPayload{Alerts: []Alert{}}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()

	m.WebhookHandler().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, notifier.alerts)
}

// --- Alert formatting tests ---

func TestFormatAlert_Firing(t *testing.T) {
	alert := Alert{
		Status: "firing",
		Labels: map[string]string{
			"alertname": "PodCrashLooping",
			"severity":  "critical",
			"namespace": "production",
			"pod":       "api-server-xyz",
		},
		Annotations: map[string]string{
			"summary":     "Pod is crash looping",
			"description": "Pod has been restarting repeatedly",
		},
		StartsAt: time.Now().Add(-10 * time.Minute),
	}

	text := formatAlert(alert)

	assert.Contains(t, text, "ALERT FIRING")
	assert.Contains(t, text, "PodCrashLooping")
	assert.Contains(t, text, "🔴") // critical emoji
	assert.Contains(t, text, "production")
	assert.Contains(t, text, "api-server-xyz")
	assert.Contains(t, text, "Pod is crash looping")
}

func TestFormatAlert_Resolved(t *testing.T) {
	alert := Alert{
		Status: "resolved",
		Labels: map[string]string{
			"alertname": "HighCPU",
			"severity":  "warning",
			"pod":       "worker-abc",
		},
		StartsAt: time.Now().Add(-30 * time.Minute),
		EndsAt:   time.Now(),
	}

	text := formatAlert(alert)

	assert.Contains(t, text, "RESOLVED")
	assert.Contains(t, text, "HighCPU")
	assert.Contains(t, text, "worker-abc")
}

// --- Severity sorting tests ---

func TestSeverityOrder(t *testing.T) {
	tests := []struct {
		severity string
		expected int
	}{
		{"critical", 0},
		{"warning", 1},
		{"info", 2},
		{"", 2},
	}
	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			assert.Equal(t, tt.expected, severityOrder(tt.severity))
		})
	}
}

func TestSeverityEmoji(t *testing.T) {
	assert.Equal(t, "🔴", severityEmoji("critical"))
	assert.Equal(t, "🟡", severityEmoji("warning"))
	assert.Equal(t, "⚪", severityEmoji("info"))
	assert.Equal(t, "⚪", severityEmoji(""))
}

// --- Critical-first sorting test ---

func TestProcessPayload_CriticalFirst(t *testing.T) {
	notifier := &mockNotifier{}
	m := NewModule("", []int64{100}, notifier, nil, zap.NewNop())

	payload := AlertManagerPayload{
		Alerts: []Alert{
			{Status: "firing", Labels: map[string]string{"alertname": "Low", "severity": "info"}, Fingerprint: "f1"},
			{Status: "firing", Labels: map[string]string{"alertname": "High", "severity": "critical"}, Fingerprint: "f2"},
			{Status: "firing", Labels: map[string]string{"alertname": "Mid", "severity": "warning"}, Fingerprint: "f3"},
		},
	}

	m.processPayload(payload)

	require.Len(t, notifier.alerts, 3)
	// First alert should be critical
	assert.Contains(t, notifier.alerts[0].text, "High")
	// Second should be warning
	assert.Contains(t, notifier.alerts[1].text, "Mid")
	// Third should be info
	assert.Contains(t, notifier.alerts[2].text, "Low")
}

// --- Module interface tests ---

func TestModule_Interface(t *testing.T) {
	m, _ := newTestModule(t)

	assert.Equal(t, "alertmanager", m.Name())
	assert.Equal(t, "Prometheus AlertManager webhook receiver", m.Description())
	assert.Nil(t, m.Commands())
}

func TestModule_Health(t *testing.T) {
	m, _ := newTestModule(t)
	assert.Equal(t, "healthy", string(m.Health()))
}

func TestModule_StartStop(t *testing.T) {
	m, _ := newTestModule(t)
	require.NoError(t, m.Start(context.TODO()))
	require.NoError(t, m.Stop(context.TODO()))
}
