package openaiforward

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"

	"github.com/TokenFlux/TokenRouter/internal/protocol"
	protocolanthropic "github.com/TokenFlux/TokenRouter/internal/protocol/anthropic"
	"github.com/TokenFlux/TokenRouter/internal/protocol/bridge"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

// cacheOutputSink retains the serialized native-Anthropic compatibility response.
type cacheOutputSink struct {
	bytes.Buffer // captured client response bytes
}

// Begin accepts output metadata without writing synthetic response content.
func (*cacheOutputSink) Begin(upstream.OutputHead) error { return nil }

// Emit retains every serialized payload for final wire-level usage assertions.
func (s *cacheOutputSink) Emit(event upstream.OutputEvent) error {
	_, err := s.Write(event.Data)
	return err
}

// TestNativeAnthropicBufferedRejectsInvalidContentIndex supplies negative and
// excessive content positions before a valid delta. Both buffered client formats
// must ignore invalid positions, retain the valid text, and complete without panic.
func TestNativeAnthropicBufferedRejectsInvalidContentIndex(t *testing.T) {
	body := "event: message_start\ndata: " + `{"type":"message_start","message":{"id":"msg_index","model":"vendor","role":"assistant","content":[],"usage":{"input_tokens":2,"output_tokens":0}}}` + "\n\n" +
		"event: content_block_start\ndata: " + `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
		"event: content_block_delta\ndata: " + `{"type":"content_block_delta","index":-1,"delta":{"type":"text_delta","text":"negative"}}` + "\n\n" +
		"event: content_block_delta\ndata: " + `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"excessive"}}` + "\n\n" +
		"event: content_block_delta\ndata: " + `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"valid"}}` + "\n\n" +
		"event: message_delta\ndata: " + `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}` + "\n\n" +
		"event: message_stop\ndata: " + `{"type":"message_stop"}` + "\n\n"
	for _, chat := range []bool{false, true} {
		t.Run(map[bool]string{false: "responses", true: "chat"}[chat], func(t *testing.T) {
			sink := &cacheOutputSink{}
			output := upstream.NewOutputContext(sink)
			options := AnthropicOutputOptions{
				Runtime:     bridge.Runtime{Now: time.Now, ReadRandom: rand.Read},
				MaxLineSize: func() int { return 1 << 20 }, StreamInterval: func() time.Duration { return 0 },
				CopyHeaders: func(http.Header, http.Header) {}, ReverseTools: func(body []byte) []byte { return body },
				Error: func(_ int, _, message string) { t.Error(message) }, Warn: func(string, ...zap.Field) {},
			}
			response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
			var result *Result
			var err error
			path := "output.0.content.0.text"
			if chat {
				result, err = ChatFromAnthropicBuffered(response, output, options, "client", "vendor", "vendor", nil, time.Now())
				path = "choices.0.message.content"
			} else {
				result, err = ResponsesFromAnthropicBuffered(response, output, options, "client", "vendor", "vendor", nil, time.Now(), bridge.ResponsesClientToolMapping{})
			}
			require.NoError(t, err)
			require.Equal(t, "valid", gjson.Get(sink.String(), path).String())
			require.Equal(t, 2, result.Usage.InputTokens)
			require.Equal(t, 1, result.Usage.OutputTokens)
		})
	}
}

// TestNativeAnthropicOutputCacheOnlyMatrix supplies cache-only usage after an
// empty start event. Every buffered and streaming client adapter must report
// the same cache counts as its native accounting snapshot.
func TestNativeAnthropicOutputCacheOnlyMatrix(t *testing.T) {
	body := "event: message_start\ndata: " + `{"type":"message_start","message":{"id":"msg_cache","model":"vendor","role":"assistant","content":[],"usage":{"input_tokens":0,"output_tokens":0}}}` + "\n\n" +
		"event: message_delta\ndata: " + `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"cached_tokens":60,"cache_creation":{"ephemeral_5m_input_tokens":12,"ephemeral_1h_input_tokens":8}}}` + "\n\n" +
		"event: message_stop\ndata: " + `{"type":"message_stop"}` + "\n\n"
	for _, chat := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			name := map[bool]string{false: "responses", true: "chat"}[chat] + map[bool]string{false: "/json", true: "/sse"}[stream]
			t.Run(name, func(t *testing.T) {
				sink := &cacheOutputSink{}
				output := upstream.NewOutputContext(sink)
				options := AnthropicOutputOptions{
					Runtime:     bridge.Runtime{Now: time.Now, ReadRandom: rand.Read},
					MaxLineSize: func() int { return 1 << 20 }, StreamInterval: func() time.Duration { return 0 },
					CopyHeaders: func(http.Header, http.Header) {}, ReverseTools: func(body []byte) []byte { return body },
					Error: func(_ int, _, message string) { t.Error(message) }, Warn: func(string, ...zap.Field) {},
				}
				response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
				var result *Result
				var err error
				if chat && stream {
					result, err = ChatFromAnthropicStreaming(response, output, options, "client", "vendor", "vendor", nil, time.Now(), true)
				} else if chat {
					result, err = ChatFromAnthropicBuffered(response, output, options, "client", "vendor", "vendor", nil, time.Now())
				} else if stream {
					result, err = ResponsesFromAnthropicStreaming(response, output, options, "client", "vendor", "vendor", nil, time.Now(), bridge.ResponsesClientToolMapping{})
				} else {
					result, err = ResponsesFromAnthropicBuffered(response, output, options, "client", "vendor", "vendor", nil, time.Now(), bridge.ResponsesClientToolMapping{})
				}
				require.NoError(t, err)
				require.Equal(t, 80, result.Usage.InputTokens)
				visible := gjson.Get(sink.String(), "usage")
				if stream {
					for line := range strings.SplitSeq(sink.String(), "\n") {
						payload := strings.TrimPrefix(line, "data: ")
						if value := gjson.Get(payload, "usage"); value.IsObject() {
							visible = value
						}
						if value := gjson.Get(payload, "response.usage"); value.IsObject() {
							visible = value
						}
					}
				}
				if chat {
					require.Equal(t, int64(80), visible.Get("prompt_tokens").Int())
					require.Equal(t, int64(60), visible.Get("prompt_tokens_details.cached_tokens").Int())
					require.Equal(t, int64(20), visible.Get("prompt_tokens_details.cache_creation_tokens").Int())
				} else {
					require.Equal(t, int64(80), visible.Get("input_tokens").Int())
					require.Equal(t, int64(60), visible.Get("input_tokens_details.cached_tokens").Int())
					require.Equal(t, int64(20), visible.Get("cache_creation_input_tokens").Int())
				}
			})
		}
	}
}

// TestMergeAnthropicUsageNormalizesKimiStreamForOpenAIBilling merges start and
// terminal usage from a Kimi stream fixture. Normalized uncached and cache-read
// buckets must reconstruct the inclusive OpenAI input total while retaining
// output tokens.
func TestMergeAnthropicUsageNormalizesKimiStreamForOpenAIBilling(t *testing.T) {
	var start protocolanthropic.AnthropicStreamEvent
	require.NoError(t, json.Unmarshal([]byte(`{"type":"message_start","message":{"usage":{"input_tokens":173306,"prompt_tokens":173306,"cached_tokens":0}}}`), &start))
	var delta protocolanthropic.AnthropicStreamEvent
	require.NoError(t, json.Unmarshal([]byte(`{"type":"message_delta","usage":{"input_tokens":250,"cache_read_input_tokens":173056,"output_tokens":166,"prompt_tokens":173306,"cached_tokens":173056}}`), &delta))

	usage := &protocol.TokenUsage{}
	protocolanthropic.MergeAnthropicUsage(usage, start.Message.Usage)
	protocolanthropic.MergeAnthropicUsage(usage, *delta.Usage)
	require.Equal(t, 250, usage.InputTokens)
	require.Equal(t, 173056, usage.CacheReadInputTokens)

	openAIUsage := AnthropicUsageToOpenAI(usage)
	require.Equal(t, 173306, openAIUsage.InputTokens, "OpenAI gateway expects an inclusive input total")
	require.Equal(t, 250, openAIUsage.InputTokens-openAIUsage.CacheReadInputTokens-openAIUsage.CacheCreationInputTokens)
	require.Equal(t, 166, openAIUsage.OutputTokens)
}

// TestMergeAnthropicUsageNormalizesGLMAndDeepSeekAliases parses provider cache
// aliases from buffered usage fixtures. Both encodings must yield separate
// uncached and cache-read buckets whose conversion recovers the inclusive input
// count.
func TestMergeAnthropicUsageNormalizesGLMAndDeepSeekAliases(t *testing.T) {
	tests := []struct {
		name string // provider fixture name
		raw  string // encoded usage response
	}{
		{
			name: "GLM",
			raw:  `{"input_tokens":1200,"output_tokens":30,"prompt_tokens":1200,"prompt_tokens_details":{"cached_tokens":800}}`,
		},
		{
			name: "DeepSeek",
			raw:  `{"input_tokens":1200,"output_tokens":30,"prompt_cache_hit_tokens":800,"prompt_cache_miss_tokens":400}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var src protocolanthropic.AnthropicUsage
			require.NoError(t, json.Unmarshal([]byte(tt.raw), &src))

			usage := &protocol.TokenUsage{}
			protocolanthropic.MergeAnthropicUsage(usage, src)
			require.Equal(t, 400, usage.InputTokens)
			require.Equal(t, 800, usage.CacheReadInputTokens)

			openAIUsage := AnthropicUsageToOpenAI(usage)
			require.Equal(t, 1200, openAIUsage.InputTokens)
			require.Equal(t, 400, openAIUsage.InputTokens-openAIUsage.CacheReadInputTokens-openAIUsage.CacheCreationInputTokens)
		})
	}
}

// TestClaudeUsageToOpenAIUsagePreservesCNProviderNativeAnthropicBuckets
// converts already-normalized Anthropic token buckets for two CN fixtures.
// Cache creation and reads must remain separate while their sum with uncached
// input forms the OpenAI total.
func TestClaudeUsageToOpenAIUsagePreservesCNProviderNativeAnthropicBuckets(t *testing.T) {
	tests := []struct {
		name         string              // provider fixture name
		usage        protocol.TokenUsage // normalized token buckets
		wantTotal    int                 // expected inclusive input
		wantUncached int                 // expected uncached input
	}{
		{
			name: "GLM",
			usage: protocol.TokenUsage{
				InputTokens:              2,
				OutputTokens:             302,
				CacheCreationInputTokens: 733,
				CacheReadInputTokens:     376156,
			},
			wantTotal:    376891,
			wantUncached: 2,
		},
		{
			name: "DeepSeek",
			usage: protocol.TokenUsage{
				InputTokens:          400,
				OutputTokens:         30,
				CacheReadInputTokens: 800,
			},
			wantTotal:    1200,
			wantUncached: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			openAIUsage := AnthropicUsageToOpenAI(&tt.usage)
			require.Equal(t, tt.wantTotal, openAIUsage.InputTokens)
			require.Equal(t, tt.wantUncached, openAIUsage.InputTokens-openAIUsage.CacheReadInputTokens-openAIUsage.CacheCreationInputTokens)
			require.Equal(t, tt.usage.CacheReadInputTokens, openAIUsage.CacheReadInputTokens)
			require.Equal(t, tt.usage.CacheCreationInputTokens, openAIUsage.CacheCreationInputTokens)
		})
	}
}
