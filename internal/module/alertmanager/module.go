// Package alertmanager implements the Prometheus AlertManager webhook receiver module.
package alertmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/module"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

// AlertManagerPayload is the JSON body sent by Prometheus AlertManager.
type AlertManagerPayload struct {
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	Status            string            `json:"status"` // "firing" | "resolved"
	Receiver          string            `json:"receiver"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
	Alerts            []Alert           `json:"alerts"`
}

// Alert represents a single alert within the payload.
type Alert struct {
	Status       string            `json:"status"` // "firing" | "resolved"
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
}

// Notifier sends messages to Telegram.
type Notifier interface {
	SendAlert(chatID int64, text string, markup *telebot.ReplyMarkup) error
}

// Silencer interacts with the AlertManager API to create/remove silences.
type Silencer struct {
	alertManagerURL string
	token           string
	client          *http.Client
}

// NewSilencer creates a Silencer.
func NewSilencer(alertManagerURL, token string) *Silencer {
	return &Silencer{
		alertManagerURL: alertManagerURL,
		token:           token,
		client:          &http.Client{Timeout: 10 * time.Second},
	}
}

type silenceRequest struct {
	Matchers  []silenceMatcher `json:"matchers"`
	StartsAt  time.Time        `json:"startsAt"`
	EndsAt    time.Time        `json:"endsAt"`
	CreatedBy string           `json:"createdBy"`
	Comment   string           `json:"comment"`
}

type silenceMatcher struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"isRegex"`
}

// Silence creates a silence in AlertManager.
func (s *Silencer) Silence(labels map[string]string, duration time.Duration, createdBy string) (string, error) {
	matchers := make([]silenceMatcher, 0, len(labels))
	for k, v := range labels {
		matchers = append(matchers, silenceMatcher{Name: k, Value: v})
	}

	req := silenceRequest{
		Matchers:  matchers,
		StartsAt:  time.Now().UTC(),
		EndsAt:    time.Now().Add(duration).UTC(),
		CreatedBy: createdBy,
		Comment:   fmt.Sprintf("Silenced via Telekube for %s", duration),
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/v2/silences", s.alertManagerURL),
		strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if s.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("alertmanager returned %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		SilenceID string `json:"silenceID"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.SilenceID, nil
}

// Module implements the Prometheus AlertManager webhook receiver.
type Module struct {
	token    string
	chats    []int64
	notifier Notifier
	silencer *Silencer
	logger   *zap.Logger
}

// NewModule creates an AlertManager module.
func NewModule(token string, chats []int64, notifier Notifier, silencer *Silencer, logger *zap.Logger) *Module {
	return &Module{
		token:    token,
		chats:    chats,
		notifier: notifier,
		silencer: silencer,
		logger:   logger,
	}
}

func (m *Module) Name() string        { return "alertmanager" }
func (m *Module) Description() string { return "Prometheus AlertManager webhook receiver" }

func (m *Module) Register(bot *telebot.Bot, _ *telebot.Group) {
	bot.Handle(&telebot.Btn{Unique: "am_silence_1h"}, m.handleSilence1h)
	bot.Handle(&telebot.Btn{Unique: "am_silence_4h"}, m.handleSilence4h)
	bot.Handle(&telebot.Btn{Unique: "am_ack"}, m.handleAck)
}

func (m *Module) Start(_ context.Context) error {
	m.logger.Info("alertmanager module started")
	return nil
}

func (m *Module) Stop(_ context.Context) error { return nil }

func (m *Module) Health() entity.HealthStatus { return entity.HealthStatusHealthy }

func (m *Module) Commands() []module.CommandInfo { return nil }

// WebhookHandler returns an http.Handler that processes AlertManager webhook POSTs.
func (m *Module) WebhookHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Validate bearer token
		if m.token != "" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+m.token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}

		// 2. Parse payload
		defer r.Body.Close()
		var payload AlertManagerPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		// 3. Send to configured chats
		m.processPayload(payload)
		w.WriteHeader(http.StatusOK)
	})
}

func (m *Module) processPayload(payload AlertManagerPayload) {
	if len(payload.Alerts) == 0 {
		return
	}

	// Sort: critical first
	sort.Slice(payload.Alerts, func(i, j int) bool {
		return severityOrder(payload.Alerts[i].Labels["severity"]) <
			severityOrder(payload.Alerts[j].Labels["severity"])
	})

	for _, alert := range payload.Alerts {
		text := formatAlert(alert)
		kbd := m.alertKeyboard(alert)

		for _, chatID := range m.chats {
			if err := m.notifier.SendAlert(chatID, text, kbd); err != nil {
				m.logger.Error("sending alert to chat",
					zap.Int64("chat_id", chatID),
					zap.Error(err))
			}
		}
	}
}

func severityOrder(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}

func formatAlert(a Alert) string {
	var sb strings.Builder

	if a.Status == "firing" {
		sev := a.Labels["severity"]
		emoji := severityEmoji(sev)
		sb.WriteString(fmt.Sprintf("🔥 ALERT FIRING — %s\n", a.Labels["alertname"]))
		sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
		sb.WriteString(fmt.Sprintf("Severity:  %s %s\n", emoji, sev))
		if ns := a.Labels["namespace"]; ns != "" {
			sb.WriteString(fmt.Sprintf("Namespace: %s\n", ns))
		}
		if pod := a.Labels["pod"]; pod != "" {
			sb.WriteString(fmt.Sprintf("Pod:       %s\n", pod))
		}
		if summary := a.Annotations["summary"]; summary != "" {
			sb.WriteString(fmt.Sprintf("\nSummary: %s\n", summary))
		}
		if description := a.Annotations["description"]; description != "" {
			sb.WriteString(fmt.Sprintf("Details: %s\n", description))
		}
		since := time.Since(a.StartsAt).Round(time.Minute)
		sb.WriteString(fmt.Sprintf("\nStarted: %s (%s ago)\n",
			a.StartsAt.UTC().Format("2006-01-02 15:04:05 UTC"), since))
	} else {
		sb.WriteString(fmt.Sprintf("✅ RESOLVED — %s\n", a.Labels["alertname"]))
		sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
		sev := a.Labels["severity"]
		sb.WriteString(fmt.Sprintf("Previous: %s %s\n", severityEmoji(sev), sev))
		if pod := a.Labels["pod"]; pod != "" {
			sb.WriteString(fmt.Sprintf("Pod:      %s\n", pod))
		}
		if !a.EndsAt.IsZero() && !a.StartsAt.IsZero() {
			dur := a.EndsAt.Sub(a.StartsAt).Round(time.Minute)
			sb.WriteString(fmt.Sprintf("Duration: %s\n", dur))
		}
	}

	return sb.String()
}

func severityEmoji(sev string) string {
	switch sev {
	case "critical":
		return "🔴"
	case "warning":
		return "🟡"
	default:
		return "⚪"
	}
}

func (m *Module) alertKeyboard(a Alert) *telebot.ReplyMarkup {
	menu := &telebot.ReplyMarkup{}
	if a.Status == "firing" {
		data := a.Fingerprint
		silence1h := menu.Data("🔇 Silence 1h", "am_silence_1h", data)
		silence4h := menu.Data("🔇 Silence 4h", "am_silence_4h", data)
		ack := menu.Data("✅ Ack", "am_ack", data)
		menu.Inline(menu.Row(silence1h, silence4h, ack))
	}
	return menu
}

func (m *Module) handleSilence1h(c telebot.Context) error {
	return m.doSilence(c, time.Hour)
}

func (m *Module) handleSilence4h(c telebot.Context) error {
	return m.doSilence(c, 4*time.Hour)
}

func (m *Module) doSilence(c telebot.Context, duration time.Duration) error {
	// In a real integration we'd look up the alert by fingerprint and get its labels.
	// Here we create a basic silence using the fingerprint as a label.
	if m.silencer == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "AlertManager silences not configured"})
	}

	createdBy := c.Sender().Username
	if createdBy == "" {
		createdBy = fmt.Sprintf("user_%d", c.Sender().ID)
	}

	labels := map[string]string{"fingerprint": c.Callback().Data}
	silenceID, err := m.silencer.Silence(labels, duration, createdBy)
	if err != nil {
		m.logger.Error("creating silence", zap.Error(err))
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Failed to create silence: " + err.Error()})
	}

	return c.Respond(&telebot.CallbackResponse{
		Text: fmt.Sprintf("🔇 Alert silenced for %s (ID: %s)", duration, silenceID),
	})
}

func (m *Module) handleAck(c telebot.Context) error {
	return c.Respond(&telebot.CallbackResponse{Text: "✅ Acknowledged"})
}
