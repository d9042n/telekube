// Package keyboard provides inline keyboard building utilities.
package keyboard

import (
	"fmt"

	"github.com/d9042n/telekube/internal/entity"
	"gopkg.in/telebot.v3"
)

// Builder creates inline keyboards for common UI patterns.
type Builder struct {
	cbs *CallbackStore
}

// NewBuilder creates a new keyboard builder.
func NewBuilder() *Builder {
	return &Builder{cbs: NewCallbackStore()}
}

// Resolve resolves a potentially shortened callback data key back to the full string.
func (b *Builder) Resolve(key string) string {
	return b.cbs.Resolve(key)
}

// StoreData stores a full callback data string, returning a short key if it exceeds 64 bytes.
func (b *Builder) StoreData(data string) string {
	return b.cbs.Store(data)
}

// ClusterSelector creates an inline keyboard for cluster selection.
func (b *Builder) ClusterSelector(clusters []entity.ClusterInfo) *telebot.ReplyMarkup {
	menu := &telebot.ReplyMarkup{}

	var buttons []telebot.Btn
	for _, c := range clusters {
		label := fmt.Sprintf("%s %s", c.Status.Emoji(), c.DisplayName)
		btn := menu.Data(label, "cluster_select", c.Name)
		buttons = append(buttons, btn)
	}

	// Arrange in rows of 2
	var rows []telebot.Row
	for i := 0; i < len(buttons); i += 2 {
		if i+1 < len(buttons) {
			rows = append(rows, menu.Row(buttons[i], buttons[i+1]))
		} else {
			rows = append(rows, menu.Row(buttons[i]))
		}
	}

	menu.Inline(rows...)
	return menu
}

// NamespaceSelector creates an inline keyboard for namespace selection.
func (b *Builder) NamespaceSelector(namespaces []string, callbackPrefix string) *telebot.ReplyMarkup {
	menu := &telebot.ReplyMarkup{}

	var buttons []telebot.Btn
	// Add "All" option
	buttons = append(buttons, menu.Data("📋 All", fmt.Sprintf("%s_ns", callbackPrefix), "_all"))

	// Find longest namespace name to decide layout
	maxLen := 0
	for _, ns := range namespaces {
		if len(ns) > maxLen {
			maxLen = len(ns)
		}
	}

	for _, ns := range namespaces {
		// Truncate long names: keep the tail since the suffix
		// is the distinguishing part (e.g. "…prxshp-saleor-core")
		label := ns
		const maxBtnText = 30
		if len(label) > maxBtnText {
			label = "…" + label[len(label)-(maxBtnText-1):]
		}
		btn := menu.Data(label, fmt.Sprintf("%s_ns", callbackPrefix), b.cbs.Store(ns))
		buttons = append(buttons, btn)
	}

	// Choose columns based on longest name:
	//   > 25 chars → 1 per row (full width, no truncation)
	//   > 15 chars → 2 per row
	//   otherwise  → 3 per row
	cols := 3
	if maxLen > 25 {
		cols = 1
	} else if maxLen > 15 {
		cols = 2
	}

	var rows []telebot.Row
	for i := 0; i < len(buttons); i += cols {
		end := i + cols
		if end > len(buttons) {
			end = len(buttons)
		}
		rows = append(rows, menu.Row(buttons[i:end]...))
	}

	menu.Inline(rows...)
	return menu
}

// Confirmation creates a Yes/Cancel confirmation dialog.
func (b *Builder) Confirmation(callbackPrefix string, data string) *telebot.ReplyMarkup {
	menu := &telebot.ReplyMarkup{}
	storedData := b.cbs.Store(data)

	btnYes := menu.Data("✅ Confirm", fmt.Sprintf("%s_confirm", callbackPrefix), storedData)
	btnNo := menu.Data("❌ Cancel", fmt.Sprintf("%s_cancel", callbackPrefix), storedData)

	menu.Inline(menu.Row(btnYes, btnNo))
	return menu
}

// BackButton creates a single back button.
func (b *Builder) BackButton(callbackPrefix string) *telebot.ReplyMarkup {
	menu := &telebot.ReplyMarkup{}
	btn := menu.Data("◀️ Back", fmt.Sprintf("%s_back", callbackPrefix))
	menu.Inline(menu.Row(btn))
	return menu
}

// ActionRow creates a row of action buttons for a pod detail view.
func (b *Builder) PodActions(podName, namespace, clusterName string) *telebot.ReplyMarkup {
	menu := &telebot.ReplyMarkup{}
	data := b.cbs.Store(fmt.Sprintf("%s|%s|%s", podName, namespace, clusterName))
	backData := b.cbs.Store(namespace + "|" + clusterName)

	btnLogs := menu.Data("📋 Logs", "k8s_logs", data)
	btnEvents := menu.Data("🔍 Events", "k8s_events", data)
	btnRestart := menu.Data("🔄 Restart", "k8s_restart", data)
	btnBack := menu.Data("◀️ Back", "k8s_pods_back", backData)

	menu.Inline(
		menu.Row(btnLogs, btnEvents),
		menu.Row(btnRestart),
		menu.Row(btnBack),
	)
	return menu
}

// LogActions creates action buttons for a log view.
func (b *Builder) LogActions(podName, namespace, clusterName, container string, currentTail int) *telebot.ReplyMarkup {
	menu := &telebot.ReplyMarkup{}
	data4 := fmt.Sprintf("%s|%s|%s|%s", podName, namespace, clusterName, container)
	data3 := fmt.Sprintf("%s|%s|%s", podName, namespace, clusterName)

	var buttons []telebot.Btn

	// More lines options
	moreTails := []int{100, 200, 500}
	for _, tail := range moreTails {
		if tail > currentTail {
			btn := menu.Data(
				fmt.Sprintf("📋 %d lines", tail),
				"k8s_logs_more",
				b.cbs.Store(fmt.Sprintf("%s|%d", data4, tail)),
			)
			buttons = append(buttons, btn)
			break // Only show next option
		}
	}

	// Previous container logs
	btnPrev := menu.Data("🔄 Previous", "k8s_logs_prev", b.cbs.Store(data4))
	buttons = append(buttons, btnPrev)

	// Back button
	btnBack := menu.Data("◀️ Back", "k8s_pod_detail", b.cbs.Store(data3))

	var rows []telebot.Row
	rows = append(rows, menu.Row(buttons...))
	rows = append(rows, menu.Row(btnBack))

	menu.Inline(rows...)
	return menu
}
