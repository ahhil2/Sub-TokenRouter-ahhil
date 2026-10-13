package provider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

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
