package i18n_test

import (
	"context"
	"testing"

	"github.com/d9042n/telekube/pkg/i18n"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_LoadsLocales(t *testing.T) {
	m, err := i18n.New(nil)
	require.NoError(t, err)
	require.NotNil(t, m)
}

func TestTranslator_EnglishKeys(t *testing.T) {
	m, err := i18n.New(nil)
	require.NoError(t, err)

	tr := m.For("en")
	assert.Equal(t, "✅ Confirm", tr.T("common.confirm"))
	assert.Equal(t, "🔄 Refresh", tr.T("common.refresh"))
}

func TestTranslator_VietnameseKeys(t *testing.T) {
	m, err := i18n.New(nil)
	require.NoError(t, err)

	tr := m.For("vi")
	assert.Equal(t, "✅ Xác nhận", tr.T("common.confirm"))
	assert.Equal(t, "🔄 Làm mới", tr.T("common.refresh"))
}

func TestTranslator_FormatArgs(t *testing.T) {
	m, err := i18n.New(nil)
	require.NoError(t, err)

	tr := m.For("en")
	result := tr.T("pods.list.title", "production", "prod-1")
	assert.Contains(t, result, "production")
	assert.Contains(t, result, "prod-1")
}

func TestTranslator_FallbackOnMissingLocale(t *testing.T) {
	m, err := i18n.New(nil)
	require.NoError(t, err)

	// Unknown locale falls back to English
	tr := m.For("fr")
	assert.Equal(t, "en", tr.Locale())
	assert.Equal(t, "✅ Confirm", tr.T("common.confirm"))
}

func TestTranslator_FallbackOnMissingKey(t *testing.T) {
	m, err := i18n.New(nil)
	require.NoError(t, err)

	// Missing key returns the key itself
	tr := m.For("en")
	result := tr.T("nonexistent.key.here")
	assert.Equal(t, "nonexistent.key.here", result)
}

func TestContext_WithAndFrom(t *testing.T) {
	m, err := i18n.New(nil)
	require.NoError(t, err)

	tr := m.For("vi")
	ctx := i18n.WithTranslator(context.Background(), tr)

	extracted := i18n.FromContext(ctx)
	require.NotNil(t, extracted)
	assert.Equal(t, "vi", extracted.Locale())
	assert.Equal(t, "✅ Xác nhận", extracted.T("common.confirm"))
}

func TestContext_FromContextNilReturnsNoop(t *testing.T) {
	// Empty context should return a safe no-op translator
	tr := i18n.FromContext(context.Background())
	require.NotNil(t, tr)
	// Should return the key itself for unknown keys
	result := tr.T("some.key")
	assert.Equal(t, "some.key", result)
}
