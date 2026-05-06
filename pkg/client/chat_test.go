package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateChatCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected Authorization header %q", got)
		}

		var payload ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "default-chat" || payload.Stream {
			t.Fatalf("unexpected payload %#v", payload)
		}
		if len(payload.Messages) != 1 || len(payload.Messages[0].Content.Parts) != 2 {
			t.Fatalf("expected multipart chat content, got %#v", payload.Messages)
		}
		if payload.Messages[0].Content.Parts[1].File == nil || payload.Messages[0].Content.Parts[1].File.FileID != "pl_file_test" {
			t.Fatalf("expected file content part, got %#v", payload.Messages[0].Content.Parts[1])
		}
		if payload.Polaris == nil || payload.Polaris.FileUnderstanding == nil || payload.Polaris.FileUnderstanding.Mode != "derived_context" {
			t.Fatalf("expected Polaris file understanding options, got %#v", payload.Polaris)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"object":"chat.completion",
			"created":1744329600,
			"model":"openai/gpt-4o",
			"choices":[{"index":0,"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14,"cached_input_tokens":3,"reasoning_tokens":2,"source":"provider_reported"},
			"polaris":{"file_understanding":{"used":true,"mode":"derived_context","profile":"quality","artifacts":[{"file_id":"pl_file_test","kind":"image_caption","processor":"captioner","source":"generated"}]}}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, WithAPIKey("secret"))
	response, err := client.CreateChatCompletion(context.Background(), &ChatCompletionRequest{
		Model: "default-chat",
		Messages: []ChatMessage{{
			Role: "user",
			Content: NewPartContent(
				ContentPart{Type: "text", Text: "Hello"},
				ContentPart{Type: "file", File: &FilePart{FileID: "pl_file_test", MimeType: "image/png", Filename: "pixel.png"}},
			),
		}},
		Polaris: &PolarisChatOptions{FileUnderstanding: &PolarisFileUnderstandingOptions{Mode: "derived_context"}},
		Stream:  true,
	})
	if err != nil {
		t.Fatalf("CreateChatCompletion() error = %v", err)
	}
	if response.Model != "openai/gpt-4o" || response.Usage.TotalTokens != 14 {
		t.Fatalf("unexpected response %#v", response)
	}
	if response.Usage.Source != "provider_reported" {
		t.Fatalf("unexpected usage source %#v", response.Usage)
	}
	if response.Usage.CachedInputTokens != 3 || response.Usage.ReasoningTokens != 2 {
		t.Fatalf("unexpected extended usage %#v", response.Usage)
	}
	if response.Polaris == nil || response.Polaris.FileUnderstanding == nil || len(response.Polaris.FileUnderstanding.Artifacts) != 1 {
		t.Fatalf("unexpected Polaris metadata %#v", response.Polaris)
	}
	if response.Polaris.FileUnderstanding.Artifacts[0].Kind != "image_caption" {
		t.Fatalf("unexpected artifact metadata %#v", response.Polaris.FileUnderstanding.Artifacts[0])
	}
}

func TestStreamChatCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected Authorization header %q", got)
		}

		var payload ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if !payload.Stream {
			t.Fatalf("expected streaming payload")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1744329600,\"model\":\"openai/gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1744329601,\"model\":\"openai/gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":4,\"total_tokens\":13,\"source\":\"provider_reported\"}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, WithAPIKey("secret"))
	stream, err := client.StreamChatCompletion(context.Background(), &ChatCompletionRequest{
		Model:    "default-chat",
		Messages: []ChatMessage{{Role: "user", Content: NewTextContent("Hello")}},
	})
	if err != nil {
		t.Fatalf("StreamChatCompletion() error = %v", err)
	}
	defer func() {
		_ = stream.Close()
	}()

	var chunks []ChatCompletionChunk
	for stream.Next() {
		chunks = append(chunks, stream.Chunk())
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err() = %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0].Choices[0].Delta.Content != "Hello" {
		t.Fatalf("unexpected first chunk %#v", chunks[0])
	}
	if chunks[1].Usage == nil || chunks[1].Usage.TotalTokens != 13 {
		t.Fatalf("unexpected final chunk %#v", chunks[1])
	}
	if chunks[1].Usage.Source != "provider_reported" {
		t.Fatalf("unexpected usage source %#v", chunks[1].Usage)
	}
}

func TestStreamChatCompletionSurfacesProviderErrorFrame(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"error\":{\"message\":\"upstream exploded\",\"type\":\"provider_error\",\"code\":\"provider_stream_error\"}}\n\n"))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	stream, err := client.StreamChatCompletion(context.Background(), &ChatCompletionRequest{
		Model:    "default-chat",
		Messages: []ChatMessage{{Role: "user", Content: NewTextContent("Hello")}},
	})
	if err != nil {
		t.Fatalf("StreamChatCompletion() error = %v", err)
	}
	defer func() {
		_ = stream.Close()
	}()

	if stream.Next() {
		t.Fatalf("expected stream to stop on provider error")
	}

	var apiErr *APIError
	if !errors.As(stream.Err(), &apiErr) {
		t.Fatalf("expected APIError, got %T", stream.Err())
	}
	if apiErr.Code != "provider_stream_error" || !strings.Contains(apiErr.Message, "upstream exploded") {
		t.Fatalf("unexpected APIError %#v", apiErr)
	}
}
