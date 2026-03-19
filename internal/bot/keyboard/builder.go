// Package keyboard provides inline keyboard building utilities.
package keyboard

import (
	"fmt"

	"github.com/d9042n/telekube/internal/entity"
	"gopkg.in/telebot.v3"
)

// Builder creates inline keyboards for common UI patterns.
type Builder struct{}

// NewBuilder creates a new keyboard builder.
func NewBuilder() *Builder {
	return &Builder{}
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

	for _, ns := range namespaces {
		btn := menu.Data(ns, fmt.Sprintf("%s_ns", callbackPrefix), ns)
		buttons = append(buttons, btn)
	}

	// Arrange in rows of 3
	var rows []telebot.Row
	for i := 0; i < len(buttons); i += 3 {
		end := i + 3
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

	btnYes := menu.Data("✅ Confirm", fmt.Sprintf("%s_confirm", callbackPrefix), data)
	btnNo := menu.Data("❌ Cancel", fmt.Sprintf("%s_cancel", callbackPrefix), data)

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
	data := fmt.Sprintf("%s|%s|%s", podName, namespace, clusterName)

	btnLogs := menu.Data("📋 Logs", "k8s_logs", data)
	btnEvents := menu.Data("🔍 Events", "k8s_events", data)
	btnRestart := menu.Data("🔄 Restart", "k8s_restart", data)
	btnBack := menu.Data("◀️ Back", "k8s_pods_back", namespace+"|"+clusterName)

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
	data := fmt.Sprintf("%s|%s|%s|%s", podName, namespace, clusterName, container)

	var buttons []telebot.Btn

	// More lines options
	moreTails := []int{100, 200, 500}
	for _, tail := range moreTails {
		if tail > currentTail {
			btn := menu.Data(
				fmt.Sprintf("📋 %d lines", tail),
				"k8s_logs_more",
				fmt.Sprintf("%s|%d", data, tail),
			)
			buttons = append(buttons, btn)
			break // Only show next option
		}
	}

	// Previous container logs
	btnPrev := menu.Data("🔄 Previous", "k8s_logs_prev", data)
	buttons = append(buttons, btnPrev)

	// Back button
	btnBack := menu.Data("◀️ Back", "k8s_pod_detail", fmt.Sprintf("%s|%s|%s", podName, namespace, clusterName))

	var rows []telebot.Row
	rows = append(rows, menu.Row(buttons...))
	rows = append(rows, menu.Row(btnBack))

	menu.Inline(rows...)
	return menu
}
