package provider

import (
	"context"
	"errors"
	"strings"
	"sync"
)

type testOutputBudgetKey struct{}

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
			return errors.New("Generated response exceeds the byte limit")
		}
		sink.text.WriteString(event.Text)
	}
	return nil
}
