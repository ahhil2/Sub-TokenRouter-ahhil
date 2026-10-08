package mediaentry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/apikey"
	"github.com/TokenFlux/TokenRouter/internal/gateway/media"
	gatewayadapter "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/scheduler"
)

// TestImagesRetryKeepsMixedCandidatePool 验证先选 Grok 后重试仍使用混合选号，执行器随本次提供商变化。
func TestImagesRetryKeepsMixedCandidatePool(t *testing.T) {
	groupID := int64(12)
	calls := 0
	runtime := &Runtime{bindings: Bindings{Platform: PlatformPorts{SelectImages: func(_ context.Context, group *int64, _ string, model string, excluded map[int64]struct{}, _ provider.OpenAIImagesCapability) (*gatewayadapter.SelectionResult, scheduler.PlatformDecision, error) {
		calls++
		require.Equal(t, groupID, *group)
		require.Equal(t, "public-image", model)
		platform := "grok"
		if calls == 2 {
			_, ok := excluded[1]
			require.True(t, ok)
			platform = "openai"
		}
		return &gatewayadapter.SelectionResult{Provider: gatewayadapter.NewExecutionProvider(&provider.Record{ID: int64(calls), Platform: platform}), Acquired: true}, scheduler.PlatformDecision{}, nil
	}}}}
	adapter := &generationRequestAdapter{h: runtime, apiKey: &apikey.APIKey{GroupID: &groupID}, requestModel: "public-image", parsed: &media.ImageRequest{RequiredCapability: media.ImageCapabilityBasic}}
	_, ok, err := adapter.SelectGeneration(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, adapter.usesGrok())
	_, ok, err = adapter.SelectGeneration(context.Background(), map[int64]struct{}{1: {}})
	require.NoError(t, err)
	require.True(t, ok)
	require.False(t, adapter.usesGrok())
	require.Equal(t, 2, calls)
}
