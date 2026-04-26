package deepseek

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

func TestChatAdapterComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-deepseek" {
			t.Fatalf("unexpected Authorization header %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"object":"chat.completion",
			"created":1744329600,
			"model":"deepseek-chat",
			"choices":[{"index":0,"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15},
			"reasoning_content":"hidden"
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-deepseek",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "deepseek/deepseek-chat")

	response, err := adapter.Complete(context.Background(), &modality.ChatRequest{
		Model: "deepseek/deepseek-chat",
		Messages: []modality.ChatMessage{
			{Role: "user", Content: modality.NewTextContent("Hello")},
		},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.Model != "deepseek/deepseek-chat" {
		t.Fatalf("expected canonical model, got %q", response.Model)
	}
	if len(response.Choices) != 1 || response.Choices[0].Message.Content.Text == nil || *response.Choices[0].Message.Content.Text != "Hello" {
		t.Fatalf("unexpected response %#v", response)
	}
}

func TestChatAdapterUsesV4ProviderModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "deepseek-v4-pro" {
			t.Fatalf("provider model = %q, want deepseek-v4-pro", payload.Model)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-v4",
			"object":"chat.completion",
			"created":1777161600,
			"model":"deepseek-v4-pro",
			"choices":[{"index":0,"message":{"role":"assistant","content":"V4"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-deepseek",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "deepseek/deepseek-v4-pro")

	response, err := adapter.Complete(context.Background(), &modality.ChatRequest{
		Model: "deepseek/deepseek-v4-pro",
		Messages: []modality.ChatMessage{
			{Role: "user", Content: modality.NewTextContent("Hello")},
		},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.Model != "deepseek/deepseek-v4-pro" {
		t.Fatalf("expected canonical model, got %q", response.Model)
	}
}

func TestChatAdapterStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1744329600,\"model\":\"deepseek-chat\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1744329600,\"model\":\"deepseek-chat\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15},\"reasoning_content\":\"hidden\"}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-deepseek",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "deepseek/deepseek-chat")

	stream, err := adapter.Stream(context.Background(), &modality.ChatRequest{
		Model: "deepseek/deepseek-chat",
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
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[1].Usage == nil || chunks[1].Usage.TotalTokens != 15 {
		t.Fatalf("expected final usage, got %#v", chunks[1].Usage)
	}
}

func TestChatAdapterMapsRateLimitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"too many requests","type":"rate_limit_error","code":"rate_limit"}}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-deepseek",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "deepseek/deepseek-chat")

	_, err := adapter.Complete(context.Background(), &modality.ChatRequest{
		Model: "deepseek/deepseek-chat",
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
	if apiErr.Status != http.StatusTooManyRequests || apiErr.Type != "rate_limit_error" {
		t.Fatalf("unexpected api error %#v", apiErr)
	}
}
