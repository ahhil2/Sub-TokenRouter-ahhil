package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	semanticFixture = "[No upstream account is currently available. Please retry later.]"
)

type fragmentReader struct {
	io.Reader
	chunk int
}

func (reader fragmentReader) Read(buffer []byte) (int, error) {
	return reader.Reader.Read(buffer[:min(len(buffer), reader.chunk)])
}

func semanticStream(text string) string {
	encoded, _ := json.Marshal(text)
	return "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"content\":[],\"usage\":{\"input_tokens\":0}}}\n\n" +
		"data: {\"type\":\"content_block_start\",\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
		"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":" + string(encoded) + "}}\n\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":0}}\n\n" +
		"data: {\"type\":\"message_stop\"}\n\n"
}

func TestDetectSemanticError(test *testing.T) {
	jsonText, err := json.Marshal(semanticFixture)
	require.NoError(test, err)
	for _, scenario := range []struct {
		name string
		body string
		want bool
	}{
		{"stream", semanticStream(semanticFixture), true},
		{"eof terminal", strings.TrimRight(semanticStream(semanticFixture), "\n"), true},
		{"event name", strings.ReplaceAll(semanticStream(semanticFixture), "data: {\"type\":\"message_stop\"}", "event: message_stop\ndata: {}"), true},
		{"split deltas", strings.ReplaceAll(semanticStream(semanticFixture), `currently available.`, `currently "}}`+"\n\n"+`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"available.`), true},
		{"normalized", semanticStream(strings.ToUpper(strings.ReplaceAll(semanticFixture, " ", " \n "))), true},
		{"crlf", strings.ReplaceAll(semanticStream(semanticFixture), "\n", "\r\n"), true},
		{"json", `{"type":"message","content":[{"type":"text","text":` + string(jsonText) + `}]}`, true},
		{"ordinary", semanticStream("Hello"), false},
		{"quote", semanticStream("The server said: " + semanticFixture), false},
		{"suffix", semanticStream(semanticFixture + " Here is an explanation."), false},
		{"empty", semanticStream(""), false},
		{"tool", "data: {\"type\":\"content_block_start\",\"content_block\":{\"type\":\"tool_use\"}}\n\n" + semanticStream(semanticFixture), false},
		{"thinking", "data: {\"type\":\"content_block_start\",\"content_block\":{\"type\":\"thinking\"}}\n\n" + semanticStream(semanticFixture), false},
		{"truncated", strings.ReplaceAll(semanticStream(semanticFixture), "data: {\"type\":\"message_stop\"}\n\n", ""), false},
		{"error", "event: error\ndata: {\"type\":\"error\"}\n\n", false},
		{"oversize", strings.Repeat(": ping\n\n", semanticProbeLimit/8+1) + semanticStream(semanticFixture), false},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			for _, chunk := range []int{1, 17, 4096} {
				if scenario.name == "oversize" && chunk == 1 {
					continue
				}
				response := &http.Response{StatusCode: 200, Body: io.NopCloser(fragmentReader{Reader: strings.NewReader(scenario.body), chunk: chunk})}
				matched, err := DetectSemanticError(context.Background(), response, []string{semanticFixture})
				require.NoError(test, err)
				require.Equal(test, scenario.want, matched)
				if !matched {
					replayed, err := io.ReadAll(response.Body)
					require.NoError(test, err)
					require.Equal(test, scenario.body, string(replayed))
				}
				require.NoError(test, response.Body.Close())
			}
		})
	}
}

func TestDetectSemanticErrorTimeoutReplaysPendingRead(test *testing.T) {
	reader, writer := io.Pipe()
	test.Cleanup(func() { require.NoError(test, reader.Close()) })
	test.Cleanup(func() { require.NoError(test, writer.Close()) })
	response := &http.Response{StatusCode: 200, Body: reader}
	matched, err := detectSemanticError(context.Background(), response, []string{semanticFixture}, 10*time.Millisecond)
	require.NoError(test, err)
	require.False(test, matched)
	body := semanticStream(semanticFixture)
	go func() {
		_, _ = io.WriteString(writer, body)
		_ = writer.Close()
	}()
	replayed, err := io.ReadAll(response.Body)
	require.NoError(test, err)
	require.Equal(test, body, string(replayed))
}

func TestDetectSemanticErrorCancelledAndDisabled(test *testing.T) {
	reader, writer := io.Pipe()
	test.Cleanup(func() { require.NoError(test, reader.Close()) })
	test.Cleanup(func() { require.NoError(test, writer.Close()) })
	response := &http.Response{StatusCode: 200, Body: reader}
	matched, err := DetectSemanticError(context.Background(), response, nil)
	require.NoError(test, err)
	require.False(test, matched)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = DetectSemanticError(ctx, response, []string{semanticFixture})
	require.ErrorIs(test, err, context.Canceled)
}

func TestDetectSemanticErrorNormalTextDoesNotWaitForTerminal(test *testing.T) {
	reader, writer := io.Pipe()
	test.Cleanup(func() { require.NoError(test, reader.Close()) })
	test.Cleanup(func() { require.NoError(test, writer.Close()) })
	response := &http.Response{StatusCode: 200, Body: reader}
	first := "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n"
	go func() { _, _ = io.WriteString(writer, first) }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	matched, err := DetectSemanticError(ctx, response, []string{semanticFixture})
	require.NoError(test, err)
	require.False(test, matched)
	buffer := make([]byte, len(first))
	_, err = io.ReadFull(response.Body, buffer)
	require.NoError(test, err)
	require.Equal(test, first, string(buffer))
}

func TestDetectSemanticErrorHTTPFailureUntouched(test *testing.T) {
	reader, writer := io.Pipe()
	test.Cleanup(func() { require.NoError(test, reader.Close()) })
	test.Cleanup(func() { require.NoError(test, writer.Close()) })
	for _, status := range []int{400, 429, 502} {
		response := &http.Response{StatusCode: status, Body: reader}
		matched, err := DetectSemanticError(context.Background(), response, []string{semanticFixture})
		require.NoError(test, err)
		require.False(test, matched)
		require.Same(test, reader, response.Body)
	}
}
