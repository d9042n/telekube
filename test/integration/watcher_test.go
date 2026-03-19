//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/d9042n/telekube/pkg/i18n"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestI18n_FullRoundTrip verifies the full i18n round trip in integration context.
func TestI18n_FullRoundTrip(t *testing.T) {
	t.Parallel()

	logger, _ := zap.NewDevelopment()
	m, err := i18n.New(logger)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// English
	enTr := m.For("en")
	ctx = i18n.WithTranslator(ctx, enTr)

	tr := i18n.FromContext(ctx)
	assert.Equal(t, "en", tr.Locale())
	assert.Equal(t, "✅ Confirm", tr.T("common.confirm"))

	// Vietnamese
	viTr := m.For("vi")
	ctx = i18n.WithTranslator(ctx, viTr)

	tr = i18n.FromContext(ctx)
	assert.Equal(t, "vi", tr.Locale())
	assert.Equal(t, "✅ Xác nhận", tr.T("common.confirm"))
}

// TestI18n_AllRequiredKeysPresent verifies all modules have both EN and VI translations.
func TestI18n_AllRequiredKeysPresent(t *testing.T) {
	t.Parallel()

	m, err := i18n.New(nil)
	require.NoError(t, err)

	requiredKeys := []string{
		"common.confirm",
		"common.cancel",
		"common.denied",
		"pods.list.title",
		"pods.restart.confirm",
		"pods.restart.success",
		"nodes.list.title",
		"audit.title",
		"rbac.denied",
	}

	for _, locale := range []string{"en", "vi"} {
		tr := m.For(locale)
		for _, key := range requiredKeys {
			result := tr.T(key)
			assert.NotEqual(t, key, result, "locale %s is missing key %s", locale, key)
		}
	}
}
