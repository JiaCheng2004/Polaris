package contracttest

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

type OpenAICompatAdapterFactory func(cfg config.ProviderConfig, model string) modality.ChatAdapter

func RunOpenAICompatChatSuite(t *testing.T, canonicalModel string, expectedWireModel string, factory OpenAICompatAdapterFactory) {
	t.Helper()

	t.Run("complete", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/chat/completions" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			var req modality.ChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if req.Model != expectedWireModel {
				t.Fatalf("wire model = %q, want %q", req.Model, expectedWireModel)
			}
			_ = json.NewEncoder(w).Encode(modality.ChatResponse{
				ID: "chatcmpl-contract",
				Choices: []modality.ChatChoice{{
					Index: 0,
					Message: modality.ChatMessage{
						Role:    "assistant",
						Content: modality.NewTextContent("ok"),
					},
					FinishReason: "stop",
				}},
				Usage: modality.Usage{
					PromptTokens:     3,
					CompletionTokens: 2,
					TotalTokens:      5,
				},
			})
		}))
		defer server.Close()

		adapter := factory(config.ProviderConfig{
			APIKey:  "sk-contract",
			BaseURL: server.URL,
			Timeout: time.Second,
		}, canonicalModel)

		resp, err := adapter.Complete(context.Background(), &modality.ChatRequest{
			Model: canonicalModel,
			Messages: []modality.ChatMessage{{
				Role:    "user",
				Content: modality.NewTextContent("hello"),
			}},
		})
		if err != nil {
			t.Fatalf("Complete() error = %v", err)
		}
		if resp.Model != canonicalModel {
			t.Fatalf("response model = %q, want %q", resp.Model, canonicalModel)
		}
		if resp.Object != "chat.completion" {
			t.Fatalf("response object = %q", resp.Object)
		}
		if resp.Usage.TotalTokens != 5 {
			t.Fatalf("usage = %#v", resp.Usage)
		}
	})

	t.Run("stream", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-contract\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hel\"}}]}\n\n")
			_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-contract\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"lo\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n")
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		}))
		defer server.Close()

		adapter := factory(config.ProviderConfig{
			APIKey:  "sk-contract",
			BaseURL: server.URL,
			Timeout: time.Second,
		}, canonicalModel)

		stream, err := adapter.Stream(context.Background(), &modality.ChatRequest{
			Model: canonicalModel,
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
			t.Fatalf("chunk count = %d", len(chunks))
		}
		if chunks[0].Model != canonicalModel {
			t.Fatalf("chunk model = %q, want %q", chunks[0].Model, canonicalModel)
		}
		if finish := chunks[1].Choices[0].FinishReason; finish == nil || *finish != "stop" {
			t.Fatalf("finish reason = %#v", chunks[1].Choices[0].FinishReason)
		}
	})
}
