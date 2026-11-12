package provider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// testing_generation_test.go checks bounded provider text collection and private
// request diagnostics. Event fixtures exercise output accumulation and byte limits,
// while routed requests retain their group identity and suppress persistent text
// excerpts through the execution context supplied to platform adapters.

// TestGenerationCollectsBoundedText supplies text chunks that fit or exceed the
// selected output budget. Successful content is retained and oversized output produces a failure.
func TestGenerationCollectsBoundedText(t *testing.T) {
	for _, limit := range []int{100, 8} {
		loader := &testLoaderStub{target: testTargetStub{info: TestTargetInfo{ProviderSnapshot: ProviderSnapshot{Platform: PlatformOpenAI, Type: ProviderTypeAPIKey}}, run: func(ctx context.Context, request PreparedTestRequest, sink TestEventSink) error {
			require.Equal(t, 16384, request.OutputTokens)
			require.Equal(t, 16384, TestOutputBudget(ctx))
			require.Equal(t, "exact prompt", request.Prompt)
			for _, event := range []TestEvent{{Type: "test_start", Model: "actual-model"}, {Type: "content", Text: "<svg>"}, {Type: "content", Text: "</svg>"}} {
				if err := sink.Emit(ctx, event); err != nil {
					return err
				}
			}
			return nil
		}}}
		result, err := NewTestService(loader, TestOptions{}).GenerateText(context.Background(), 1, "alias", "exact prompt", 16384, limit)
		if limit == 8 {
			require.Error(t, err)
			require.LessOrEqual(t, len(result.Text), 8)
		} else {
			require.NoError(t, err)
			require.Equal(t, "<svg></svg>", result.Text)
			require.Equal(t, "actual-model", result.Model)
		}
	}
	require.Zero(t, TestOutputBudget(context.Background()))
}

// TestGenerationRequestRetainsRoutingAndSuppressesDiagnostics passes a user text
// request through the normal test service. The loader receives group routing and validation errors avoid diagnostic callbacks.
func TestGenerationRequestRetainsRoutingAndSuppressesDiagnostics(t *testing.T) {
	diagnostics := 0
	loader := &testLoaderStub{target: testTargetStub{info: TestTargetInfo{ProviderSnapshot: ProviderSnapshot{Platform: PlatformOpenAI, Type: ProviderTypeAPIKey}}, run: func(ctx context.Context, request PreparedTestRequest, sink TestEventSink) error {
		require.Equal(t, int64(12), request.GroupID)
		require.Equal(t, "private scene", request.Prompt)
		require.NotContains(t, TextGenerationDiagnostic(ctx, "echoed private scene"), "private scene")
		return sink.Emit(ctx, TestEvent{Type: "content", Text: "expanded scene"})
	}}}
	service := NewTestService(loader, TestOptions{Error: func(string) { diagnostics++ }})
	got, err := service.GenerateTextRequest(t.Context(), TestRequest{ProviderID: 1, GroupID: 12, Model: "alias", Prompt: "private scene"}, 4096, 100)
	require.NoError(t, err)
	require.Equal(t, "expanded scene", got.Text)
	require.Equal(t, int64(12), loader.request.GroupID)
	_, err = service.GenerateTextRequest(t.Context(), TestRequest{Protocol: "invalid"}, 4096, 100)
	require.Error(t, err)
	require.Zero(t, diagnostics)
}
