package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestChatAdapterCompleteTranslatesSystemMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "sk-anthropic" {
			t.Fatalf("unexpected x-api-key header %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["system"] != "You are helpful." {
			t.Fatalf("expected system message to be hoisted, got %#v", payload["system"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_1",
			"model":"claude-sonnet-4-6",
			"content":[{"type":"text","text":"Hello"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":12,"output_tokens":7}
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-anthropic",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "anthropic/claude-sonnet-4-6", 4096)

	response, err := adapter.Complete(context.Background(), &modality.ChatRequest{
		Model: "anthropic/claude-sonnet-4-6",
		Messages: []modality.ChatMessage{
			{Role: "system", Content: modality.NewTextContent("You are helpful.")},
			{Role: "user", Content: modality.NewTextContent("Hello")},
		},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.Model != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("expected canonical model, got %q", response.Model)
	}
	if response.Usage.TotalTokens != 19 {
		t.Fatalf("expected total tokens 19, got %d", response.Usage.TotalTokens)
	}
}

func TestChatAdapterStreamNormalizesEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\n"))
		_, _ = w.Write([]byte("data: {\"message\":{\"id\":\"msg_1\",\"usage\":{\"input_tokens\":12}}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_start\n"))
		_, _ = w.Write([]byte("data: {\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\n"))
		_, _ = w.Write([]byte("data: {\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n"))
		_, _ = w.Write([]byte("event: message_delta\n"))
		_, _ = w.Write([]byte("data: {\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":7}}\n\n"))
		_, _ = w.Write([]byte("event: message_stop\n"))
		_, _ = w.Write([]byte("data: {}\n\n"))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-anthropic",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "anthropic/claude-sonnet-4-6", 4096)

	stream, err := adapter.Stream(context.Background(), &modality.ChatRequest{
		Model: "anthropic/claude-sonnet-4-6",
		Messages: []modality.ChatMessage{
			{Role: "user", Content: modality.NewTextContent("Hello")},
		},
		Stream: true,
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	var chunks []modality.ChatChunk
	for chunk := range stream {
		if chunk.Err != nil {
			t.Fatalf("unexpected stream error: %v", chunk.Err)
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0].Choices[0].Delta.Role != "assistant" {
		t.Fatalf("expected first chunk role delta, got %#v", chunks[0].Choices[0].Delta)
	}
	if chunks[1].Choices[0].Delta.Content != "Hello" {
		t.Fatalf("expected text delta, got %#v", chunks[1].Choices[0].Delta)
	}
	if chunks[2].Usage == nil || chunks[2].Usage.TotalTokens != 19 {
		t.Fatalf("expected final usage, got %#v", chunks[2].Usage)
	}
}

func TestChatAdapterCountTokensUsesNativeEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages/count_tokens" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "sk-anthropic" {
			t.Fatalf("unexpected x-api-key header %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["system"] != "You are helpful." {
			t.Fatalf("expected hoisted system prompt, got %#v", payload["system"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens":23}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-anthropic",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "anthropic/claude-sonnet-4-6", 4096)

	result, err := adapter.CountTokens(context.Background(), &modality.ChatRequest{
		Model: "anthropic/claude-sonnet-4-6",
		Messages: []modality.ChatMessage{
			{Role: "system", Content: modality.NewTextContent("You are helpful.")},
			{Role: "user", Content: modality.NewTextContent("Count this prompt.")},
		},
	})
	if err != nil {
		t.Fatalf("CountTokens() error = %v", err)
	}
	if result.Source != modality.TokenCountSourceProviderReported || result.InputTokens != 23 {
		t.Fatalf("unexpected count token result %#v", result)
	}
}

func TestChatAdapterMapsAuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"bad provider key"}}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-anthropic",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "anthropic/claude-sonnet-4-6", 4096)

	_, err := adapter.Complete(context.Background(), &modality.ChatRequest{
		Model: "anthropic/claude-sonnet-4-6",
		Messages: []modality.ChatMessage{
			{Role: "user", Content: modality.NewTextContent("Hello")},
		},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	var apiErr *httputil.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Status != http.StatusBadGateway || apiErr.Type != "provider_error" {
		t.Fatalf("unexpected api error %#v", apiErr)
	}
}
