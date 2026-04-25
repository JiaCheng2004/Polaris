package openaicompat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestChatAdapterCompleteNormalizesResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req modality.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got, want := req.Model, "test-model"; got != want {
			t.Fatalf("request model = %q, want %q", got, want)
		}
		_ = json.NewEncoder(w).Encode(modality.ChatResponse{
			ID: "chatcmpl-test",
			Choices: []modality.ChatChoice{{
				Index: 0,
				Message: modality.ChatMessage{
					Role:    "assistant",
					Content: modality.NewTextContent("ok"),
				},
				FinishReason: "stop",
			}},
		})
	}))
	defer server.Close()

	client := NewClient("test", "Test", config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL,
		Timeout: time.Second,
	}, server.URL, nil)
	adapter := NewChatAdapter(client, "test/test-model", nil)

	resp, err := adapter.Complete(context.Background(), &modality.ChatRequest{
		Model: "test/test-model",
		Messages: []modality.ChatMessage{{
			Role:    "user",
			Content: modality.NewTextContent("hello"),
		}},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if got, want := resp.Model, "test/test-model"; got != want {
		t.Fatalf("response model = %q, want %q", got, want)
	}
	if resp.Object != "chat.completion" {
		t.Fatalf("response object = %q, want chat.completion", resp.Object)
	}
}

func TestChatAdapterStreamDecodesSSE(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hel\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"lo\"},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewClient("test", "Test", config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL,
		Timeout: time.Second,
	}, server.URL, nil)
	adapter := NewChatAdapter(client, "test/test-model", nil)

	stream, err := adapter.Stream(context.Background(), &modality.ChatRequest{
		Model: "test/test-model",
		Messages: []modality.ChatMessage{{
			Role:    "user",
			Content: modality.NewTextContent("hello"),
		}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	var chunks []modality.ChatChunk
	for chunk := range stream {
		if chunk.Err != nil {
			t.Fatalf("stream error = %v", chunk.Err)
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if got, want := chunks[0].Model, "test/test-model"; got != want {
		t.Fatalf("chunk model = %q, want %q", got, want)
	}
	if got, want := *chunks[1].Choices[0].FinishReason, "stop"; got != want {
		t.Fatalf("finish_reason = %q, want %q", got, want)
	}
}

func TestProviderModelName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		requestModel  string
		fallbackModel string
		want          string
	}{
		{requestModel: "openrouter/openai/gpt-5.4-mini", fallbackModel: "openrouter/openai/gpt-5.4-mini", want: "openai/gpt-5.4-mini"},
		{requestModel: "", fallbackModel: "moonshot/kimi-k2-turbo-preview", want: "kimi-k2-turbo-preview"},
		{requestModel: "glm/GLM-5.1", fallbackModel: "glm/GLM-5.1", want: "GLM-5.1"},
	}

	for _, tc := range cases {
		if got := ProviderModelName(tc.requestModel, tc.fallbackModel); got != tc.want {
			t.Fatalf("ProviderModelName(%q, %q) = %q, want %q", tc.requestModel, tc.fallbackModel, got, tc.want)
		}
	}
}
