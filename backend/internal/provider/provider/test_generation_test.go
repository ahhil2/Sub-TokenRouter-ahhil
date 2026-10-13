package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

type (
	generationLoader struct{ target generationTarget }
	generationTarget struct {
		execute func(context.Context, provider.TestEventSink) error
	}
)

func (loader generationLoader) LoadTestTarget(context.Context, provider.TestRequest) (provider.TestTarget, error) {
	return loader.target, nil
}

func (generationTarget) Information() provider.TestTargetInfo {
	return provider.TestTargetInfo{ProviderSnapshot: provider.ProviderSnapshot{Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeAPIKey}}
}

func (target generationTarget) Execute(ctx context.Context, _ provider.PreparedTestRequest, sink provider.TestEventSink) error {
	return target.execute(ctx, sink)
}

func TestGenerationProtocols(test *testing.T) {
	for _, scenario := range []struct {
		name, body string
		wantError  bool
	}{
		{"chat", `data: {"choices":[{"delta":{"content":"<svg></svg>"},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n", false},
		{"chat", `data: {"choices":[{"delta":{"content":"<svg></svg>"},"finish_reason":"length"}]}` + "\n", true},
		{"anthropic", `data: {"type":"message_delta","delta":{"stop_reason":"max_tokens"}}` + "\n", true},
		{"anthropic", `data: {"type":"content_block_delta","delta":{"text":"<svg></svg>"}}` + "\n", true},
		{"gemini", `data: {"candidates":[{"content":{"parts":[{"text":"<svg></svg>"}]},"finishReason":"MAX_TOKENS"}]}` + "\n", true},
		{"responses", `data: {"type":"response.incomplete"}` + "\n", true},
		{"responses", `data: {"type":"response.output_text.delta","delta":"<svg></svg>"}` + "\n" + `data: {"type":"response.completed"}` + "\n", false},
	} {
		loader := generationLoader{target: generationTarget{execute: func(ctx context.Context, sink provider.TestEventSink) error {
			run := NewTestRun(ctx, nil, sink)
			defer run.Cancel()
			payload := map[string]any{"max_tokens": 16}
			run.generationPayload(payload, "chat")
			require.Equal(test, 16384, payload["max_tokens"])
			body := strings.NewReader(scenario.body)
			output := TestStreamOutput{}
			var err error
			switch scenario.name {
			case "chat":
				err = output.ChatCompletions(run, body)
			case "anthropic":
				err = output.Anthropic(run, body)
			case "gemini":
				err = output.Gemini(run, body)
			default:
				err = output.Responses(run, body)
			}
			return run.Result(err)
		}}}
		_, err := provider.NewTestService(loader, provider.TestOptions{}).GenerateText(context.Background(), 1, "model", "prompt", 16384, 1<<20)
		if scenario.wantError {
			require.Error(test, err, scenario.name)
		} else {
			require.NoError(test, err, scenario.name)
		}
	}
	run := NewTestRun(context.Background(), nil, &generationBufferSink{})
	defer run.Cancel()
	payload := map[string]any{"max_tokens": 16}
	run.generationPayload(payload, "chat")
	require.Equal(test, 16, payload["max_tokens"])
}
