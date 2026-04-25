package ollama

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
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("expected no Authorization header, got %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "llama3" {
			t.Fatalf("unexpected model payload %#v", payload["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"llama3",
			"created_at":"2026-04-11T23:00:00Z",
			"message":{"role":"assistant","content":"Hello"},
			"done":true,
			"done_reason":"stop",
			"prompt_eval_count":10,
			"eval_count":5
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "ollama/llama3")

	response, err := adapter.Complete(context.Background(), &modality.ChatRequest{
		Model: "ollama/llama3",
		Messages: []modality.ChatMessage{
			{Role: "user", Content: modality.NewTextContent("Hello")},
		},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.Model != "ollama/llama3" {
		t.Fatalf("expected canonical model, got %q", response.Model)
	}
	if response.Usage.TotalTokens != 15 {
		t.Fatalf("expected total tokens 15, got %d", response.Usage.TotalTokens)
	}
	if response.Choices[0].Message.Content.Text == nil || *response.Choices[0].Message.Content.Text != "Hello" {
		t.Fatalf("unexpected response %#v", response)
	}
}

func TestChatAdapterStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte("{\"model\":\"llama3\",\"created_at\":\"2026-04-11T23:00:00Z\",\"message\":{\"role\":\"assistant\",\"content\":\"Hel\"},\"done\":false}\n"))
		_, _ = w.Write([]byte("{\"model\":\"llama3\",\"created_at\":\"2026-04-11T23:00:01Z\",\"message\":{\"content\":\"lo\"},\"done\":false}\n"))
		_, _ = w.Write([]byte("{\"model\":\"llama3\",\"created_at\":\"2026-04-11T23:00:02Z\",\"message\":{\"content\":\"\"},\"done\":true,\"done_reason\":\"stop\",\"prompt_eval_count\":10,\"eval_count\":5}\n"))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "ollama/llama3")

	stream, err := adapter.Stream(context.Background(), &modality.ChatRequest{
		Model: "ollama/llama3",
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
	if chunks[1].Choices[0].Delta.Content != "lo" {
		t.Fatalf("expected content delta lo, got %#v", chunks[1].Choices[0].Delta)
	}
	if chunks[2].Usage == nil || chunks[2].Usage.TotalTokens != 15 {
		t.Fatalf("expected final usage, got %#v", chunks[2].Usage)
	}
}

func TestChatAdapterMapsMissingModelError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"model 'llama3' not found"}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "ollama/llama3")

	_, err := adapter.Complete(context.Background(), &modality.ChatRequest{
		Model: "ollama/llama3",
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
	if apiErr.Status != http.StatusBadGateway || apiErr.Code != "provider_model_unavailable" {
		t.Fatalf("unexpected api error %#v", apiErr)
	}
}
