package google

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

func TestChatAdapterCompleteTranslatesSystemAndTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-2.5-flash:generateContent" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "google-key" {
			t.Fatalf("unexpected x-goog-api-key header %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		systemInstruction := payload["system_instruction"].(map[string]any)
		systemParts := systemInstruction["parts"].([]any)
		if systemParts[0].(map[string]any)["text"] != "You are helpful." {
			t.Fatalf("expected system instruction, got %#v", payload["system_instruction"])
		}
		contents := payload["contents"].([]any)
		if len(contents) != 1 {
			t.Fatalf("expected 1 content turn, got %d", len(contents))
		}
		tools := payload["tools"].([]any)
		if len(tools) != 1 {
			t.Fatalf("expected 1 tools entry, got %d", len(tools))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"responseId":"resp-1",
			"candidates":[
				{
					"index":0,
					"content":{
						"role":"model",
						"parts":[
							{"text":"Hello"},
							{"functionCall":{"id":"call_1","name":"lookup_weather","args":{"city":"SF"}}}
						]
					},
					"finishReason":"STOP"
				}
			],
			"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":7,"totalTokenCount":19}
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "google-key",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "google/gemini-2.5-flash")

	response, err := adapter.Complete(context.Background(), &modality.ChatRequest{
		Model: "google/gemini-2.5-flash",
		Messages: []modality.ChatMessage{
			{Role: "system", Content: modality.NewTextContent("You are helpful.")},
			{Role: "user", Content: modality.NewTextContent("Hello")},
		},
		Tools: []modality.ToolDefinition{
			{
				Type: "function",
				Function: modality.FunctionDefinition{
					Name:        "lookup_weather",
					Description: "Look up weather",
					Parameters:  json.RawMessage(`{"type":"object"}`),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.Model != "google/gemini-2.5-flash" {
		t.Fatalf("expected canonical model, got %q", response.Model)
	}
	if response.Usage.TotalTokens != 19 {
		t.Fatalf("expected total tokens 19, got %d", response.Usage.TotalTokens)
	}
	if response.Choices[0].Message.Content.Text == nil || *response.Choices[0].Message.Content.Text != "Hello" {
		t.Fatalf("unexpected message content %#v", response.Choices[0].Message)
	}
	if len(response.Choices[0].Message.ToolCalls) != 1 || response.Choices[0].Message.ToolCalls[0].Function.Name != "lookup_weather" {
		t.Fatalf("unexpected tool calls %#v", response.Choices[0].Message.ToolCalls)
	}
}

func TestChatAdapterStreamNormalizesSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-goog-api-key"); got != "google-key" {
			t.Fatalf("unexpected x-goog-api-key header %q", got)
		}
		if r.URL.RawQuery != "alt=sse" {
			t.Fatalf("expected alt=sse query, got %q", r.URL.RawQuery)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"responseId\":\"resp-1\",\"candidates\":[{\"index\":0,\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"Hello\"}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"responseId\":\"resp-1\",\"candidates\":[{\"index\":0,\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":12,\"candidatesTokenCount\":7,\"totalTokenCount\":19}}\n\n"))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "google-key",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "google/gemini-2.5-flash")

	stream, err := adapter.Stream(context.Background(), &modality.ChatRequest{
		Model: "google/gemini-2.5-flash",
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
		t.Fatalf("expected assistant role delta, got %#v", chunks[0].Choices[0].Delta)
	}
	if chunks[1].Choices[0].Delta.Content != "Hello" {
		t.Fatalf("expected text delta Hello, got %#v", chunks[1].Choices[0].Delta)
	}
	if chunks[2].Usage == nil || chunks[2].Usage.TotalTokens != 19 {
		t.Fatalf("expected final usage, got %#v", chunks[2].Usage)
	}
}

func TestChatAdapterCountTokensUsesNativeEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-2.5-flash:countTokens" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "google-key" {
			t.Fatalf("unexpected x-goog-api-key header %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if _, ok := payload["generateContentRequest"]; !ok {
			t.Fatalf("expected generateContentRequest payload, got %#v", payload)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totalTokens":21}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "google-key",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "google/gemini-2.5-flash")

	result, err := adapter.CountTokens(context.Background(), &modality.ChatRequest{
		Model: "google/gemini-2.5-flash",
		Messages: []modality.ChatMessage{
			{Role: "system", Content: modality.NewTextContent("You are helpful.")},
			{Role: "user", Content: modality.NewTextContent("Count this prompt.")},
		},
	})
	if err != nil {
		t.Fatalf("CountTokens() error = %v", err)
	}
	if result.Source != modality.TokenCountSourceProviderReported || result.InputTokens != 21 {
		t.Fatalf("unexpected count token result %#v", result)
	}
}

func TestChatAdapterMapsAuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":401,"message":"bad provider key","status":"UNAUTHENTICATED"}}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "google-key",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewChatAdapter(client, "google/gemini-2.5-flash")

	_, err := adapter.Complete(context.Background(), &modality.ChatRequest{
		Model: "google/gemini-2.5-flash",
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
