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
	if resp.Usage.TotalTokens != 19 || resp.Usage.CachedInputTokens != 3 || resp.Usage.CacheWrite5mTokens != 2 {
		t.Fatalf("unexpected usage %#v", resp.Usage)
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
