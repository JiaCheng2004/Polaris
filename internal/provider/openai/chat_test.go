package openai

import (
	"context"
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
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("unexpected Authorization header %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"object":"chat.completion",
			"created":1744329600,
			"model":"gpt-4o",
			"choices":[{"index":0,"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "openai/gpt-4o")

	response, err := adapter.Complete(context.Background(), &modality.ChatRequest{
		Model: "openai/gpt-4o",
		Messages: []modality.ChatMessage{
			{Role: "user", Content: modality.NewTextContent("Hello")},
		},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.Model != "openai/gpt-4o" {
		t.Fatalf("expected canonical model, got %q", response.Model)
	}
	if len(response.Choices) != 1 || response.Choices[0].Message.Content.Text == nil || *response.Choices[0].Message.Content.Text != "Hello" {
		t.Fatalf("unexpected response %#v", response)
	}
}

func TestChatAdapterStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1744329600,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1744329600,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "openai/gpt-4o")

	stream, err := adapter.Stream(context.Background(), &modality.ChatRequest{
		Model: "openai/gpt-4o",
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
		APIKey:  "sk-test",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "openai/gpt-4o")

	_, err := adapter.Complete(context.Background(), &modality.ChatRequest{
		Model: "openai/gpt-4o",
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
