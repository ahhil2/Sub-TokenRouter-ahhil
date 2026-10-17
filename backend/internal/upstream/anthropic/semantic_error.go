package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

const (
	semanticProbeLimit = 64 << 10
	semanticProbeWait  = 2 * time.Second
)

type semanticRead struct {
	data []byte
	err  error
}

type semanticReplayBody struct {
	prefix  *bytes.Reader
	body    io.ReadCloser
	pending <-chan semanticRead
	err     error
}

type semanticProbe struct {
	texts     []string
	text      string
	line      string
	data      []string
	eventName string
	json      bool
	started   bool
}

func DetectSemanticError(ctx context.Context, response *http.Response, texts []string) (bool, error) {
	return detectSemanticError(ctx, response, texts, semanticProbeWait)
}

func detectSemanticError(ctx context.Context, response *http.Response, texts []string, wait time.Duration) (bool, error) {
	if response == nil || response.Body == nil || response.StatusCode < 200 || response.StatusCode >= 300 || len(texts) == 0 {
		return false, nil
	}
	probe := semanticProbe{texts: make([]string, len(texts))}
	for index, text := range texts {
		probe.texts[index] = normalizeSemanticText(text)
	}
	body := response.Body
	prefix := make([]byte, 0, 4096)
	timer := time.NewTimer(wait)
	defer timer.Stop()
	emptyReads := 0
	for len(prefix) < semanticProbeLimit {
		pending := make(chan semanticRead, 1)
		buffer := make([]byte, min(4096, semanticProbeLimit-len(prefix)))
		go func() {
			count, err := body.Read(buffer)
			pending <- semanticRead{data: buffer[:count], err: err}
		}()
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-timer.C:
			response.Body = &semanticReplayBody{prefix: bytes.NewReader(prefix), body: body, pending: pending}
			return false, nil
		case result := <-pending:
			if len(result.data) == 0 {
				emptyReads++
			} else {
				emptyReads = 0
			}
			prefix = append(prefix, result.data...)
			matched, finished := probe.consume(result.data, prefix)
			if !finished && result.err == io.EOF && !probe.json {
				matched, finished = probe.consume([]byte("\n\n"), prefix)
			}
			if matched {
				return true, nil
			}
			if finished || result.err != nil || emptyReads >= 3 {
				response.Body = &semanticReplayBody{prefix: bytes.NewReader(prefix), body: body, err: result.err}
				return false, nil
			}
		}
	}
	response.Body = &semanticReplayBody{prefix: bytes.NewReader(prefix), body: body}
	return false, nil
}

func (reader *semanticReplayBody) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if reader.prefix.Len() > 0 {
		return reader.prefix.Read(buffer)
	}
	if reader.pending != nil {
		result := <-reader.pending
		reader.pending = nil
		reader.prefix = bytes.NewReader(result.data)
		reader.err = result.err
		return reader.Read(buffer)
	}
	if reader.err != nil {
		return 0, reader.err
	}
	return reader.body.Read(buffer)
}

func (reader *semanticReplayBody) Close() error {
	return reader.body.Close()
}

func (probe *semanticProbe) consume(chunk, prefix []byte) (bool, bool) {
	if !probe.started {
		trimmed := bytes.TrimSpace(prefix)
		if len(trimmed) == 0 {
			return false, false
		}
		probe.started = true
		probe.json = trimmed[0] == '{'
	}
	if probe.json {
		if !json.Valid(prefix) {
			return false, false
		}
		var message struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if json.Unmarshal(prefix, &message) != nil || message.Type != "message" || len(message.Content) == 0 {
			return false, true
		}
		for _, block := range message.Content {
			if block.Type != "text" {
				return false, true
			}
			probe.text += block.Text
		}
		return probe.matches(), true
	}
	probe.line += string(chunk)
	for {
		line, rest, found := strings.Cut(probe.line, "\n")
		if !found {
			return false, false
		}
		probe.line = rest
		line = strings.TrimSuffix(line, "\r")
		if line != "" {
			if data, ok := strings.CutPrefix(line, "data:"); ok {
				probe.data = append(probe.data, strings.TrimPrefix(data, " "))
			}
			if name, ok := strings.CutPrefix(line, "event:"); ok {
				probe.eventName = strings.TrimSpace(name)
			}
			continue
		}
		data := strings.Join(probe.data, "\n")
		probe.data = nil
		if data == "" {
			probe.eventName = ""
			continue
		}
		matched, finished := probe.event(data)
		probe.eventName = ""
		if finished {
			return matched, true
		}
	}
}

func (probe *semanticProbe) event(data string) (bool, bool) {
	var event struct {
		Type         string `json:"type"`
		ContentBlock struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content_block"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
		Message struct {
			Content []json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal([]byte(data), &event) != nil {
		return false, true
	}
	if event.Type == "" {
		event.Type = probe.eventName
	}
	switch event.Type {
	case "message_start":
		if len(event.Message.Content) != 0 {
			return false, true
		}
	case "content_block_start":
		if event.ContentBlock.Type != "text" {
			return false, true
		}
		probe.text += event.ContentBlock.Text
	case "content_block_delta":
		if event.Delta.Type != "text_delta" {
			return false, true
		}
		probe.text += event.Delta.Text
	case "message_stop":
		return probe.matches(), true
	case "ping", "content_block_stop", "message_delta":
	default:
		return false, true
	}
	text := normalizeSemanticText(probe.text)
	for _, candidate := range probe.texts {
		if strings.HasPrefix(candidate, text) {
			return false, false
		}
	}
	return false, true
}

func (probe *semanticProbe) matches() bool {
	text := normalizeSemanticText(probe.text)
	if text == "" {
		return false
	}
	return slices.Contains(probe.texts, text)
}

func normalizeSemanticText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}
