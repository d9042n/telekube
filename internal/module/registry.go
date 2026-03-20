package module

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

// Registry manages module lifecycle.
type Registry struct {
	modules map[string]Module
	order   []string
	logger  *zap.Logger
	mu      sync.RWMutex
}

// NewRegistry creates a new module registry.
func NewRegistry(logger *zap.Logger) *Registry {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Registry{
		modules: make(map[string]Module),
		logger:  logger,
	}
}

// Register adds a module to the registry.
func (r *Registry) Register(m Module) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := m.Name()
	if _, exists := r.modules[name]; exists {
		return fmt.Errorf("module %q already registered", name)
	}

	r.modules[name] = m
	r.order = append(r.order, name)

	r.logger.Info("module registered",
		zap.String("module", name),
		zap.String("description", m.Description()),
		zap.Int("commands", len(m.Commands())),
	)

	return nil
}

// RegisterAll registers all modules' commands with the bot.
func (r *Registry) RegisterAll(bot *telebot.Bot) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, name := range r.order {
		m := r.modules[name]
		group := bot.Group()
		m.Register(bot, group)

		r.logger.Info("module commands registered",
			zap.String("module", name),
		)
	}
}

// StartAll starts all registered modules.
func (r *Registry) StartAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, name := range r.order {
		m := r.modules[name]
		if err := m.Start(ctx); err != nil {
			r.logger.Error("module failed to start",
				zap.String("module", name),
				zap.Error(err),
			)
			// Don't block other modules
			continue
		}
		r.logger.Info("module started",
			zap.String("module", name),
		)
	}
	return nil
}

// StopAll stops all modules in reverse order.
func (r *Registry) StopAll(ctx context.Context) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Stop in reverse order
	for i := len(r.order) - 1; i >= 0; i-- {
		name := r.order[i]
		m := r.modules[name]

		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := m.Stop(stopCtx); err != nil {
			r.logger.Error("module failed to stop",
				zap.String("module", name),
				zap.Error(err),
			)
		} else {
			r.logger.Info("module stopped",
				zap.String("module", name),
			)
		}
		cancel()
	}
}

// Get returns a module by name.
func (r *Registry) Get(name string) (Module, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.modules[name]
	return m, ok
}

// AllCommands returns all commands from all registered modules.
func (r *Registry) AllCommands() []CommandInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var cmds []CommandInfo
	for _, name := range r.order {
		m := r.modules[name]
		cmds = append(cmds, m.Commands()...)
	}
	return cmds
}

// ModuleCommands pairs a module name with its commands.
type ModuleCommands struct {
	Name        string
	Description string
	Commands    []CommandInfo
}

// ModulesWithCommands returns each module's commands in registration order.
func (r *Registry) ModulesWithCommands() []ModuleCommands {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []ModuleCommands
	for _, name := range r.order {
		m := r.modules[name]
		cmds := m.Commands()
		if len(cmds) > 0 {
			result = append(result, ModuleCommands{
				Name:        name,
				Description: m.Description(),
				Commands:    cmds,
			})
		}
	}
	return result
}

// HealthAll returns health status of all modules.
func (r *Registry) HealthAll() map[string]entity.HealthStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	statuses := make(map[string]entity.HealthStatus, len(r.modules))
	for name, m := range r.modules {
		statuses[name] = m.Health()
	}
	return statuses
}

// Names returns all registered module names in order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, len(r.order))
	copy(names, r.order)
	return names
}
