package bytedance

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

func TestChatAdapterCompleteUsesEndpointOverride(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer ark-key" {
			t.Fatalf("unexpected Authorization header %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"object":"chat.completion",
			"created":1744329600,
			"model":"doubao-pro-256k",
			"choices":[{"index":0,"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "ark-key",
		BaseURL: "https://unused.example.com/api/v3",
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "bytedance/doubao-pro-256k", server.URL+"/api/v3")

	response, err := adapter.Complete(context.Background(), &modality.ChatRequest{
		Model: "bytedance/doubao-pro-256k",
		Messages: []modality.ChatMessage{
			{Role: "user", Content: modality.NewTextContent("Hello")},
		},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.Model != "bytedance/doubao-pro-256k" {
		t.Fatalf("expected canonical model, got %q", response.Model)
	}
	if len(response.Choices) != 1 || response.Choices[0].Message.Content.Text == nil || *response.Choices[0].Message.Content.Text != "Hello" {
		t.Fatalf("unexpected response %#v", response)
	}
}

func TestChatAdapterMapsCurrentModelNames(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{
			name:  "doubao seed 2 pro",
			model: "bytedance/doubao-seed-2.0-pro",
			want:  "doubao-seed-2-0-pro-260215",
		},
		{
			name:  "doubao seed 1.6 vision",
			model: "bytedance/doubao-seed-1.6-vision",
			want:  "doubao-seed-1-6-vision-250815",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				if payload["model"] != tc.want {
					t.Fatalf("payload model = %#v, want %q", payload["model"], tc.want)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"id":"chatcmpl-1",
					"object":"chat.completion",
					"created":1744329600,
					"choices":[{"index":0,"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],
					"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
				}`))
			}))
			defer server.Close()

			client := NewClient(config.ProviderConfig{
				APIKey:  "ark-key",
				BaseURL: server.URL + "/api/v3",
				Timeout: time.Second,
			})
			adapter := NewChatAdapter(client, tc.model, server.URL+"/api/v3")

			if _, err := adapter.Complete(context.Background(), &modality.ChatRequest{
				Model: tc.model,
				Messages: []modality.ChatMessage{
					{Role: "user", Content: modality.NewTextContent("Hello")},
				},
			}); err != nil {
				t.Fatalf("Complete() error = %v", err)
			}
		})
	}
}

func TestChatAdapterStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		streamOptions, ok := payload["stream_options"].(map[string]any)
		if !ok || streamOptions["include_usage"] != true {
			t.Fatalf("expected stream_options.include_usage=true, got %#v", payload["stream_options"])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1744329600,\"model\":\"doubao-pro-256k\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1744329600,\"model\":\"doubao-pro-256k\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "ark-key",
		BaseURL: server.URL + "/api/v3",
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "bytedance/doubao-pro-256k", "")

	stream, err := adapter.Stream(context.Background(), &modality.ChatRequest{
		Model: "bytedance/doubao-pro-256k",
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

func TestChatAdapterMapsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"provider down","type":"server_error","code":"server_error"}}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "ark-key",
		BaseURL: server.URL + "/api/v3",
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "bytedance/doubao-pro-256k", "")

	_, err := adapter.Complete(context.Background(), &modality.ChatRequest{
		Model: "bytedance/doubao-pro-256k",
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
	if apiErr.Status != http.StatusBadGateway || apiErr.Code != "provider_server_error" {
		t.Fatalf("unexpected api error %#v", apiErr)
	}
}
