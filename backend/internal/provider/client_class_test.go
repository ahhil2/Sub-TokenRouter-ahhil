package provider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderClientClass(t *testing.T) {
	for _, platform := range []string{PlatformOpenAI, PlatformAnthropic, PlatformGemini} {
		for _, kind := range []string{ProviderTypeAPIKey, ProviderTypeOAuth, ProviderTypeSetupToken} {
			for _, class := range []string{ClientClassAny, ClientClassCodex, ClientClassNonCodex} {
				t.Run(platform+"/"+kind+"/"+class, func(t *testing.T) {
					record := &Record{Platform: platform, Type: kind, Extra: map[string]any{OpenAIClientClassExtraKey: class}}
					assert.Equal(t, platform != PlatformOpenAI || class != ClientClassNonCodex, record.AllowsClientClass(true))
					assert.Equal(t, platform != PlatformOpenAI || class != ClientClassCodex, record.AllowsClientClass(false))
				})
			}
		}
	}
	record := &Record{Platform: PlatformOpenAI}
	assert.True(t, record.AllowsClientClass(true))
	assert.True(t, record.AllowsClientClass(false))
	record.Extra = map[string]any{OpenAIClientClassExtraKey: "unknown"}
	assert.False(t, record.AllowsClientClass(true))
	assert.False(t, record.AllowsClientClass(false))
}

func TestValidateClientClassExtra(t *testing.T) {
	for _, value := range []any{nil, "", ClientClassAny, ClientClassCodex, ClientClassNonCodex} {
		require.NoError(t, ValidateClientClassExtra(map[string]any{OpenAIClientClassExtraKey: value}))
	}
	for _, value := range []any{true, 1, "both", "CODEX", []string{"codex"}} {
		extra := map[string]any{OpenAIClientClassExtraKey: value}
		require.Error(t, ValidateClientClassExtra(extra))
		_, err := BuildProviderForCreate(&CreateProviderInput{}, extra, CreationOptions{})
		require.Error(t, err)
		admin := &Admin{}
		_, err = admin.UpdateProvider(context.Background(), 1, &UpdateProviderInput{Extra: extra})
		require.Error(t, err)
		require.Error(t, admin.UpdateProviderExtra(context.Background(), 1, extra))
		_, err = admin.BulkUpdateProviders(context.Background(), &BulkUpdateProvidersInput{Extra: extra})
		require.Error(t, err)
	}
}
