package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected Authorization header %q", got)
		}

		var payload ResponsesRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "default-chat" || payload.Stream {
			t.Fatalf("unexpected payload %#v", payload)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_1",
			"object":"response",
			"created_at":1744329600,
			"status":"completed",
			"model":"openai/gpt-4o",
			"output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello"}]}],
			"output_text":"Hello",
			"usage":{"input_tokens":10,"output_tokens":4,"total_tokens":14,"source":"provider_reported"}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, WithAPIKey("secret"))
	response, err := client.CreateResponse(context.Background(), &ResponsesRequest{
		Model: "default-chat",
		Input: json.RawMessage(`"Hello"`),
	})
	if err != nil {
		t.Fatalf("CreateResponse() error = %v", err)
	}
	if response.Model != "openai/gpt-4o" || response.OutputText != "Hello" || response.Usage.TotalTokens != 14 {
		t.Fatalf("unexpected response %#v", response)
	}
	if response.Usage.Source != "provider_reported" {
		t.Fatalf("unexpected usage source %#v", response.Usage)
	}
}

func TestCreateMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		var payload MessagesRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "default-chat" || payload.Stream {
			t.Fatalf("unexpected payload %#v", payload)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_1",
			"type":"message",
			"role":"assistant",
			"model":"anthropic/claude-sonnet-4-6",
			"content":[{"type":"text","text":"Hello from Claude"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":9,"output_tokens":5,"source":"provider_reported"}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	response, err := client.CreateMessage(context.Background(), &MessagesRequest{
		Model: "default-chat",
		Messages: []MessagesInputMessage{{
			Role:    "user",
			Content: json.RawMessage(`"Hello"`),
		}},
	})
	if err != nil {
		t.Fatalf("CreateMessage() error = %v", err)
	}
	if response.Role != "assistant" || len(response.Content) != 1 || response.Content[0].Text != "Hello from Claude" {
		t.Fatalf("unexpected response %#v", response)
	}
	if response.Usage.Source != "provider_reported" {
		t.Fatalf("unexpected usage source %#v", response.Usage)
	}
}

func TestCountTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tokens/count" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		var payload TokenCountRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "default-chat" {
			t.Fatalf("unexpected payload %#v", payload)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"openai/gpt-4o",
			"input_tokens":11,
			"output_tokens_estimate":2048,
			"source":"estimated",
			"notes":["estimated from normalized request content"]
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	response, err := client.CountTokens(context.Background(), &TokenCountRequest{
		Model:              "default-chat",
		Input:              json.RawMessage(`"Count these tokens"`),
		RequestedInterface: "responses",
	})
	if err != nil {
		t.Fatalf("CountTokens() error = %v", err)
	}
	if response.Source != "estimated" || response.InputTokens != 11 || response.OutputTokensEstimate != 2048 {
		t.Fatalf("unexpected response %#v", response)
	}
}

func TestStreamResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":1744329600,\"status\":\"in_progress\",\"model\":\"openai/gpt-4o\",\"output\":[],\"usage\":{\"input_tokens\":0,\"output_tokens\":0,\"total_tokens\":0,\"source\":\"unavailable\"}}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":1744329600,\"status\":\"completed\",\"model\":\"openai/gpt-4o\",\"output\":[],\"output_text\":\"Hello\",\"usage\":{\"input_tokens\":9,\"output_tokens\":4,\"total_tokens\":13,\"source\":\"provider_reported\"}}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	stream, err := client.StreamResponse(context.Background(), &ResponsesRequest{
		Model:  "default-chat",
		Input:  json.RawMessage(`"Hello"`),
		Stream: true,
	})
	if err != nil {
		t.Fatalf("StreamResponse() error = %v", err)
	}
	defer func() {
		_ = stream.Close()
	}()

	var events []ResponsesStreamEvent
	for stream.Next() {
		events = append(events, stream.Event())
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err() = %v", err)
	}
	if len(events) != 3 || events[1].Type != "response.output_text.delta" || events[1].Delta != "Hello" {
		t.Fatalf("unexpected events %#v", events)
	}
	if events[2].Response == nil || events[2].Response.Usage.Source != "provider_reported" {
		t.Fatalf("unexpected response usage source %#v", events)
	}
}

func TestStreamMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"anthropic/claude-sonnet-4-6\",\"usage\":{\"input_tokens\":0,\"output_tokens\":0,\"source\":\"unavailable\"}}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	stream, err := client.StreamMessage(context.Background(), &MessagesRequest{
		Model: "default-chat",
		Messages: []MessagesInputMessage{{
			Role:    "user",
			Content: json.RawMessage(`"Hello"`),
		}},
		Stream: true,
	})
	if err != nil {
		t.Fatalf("StreamMessage() error = %v", err)
	}
	defer func() {
		_ = stream.Close()
	}()

	var events []MessagesStreamEvent
	for stream.Next() {
		events = append(events, stream.Event())
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err() = %v", err)
	}
	if len(events) != 3 || events[1].Type != "content_block_delta" || len(events[1].Delta) == 0 {
		t.Fatalf("unexpected events %#v", events)
	}
	if events[0].Message == nil || events[0].Message.Usage.Source != "unavailable" {
		t.Fatalf("unexpected message usage source %#v", events)
	}
}
