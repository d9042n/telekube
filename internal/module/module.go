// Package module defines the module system for pluggable features.
package module

import (
	"context"

	"github.com/d9042n/telekube/internal/entity"
	"gopkg.in/telebot.v3"
)

// Module is the interface that all Telekube feature modules must implement.
type Module interface {
	// Name returns the module identifier (e.g., "kubernetes", "argocd").
	Name() string

	// Description returns a human-readable description.
	Description() string

	// Register registers commands and callback handlers with the bot.
	Register(bot *telebot.Bot, group *telebot.Group)

	// Start begins background workers (watchers, schedulers).
	// Called only after all modules are registered.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the module.
	Stop(ctx context.Context) error

	// Health returns the module's health status.
	Health() entity.HealthStatus

	// Commands returns the list of commands this module provides.
	// Used by /help to build dynamic help text.
	Commands() []CommandInfo
}

// CommandInfo describes a command provided by a module.
type CommandInfo struct {
	Command     string // e.g. "/pods"
	Description string // e.g. "List pods in a namespace"
	Permission  string // e.g. "kubernetes.pods.list"
	ChatType    string // "all", "private", "group"
}
