package provider

import (
	"context"
	"errors"
	"strings"
	"sync"
)

// testing_generation.go collects bounded text from provider execution events.
// Callers choose an output-token budget and byte limit, and the sink rejects
// oversized content before retaining it. Routed user requests suppress diagnostic
// callbacks while preserving group and protocol metadata for transport validation.

type testOutputBudgetKey struct{}

// privateGenerationKey marks user text whose upstream diagnostics omit content.
type privateGenerationKey struct{}

type GeneratedTestText struct {
	Text  string
	Model string
}

type generationTestSink struct {
	mu      sync.Mutex
	text    strings.Builder
	model   string
	message string
	limit   int
}

func TestOutputBudget(ctx context.Context) int {
	value, _ := ctx.Value(testOutputBudgetKey{}).(int)
	return value
}

func (service *TestService) GenerateText(ctx context.Context, providerID int64, model, prompt string, outputTokens, maxBytes int) (GeneratedTestText, error) {
	sink := &generationTestSink{limit: maxBytes}
	kind := ProviderTestTypeText
	err := service.Test(ctx, TestRequest{ProviderID: providerID, Model: model, Prompt: prompt, Type: &kind, Automatic: true, OutputTokens: outputTokens}, sink)
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if err == nil && sink.message != "" {
		err = errors.New(sink.message)
	}
	return GeneratedTestText{Text: sink.text.String(), Model: sink.model}, err
}

// GenerateTextRequest executes a routed text request through a bounded memory sink.
// Route metadata stays attached to the request, and provider diagnostic callbacks
// are disabled because user prompts and upstream excerpts may contain private text.
// Callers provide positive output budgets and discard returned text when an
// error accompanies partial content. The private context marker also replaces
// authentication diagnostics before platform adapters persist provider errors.
func (service *TestService) GenerateTextRequest(ctx context.Context, request TestRequest, outputTokens, maxBytes int) (GeneratedTestText, error) {
	sink := &generationTestSink{limit: maxBytes}
	kind := ProviderTestTypeText
	request.Type = &kind
	request.Automatic = true
	request.OutputTokens = outputTokens
	quiet := *service
	quiet.options.Error = nil
	quiet.options.WriteError = nil
	ctx = context.WithValue(ctx, privateGenerationKey{}, true)
	err := quiet.Test(ctx, request, sink)
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if err == nil && sink.message != "" {
		err = errors.New(sink.message)
	}
	return GeneratedTestText{Text: sink.text.String(), Model: sink.model}, err
}

func (*generationTestSink) Begin(context.Context, bool) error { return nil }

func (sink *generationTestSink) Emit(_ context.Context, event TestEvent) error {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if event.Model != "" {
		sink.model = event.Model
	}
	if event.Type == "error" {
		sink.message = event.Error
	}
	if event.Type == "content" {
		if sink.text.Len()+len(event.Text) > sink.limit {
			return errors.New("generated response exceeds the byte limit")
		}
		_, _ = sink.text.WriteString(event.Text)
	}
	return nil
}

// TextGenerationDiagnostic removes upstream excerpts for private user text requests.
// Provider state may persist this message, so the generic replacement carries
// no user content. Ordinary administrator probes retain their supplied diagnostic.
func TextGenerationDiagnostic(ctx context.Context, message string) string {
	private, _ := ctx.Value(privateGenerationKey{}).(bool)
	if private {
		return "Text generation failed: provider authentication or access was rejected"
	}
	return message
}
