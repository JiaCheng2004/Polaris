package zaitoken

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

func TestChatAdapterSendsZaiTokenHeadersAndSystem(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got, want := r.Header.Get("x-api-key"), "sk-zai"; got != want {
			t.Fatalf("x-api-key = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("anthropic-version"), "2023-06-01"; got != want {
			t.Fatalf("anthropic-version = %q, want %q", got, want)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["system"] != "You are helpful." {
			t.Fatalf("system = %#v", payload["system"])
		}
		if payload["model"] != "glm-5.1" {
			t.Fatalf("model = %#v", payload["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_1",
			"model":"glm-5.1",
			"content":[{"type":"text","text":"ok"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":4,"output_tokens":2}
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-zai",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "zai-token/glm-5.1", 4096)

	resp, err := adapter.Complete(context.Background(), &modality.ChatRequest{
		Model: "zai-token/glm-5.1",
		Messages: []modality.ChatMessage{
			{Role: "system", Content: modality.NewTextContent("You are helpful.")},
			{Role: "user", Content: modality.NewTextContent("hello")},
		},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if resp.Model != "zai-token/glm-5.1" || resp.Usage.TotalTokens != 6 {
		t.Fatalf("unexpected response %#v", resp)
	}
}

func TestChatAdapterStreamsZaiTokenEvents(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: message_start\n")
		_, _ = fmt.Fprint(w, "data: {\"message\":{\"id\":\"msg_1\",\"usage\":{\"input_tokens\":4}}}\n\n")
		_, _ = fmt.Fprint(w, "event: content_block_start\n")
		_, _ = fmt.Fprint(w, "data: {\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		_, _ = fmt.Fprint(w, "event: content_block_delta\n")
		_, _ = fmt.Fprint(w, "data: {\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\n")
		_, _ = fmt.Fprint(w, "event: message_delta\n")
		_, _ = fmt.Fprint(w, "data: {\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n")
		_, _ = fmt.Fprint(w, "event: message_stop\n")
		_, _ = fmt.Fprint(w, "data: {}\n\n")
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-zai",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "zai-token/glm-5.1", 4096)

	stream, err := adapter.Stream(context.Background(), &modality.ChatRequest{
		Model:    "zai-token/glm-5.1",
		Messages: []modality.ChatMessage{{Role: "user", Content: modality.NewTextContent("hello")}},
		Stream:   true,
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	var text strings.Builder
	for chunk := range stream {
		if chunk.Err != nil {
			t.Fatalf("stream error = %v", chunk.Err)
		}
		if len(chunk.Choices) > 0 {
			text.WriteString(chunk.Choices[0].Delta.Content)
		}
	}
	if text.String() != "ok" {
		t.Fatalf("stream text = %q", text.String())
	}
}
