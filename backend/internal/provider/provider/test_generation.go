package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

type generationBufferSink struct{ text strings.Builder }

func (*generationBufferSink) Begin(context.Context, bool) error { return nil }
func (sink *generationBufferSink) Emit(_ context.Context, event provider.TestEvent) error {
	if event.Type == "content" {
		if sink.text.Len()+len(event.Text) > 1<<20 {
			return errors.New("Generated response exceeds the byte limit")
		}
		sink.text.WriteString(event.Text)
	}
	return nil
}

func (run *TestRun) generationReader(body io.Reader) io.Reader {
	if provider.TestOutputBudget(run.Context) > 0 {
		return io.LimitReader(body, 8<<20)
	}
	return body
}

func (run *TestRun) outputBudget(fallback int) int {
	if budget := provider.TestOutputBudget(run.Context); budget > 0 {
		return budget
	}
	return fallback
}

func (run *TestRun) generationPayload(payload map[string]any, protocol string) {
	budget := provider.TestOutputBudget(run.Context)
	if budget <= 0 {
		return
	}
	switch protocol {
	case "responses":
		payload["max_output_tokens"] = budget
	case "gemini":
		config, ok := payload["generationConfig"].(map[string]any)
		if !ok {
			config = map[string]any{}
			payload["generationConfig"] = config
		}
		config["maxOutputTokens"] = budget
	default:
		payload["max_tokens"] = budget
	}
}

func (run *TestRun) generationJSON(raw []byte, protocol string) []byte {
	if provider.TestOutputBudget(run.Context) <= 0 {
		return raw
	}
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil {
		return raw
	}
	run.generationPayload(payload, protocol)
	result, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return result
}

func (run *TestRun) truncated(reason string) bool {
	if provider.TestOutputBudget(run.Context) <= 0 {
		return false
	}
	switch strings.ToLower(reason) {
	case "length", "max_tokens", "max_output_tokens", "incomplete", "content_filter", "safety", "recitation":
		return true
	default:
		return false
	}
}
