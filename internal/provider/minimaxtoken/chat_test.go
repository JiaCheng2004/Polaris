package minimaxtoken

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestChatAdapterSendsMiniMaxTokenHeadersAndWireModel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer sk-minimax"; got != want {
			t.Fatalf("Authorization = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("anthropic-version"), "2023-06-01"; got != want {
			t.Fatalf("anthropic-version = %q, want %q", got, want)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "MiniMax-M2.7" {
			t.Fatalf("model = %#v", payload["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_1",
			"model":"MiniMax-M2.7",
			"content":[{"type":"text","text":"ok"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":4,"output_tokens":2}
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-minimax",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "minimax-token/minimax-m2.7", 4096)

	resp, err := adapter.Complete(context.Background(), &modality.ChatRequest{
		Model:    "minimax-token/minimax-m2.7",
		Messages: []modality.ChatMessage{{Role: "user", Content: modality.NewTextContent("hello")}},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if resp.Model != "minimax-token/minimax-m2.7" || resp.Usage.TotalTokens != 6 {
		t.Fatalf("unexpected response %#v", resp)
	}
}

func TestNativeMessagesAdapterSendsMiniMaxWireModel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "MiniMax-M2" {
			t.Fatalf("model = %#v", payload["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_1",
			"type":"message",
			"role":"assistant",
			"model":"MiniMax-M2",
			"content":[{"type":"text","text":"ok"}],
			"usage":{"input_tokens":4,"output_tokens":2}
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-minimax",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewNativeMessagesAdapter(client, "minimax-token/minimax-m2")

	resp, err := adapter.CreateMessage(context.Background(), json.RawMessage(`{"model":"wrong","messages":[]}`), "minimax-token/minimax-m2")
	if err != nil {
		t.Fatalf("CreateMessage() error = %v", err)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 6 {
		t.Fatalf("usage = %#v", resp.Usage)
	}
}
