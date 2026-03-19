// Package i18n provides internationalization support for Telekube.
// It supports multiple locales with YAML-based translation files and
// per-user locale preferences.
package i18n

import (
	"context"
	"embed"
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

//go:embed locales/*.yaml
var localeFS embed.FS

// defaultLocale is the fallback locale used when a key is missing.
const defaultLocale = "en"

// Translator provides translation lookup for a specific locale.
type Translator interface {
	// T translates a dot-notation key with optional format args.
	// Example: T("pods.list.title", "production", "prod-1")
	T(key string, args ...interface{}) string

	// Locale returns the current locale code.
	Locale() string
}

// contextKey is the key used to store the translator in a context.
type contextKey struct{}

// Manager loads translation files and creates locale-specific translators.
type Manager struct {
	mu      sync.RWMutex
	locales map[string]map[string]interface{}
	logger  *zap.Logger
}

// New creates a Manager and loads all embedded locale files.
func New(logger *zap.Logger) (*Manager, error) {
	m := &Manager{
		locales: make(map[string]map[string]interface{}),
		logger:  logger,
	}

	entries, err := localeFS.ReadDir("locales")
	if err != nil {
		return nil, fmt.Errorf("reading locales dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		localeName := strings.TrimSuffix(entry.Name(), ".yaml")
		data, err := localeFS.ReadFile("locales/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("reading locale %s: %w", entry.Name(), err)
		}

		var translations map[string]interface{}
		if err := yaml.Unmarshal(data, &translations); err != nil {
			return nil, fmt.Errorf("parsing locale %s: %w", entry.Name(), err)
		}

		m.mu.Lock()
		m.locales[localeName] = translations
		m.mu.Unlock()

		if logger != nil {
			logger.Info("loaded locale", zap.String("locale", localeName))
		}
	}

	return m, nil
}

// For returns a Translator for the given locale.
// Falls back to English if the locale is not found.
func (m *Manager) For(locale string) Translator {
	m.mu.RLock()
	translations, ok := m.locales[locale]
	m.mu.RUnlock()

	if !ok {
		// Fallback to default locale
		m.mu.RLock()
		translations = m.locales[defaultLocale]
		m.mu.RUnlock()
		locale = defaultLocale
	}

	var fallback map[string]interface{}
	if locale != defaultLocale {
		m.mu.RLock()
		fallback = m.locales[defaultLocale]
		m.mu.RUnlock()
	}

	return &translator{
		locale:       locale,
		translations: translations,
		fallback:     fallback,
	}
}

// WithTranslator returns a new context with the given translator.
func WithTranslator(ctx context.Context, t Translator) context.Context {
	return context.WithValue(ctx, contextKey{}, t)
}

// FromContext extracts the Translator from a context.
// Returns a no-op English translator if none is found.
func FromContext(ctx context.Context) Translator {
	if t, ok := ctx.Value(contextKey{}).(Translator); ok {
		return t
	}
	return &noopTranslator{}
}

// translator is the concrete Translator implementation.
type translator struct {
	locale       string
	translations map[string]interface{}
	fallback     map[string]interface{}
}

// Locale returns the current locale.
func (t *translator) Locale() string {
	return t.locale
}

// T translates a dot-notation key with optional format args.
func (t *translator) T(key string, args ...interface{}) string {
	// Try primary locale
	if val := lookupKey(t.translations, key); val != "" {
		return formatValue(val, args...)
	}

	// Try fallback locale
	if t.fallback != nil {
		if val := lookupKey(t.fallback, key); val != "" {
			return formatValue(val, args...)
		}
	}

	// Return the key itself as last resort
	return key
}

// lookupKey traverses nested maps using dot-notation.
func lookupKey(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}

	parts := strings.SplitN(key, ".", 2)
	if len(parts) == 0 {
		return ""
	}

	val, ok := m[parts[0]]
	if !ok {
		return ""
	}

	if len(parts) == 1 {
		// Leaf node
		if s, ok := val.(string); ok {
			return s
		}
		return ""
	}

	// Recurse into nested map
	nested, ok := val.(map[string]interface{})
	if !ok {
		return ""
	}
	return lookupKey(nested, parts[1])
}

// formatValue applies fmt.Sprintf if args are provided.
func formatValue(val string, args ...interface{}) string {
	if len(args) == 0 {
		return val
	}
	return fmt.Sprintf(val, args...)
}

// noopTranslator returns keys as-is (used as a safe fallback).
type noopTranslator struct{}

func (n *noopTranslator) T(key string, args ...interface{}) string {
	if len(args) == 0 {
		return key
	}
	return fmt.Sprintf(key, args...)
}

func (n *noopTranslator) Locale() string { return defaultLocale }
