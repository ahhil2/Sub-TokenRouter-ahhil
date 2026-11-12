package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/protocol"
	"github.com/TokenFlux/TokenRouter/internal/routing"
)

// creative_prompt_test.go verifies text-route selection for configured prompt models.
// A group can admit multiple client protocols while its providers support a
// subset. The selector fixture counts attempts so reaching a usable route does
// not accidentally imply multiple generation requests or ignore request cancellation.

// TestCreativePromptSelectsAvailableTextProtocol rejects early client entries
// for an Anthropic route. Selection reaches Messages and returns its provider
// after the unavailable Chat and Responses entries, with no later selection attempts.
func TestCreativePromptSelectsAvailableTextProtocol(t *testing.T) {
	group := &routing.Group{ID: 12, AllowedProtocols: []protocol.ProtocolID{protocol.ProtocolOpenAIChatCompletions, protocol.ProtocolOpenAIResponses, protocol.ProtocolAnthropicMessages}}
	var tried []protocol.ProtocolID
	target, err := selectCreativePromptTarget(t.Context(), group, "model", func(_ context.Context, due routing.GroupAvailabilityProbeDueGroup, model string) (routing.GroupProbeTarget, error) {
		require.Equal(t, "model", model)
		tried = append(tried, due.Config.Protocol)
		if due.Config.Protocol == protocol.ProtocolAnthropicMessages {
			return routing.GroupProbeTarget{ProviderID: 3, GroupID: 12, ModelID: model}, nil
		}
		return routing.GroupProbeTarget{}, errors.New("no compatible provider")
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), target.ProviderID)
	require.Equal(t, []protocol.ProtocolID{protocol.ProtocolOpenAIChatCompletions, protocol.ProtocolOpenAIResponses, protocol.ProtocolAnthropicMessages}, tried)
}

// TestCreativePromptSelectionHonorsCancellation cancels before provider selection.
// The callback must remain unused and the caller receives the original
// cancellation before an enabled group entry can consult its provider candidates.
func TestCreativePromptSelectionHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := selectCreativePromptTarget(ctx, &routing.Group{ID: 12, AllowedProtocols: []protocol.ProtocolID{protocol.ProtocolOpenAIChatCompletions, protocol.ProtocolOpenAIResponses, protocol.ProtocolAnthropicMessages}}, "model", func(context.Context, routing.GroupAvailabilityProbeDueGroup, string) (routing.GroupProbeTarget, error) {
		t.Fatal("selection called after cancellation")
		return routing.GroupProbeTarget{}, nil
	})
	require.ErrorIs(t, err, context.Canceled)
}
