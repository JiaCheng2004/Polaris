package anthropiccompat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestChatAdapterCompleteNormalizesAnthropicMessages(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got, want := payload["model"], "test-model"; got != want {
			t.Fatalf("model = %#v, want %q", got, want)
		}
		if got, want := payload["system"], "You are helpful."; got != want {
			t.Fatalf("system = %#v, want %q", got, want)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_1",
			"model":"test-model",
			"content":[{"type":"text","text":"Hello"}],
			"stop_reason":"end_turn",
			"usage":{
				"input_tokens":12,
				"output_tokens":7,
				"cache_read_input_tokens":3,
				"cache_creation_input_tokens":2
			}
		}`))
	}))
	defer server.Close()

	client := NewClient("test", "Test", config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL,
		Timeout: time.Second,
	}, server.URL, nil)
	adapter := NewChatAdapter(client, "test/test-model", 4096, nil)

	resp, err := adapter.Complete(context.Background(), &modality.ChatRequest{
		Model: "test/test-model",
		Messages: []modality.ChatMessage{
			{Role: "system", Content: modality.NewTextContent("You are helpful.")},
			{Role: "user", Content: modality.NewTextContent("Hello")},
		},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if resp.Model != "test/test-model" {
		t.Fatalf("model = %q, want test/test-model", resp.Model)
	}
	if resp.Choices[0].Message.Content.Text == nil || *resp.Choices[0].Message.Content.Text != "Hello" {
		t.Fatalf("unexpected message %#v", resp.Choices[0].Message)
	}
	// Anthropic reports input_tokens (12) EXCLUDING cache reads (3) and cache
	// creation (2). The gateway convention folds those back into the prompt total,
	// so prompt=17, total=24, and the fresh (non-cached) input that bills at the
	// full input rate is 17-3-2=12 — never negative/zeroed by a large cache read.
	if resp.Usage.PromptTokens != 17 || resp.Usage.TotalTokens != 24 {
		t.Fatalf("prompt/total = %d/%d, want 17/24 (cache folded into prompt): %#v", resp.Usage.PromptTokens, resp.Usage.TotalTokens, resp.Usage)
	}
	if resp.Usage.CachedInputTokens != 3 || resp.Usage.CacheWrite5mTokens != 2 {
		t.Fatalf("unexpected cache usage %#v", resp.Usage)
	}
}

func TestAnthropicUsageFoldsCacheIntoPromptTotal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		usage     anthropicUsage
		prompt    int
		cacheRead int
		write5m   int
		write1h   int
	}{
		{
			name:      "large cache read does not zero fresh input",
			usage:     anthropicUsage{InputTokens: 200, OutputTokens: 40, CacheReadInputTokens: 5000},
			prompt:    5200,
			cacheRead: 5000,
		},
		{
			name:    "5m and 1h cache writes fold in",
			usage:   anthropicUsage{InputTokens: 100, OutputTokens: 10, CacheCreation5mInputTokens: 30, CacheCreation1hInputTokens: 20},
			prompt:  150,
			write5m: 30,
			write1h: 20,
		},
		{
			name:      "no cache activity is unchanged",
			usage:     anthropicUsage{InputTokens: 80, OutputTokens: 12},
			prompt:    80,
			cacheRead: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := tc.usage.toModalityUsage()
			if u.PromptTokens != tc.prompt {
				t.Fatalf("PromptTokens = %d, want %d", u.PromptTokens, tc.prompt)
			}
			if u.CachedInputTokens != tc.cacheRead || u.CacheWrite5mTokens != tc.write5m || u.CacheWrite1hTokens != tc.write1h {
				t.Fatalf("cache buckets = %d/%d/%d, want %d/%d/%d", u.CachedInputTokens, u.CacheWrite5mTokens, u.CacheWrite1hTokens, tc.cacheRead, tc.write5m, tc.write1h)
			}
			if u.TotalTokens != tc.prompt+tc.usage.OutputTokens {
				t.Fatalf("TotalTokens = %d, want %d", u.TotalTokens, tc.prompt+tc.usage.OutputTokens)
			}
			// This is exactly what the pricing estimator bills at the full input rate
			// (billableInputTokens = prompt - cached - writes). It must equal the fresh
			// input Anthropic reported and never go negative — the regression that made
			// a large cache read erase all input cost.
			fresh := u.PromptTokens - u.CachedInputTokens - u.CacheWrite5mTokens - u.CacheWrite1hTokens
			if fresh != tc.usage.InputTokens {
				t.Fatalf("billable fresh input = %d, want %d", fresh, tc.usage.InputTokens)
			}
		})
	}
}

func TestChatAdapterStreamDecodesAnthropicSSE(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: message_start\n")
		_, _ = fmt.Fprint(w, "data: {\"message\":{\"id\":\"msg_1\",\"usage\":{\"input_tokens\":12}}}\n\n")
		_, _ = fmt.Fprint(w, "event: content_block_start\n")
		_, _ = fmt.Fprint(w, "data: {\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		_, _ = fmt.Fprint(w, "event: content_block_delta\n")
		_, _ = fmt.Fprint(w, "data: {\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hel\"}}\n\n")
		_, _ = fmt.Fprint(w, "event: content_block_delta\n")
		_, _ = fmt.Fprint(w, "data: {\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"lo\"}}\n\n")
		_, _ = fmt.Fprint(w, "event: message_delta\n")
		_, _ = fmt.Fprint(w, "data: {\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":7}}\n\n")
		_, _ = fmt.Fprint(w, "event: message_stop\n")
		_, _ = fmt.Fprint(w, "data: {}\n\n")
	}))
	defer server.Close()

	client := NewClient("test", "Test", config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL,
		Timeout: time.Second,
	}, server.URL, nil)
	adapter := NewChatAdapter(client, "test/test-model", 4096, nil)

	stream, err := adapter.Stream(context.Background(), &modality.ChatRequest{
		Model: "test/test-model",
		Messages: []modality.ChatMessage{
			{Role: "user", Content: modality.NewTextContent("Hello")},
		},
		Stream: true,
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	var text strings.Builder
	var final *modality.Usage
	for chunk := range stream {
		if chunk.Err != nil {
			t.Fatalf("stream error = %v", chunk.Err)
		}
		if len(chunk.Choices) > 0 {
			text.WriteString(chunk.Choices[0].Delta.Content)
		}
		if chunk.Usage != nil {
			final = chunk.Usage
		}
	}
	if text.String() != "Hello" {
		t.Fatalf("stream text = %q, want Hello", text.String())
	}
	if final == nil || final.TotalTokens != 19 {
		t.Fatalf("final usage = %#v, want total 19", final)
	}
}

func TestChatAdapterStreamCarriesCacheUsage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: message_start\n")
		_, _ = fmt.Fprint(w, "data: {\"message\":{\"id\":\"msg_1\",\"usage\":{\"input_tokens\":200,\"cache_read_input_tokens\":5000,\"cache_creation_5m_input_tokens\":40}}}\n\n")
		_, _ = fmt.Fprint(w, "event: content_block_start\n")
		_, _ = fmt.Fprint(w, "data: {\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		_, _ = fmt.Fprint(w, "event: content_block_delta\n")
		_, _ = fmt.Fprint(w, "data: {\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi\"}}\n\n")
		_, _ = fmt.Fprint(w, "event: message_delta\n")
		_, _ = fmt.Fprint(w, "data: {\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":9}}\n\n")
		_, _ = fmt.Fprint(w, "event: message_stop\n")
		_, _ = fmt.Fprint(w, "data: {}\n\n")
	}))
	defer server.Close()

	client := NewClient("test", "Test", config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL,
		Timeout: time.Second,
	}, server.URL, nil)
	adapter := NewChatAdapter(client, "test/test-model", 4096, nil)

	stream, err := adapter.Stream(context.Background(), &modality.ChatRequest{
		Model:    "test/test-model",
		Messages: []modality.ChatMessage{{Role: "user", Content: modality.NewTextContent("Hi")}},
		Stream:   true,
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	var final *modality.Usage
	for chunk := range stream {
		if chunk.Err != nil {
			t.Fatalf("stream error = %v", chunk.Err)
		}
		if chunk.Usage != nil {
			final = chunk.Usage
		}
	}
	if final == nil {
		t.Fatal("stream produced no final usage")
	}
	// prompt = 200 fresh + 5000 cache read + 40 cache write = 5240; total = 5240 + 9.
	if final.PromptTokens != 5240 || final.TotalTokens != 5249 {
		t.Fatalf("prompt/total = %d/%d, want 5240/5249: %#v", final.PromptTokens, final.TotalTokens, final)
	}
	if final.CachedInputTokens != 5000 || final.CacheWrite5mTokens != 40 {
		t.Fatalf("streaming dropped cache usage: %#v", final)
	}
}

func TestNativeMessagesAdapterRewritesModel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got, want := payload["model"], "test-model"; got != want {
			t.Fatalf("model = %#v, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_1",
			"type":"message",
			"role":"assistant",
			"model":"test-model",
			"content":[{"type":"text","text":"ok"}],
			"usage":{"input_tokens":5,"output_tokens":2}
		}`))
	}))
	defer server.Close()

	client := NewClient("test", "Test", config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL,
		Timeout: time.Second,
	}, server.URL, nil)
	adapter := NewNativeMessagesAdapter(client, "test/test-model")

	resp, err := adapter.CreateMessage(context.Background(), json.RawMessage(`{"model":"wrong","messages":[]}`), "test/test-model")
	if err != nil {
		t.Fatalf("CreateMessage() error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["model"] != "test/test-model" {
		t.Fatalf("response model = %#v", payload["model"])
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 7 {
		t.Fatalf("usage = %#v, want total 7", resp.Usage)
	}
}
