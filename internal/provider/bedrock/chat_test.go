package bedrock

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestChatAdapterCompleteTranslatesConverse(t *testing.T) {
	t.Parallel()

	type observedRequest struct {
		Path          string
		Authorization string
		XAmzDate      string
		Payload       converseRequest
	}

	observed := observedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed.Path = r.URL.Path
		observed.Authorization = r.Header.Get("Authorization")
		observed.XAmzDate = r.Header.Get("X-Amz-Date")
		if err := json.NewDecoder(r.Body).Decode(&observed.Payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		_ = json.NewEncoder(w).Encode(converseResponse{
			StopReason: "tool_use",
			Usage: struct {
				InputTokens  int `json:"inputTokens"`
				OutputTokens int `json:"outputTokens"`
				TotalTokens  int `json:"totalTokens"`
			}{
				InputTokens:  11,
				OutputTokens: 7,
				TotalTokens:  18,
			},
			Output: struct {
				Message bedrockMessage `json:"message"`
			}{
				Message: bedrockMessage{
					Role: "assistant",
					Content: []bedrockContentBlock{
						{Text: "hello"},
						{ToolUse: &bedrockToolUseBlock{
							ToolUseID: "tool_1",
							Name:      "lookup",
							Input:     map[string]any{"city": "sf"},
						}},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		BaseURL:         server.URL,
		Location:        "us-east-1",
		AccessKeyID:     "AKIAEXAMPLE",
		AccessKeySecret: "secret",
		SessionToken:    "token",
		Timeout:         time.Second,
	})
	adapter := NewChatAdapter(client, "bedrock/amazon.nova-2-lite-v1:0")

	temperature := 0.2
	resp, err := adapter.Complete(context.Background(), &modality.ChatRequest{
		Model:       "bedrock/amazon.nova-2-lite-v1:0",
		MaxTokens:   256,
		Temperature: &temperature,
		Messages: []modality.ChatMessage{
			{Role: "system", Content: modality.NewTextContent("be concise")},
			{
				Role: "user",
				Content: modality.NewPartContent(
					modality.ContentPart{Type: "text", Text: "describe"},
					modality.ContentPart{
						Type: "image_url",
						ImageURL: &modality.ImageURLPart{
							URL: "data:image/png;base64,ZmFrZQ==",
						},
					},
				),
			},
		},
		Tools: []modality.ToolDefinition{{
			Type: "function",
			Function: modality.FunctionDefinition{
				Name:        "lookup",
				Description: "Lookup a city",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
			},
		}},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if observed.Path != "/model/amazon.nova-2-lite-v1:0/converse" {
		t.Fatalf("path = %q", observed.Path)
	}
	if !strings.HasPrefix(observed.Authorization, "AWS4-HMAC-SHA256 ") {
		t.Fatalf("authorization = %q", observed.Authorization)
	}
	if observed.XAmzDate == "" {
		t.Fatal("missing X-Amz-Date header")
	}
	if len(observed.Payload.System) != 1 || observed.Payload.System[0].Text != "be concise" {
		t.Fatalf("system = %#v", observed.Payload.System)
	}
	if len(observed.Payload.Messages) != 1 || observed.Payload.Messages[0].Role != "user" {
		t.Fatalf("messages = %#v", observed.Payload.Messages)
	}
	if len(observed.Payload.Messages[0].Content) != 2 {
		t.Fatalf("user content length = %d", len(observed.Payload.Messages[0].Content))
	}
	if observed.Payload.Messages[0].Content[1].Image == nil || observed.Payload.Messages[0].Content[1].Image.Source.Bytes != "ZmFrZQ==" {
		t.Fatalf("image block = %#v", observed.Payload.Messages[0].Content[1].Image)
	}
	if observed.Payload.ToolConfig == nil || len(observed.Payload.ToolConfig.Tools) != 1 {
		t.Fatalf("tool config = %#v", observed.Payload.ToolConfig)
	}

	if resp.Model != "bedrock/amazon.nova-2-lite-v1:0" {
		t.Fatalf("response model = %q", resp.Model)
	}
	if resp.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("finish reason = %q", resp.Choices[0].FinishReason)
	}
	if len(resp.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("tool calls = %#v", resp.Choices[0].Message.ToolCalls)
	}
	if got := resp.Usage.Source; got != modality.TokenCountSourceProviderReported {
		t.Fatalf("usage source = %q", got)
	}
}

func TestChatAdapterStreamDecodesEventStream(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		_, _ = w.Write(encodeEventStreamFrame(t, []byte(`{"messageStart":{"role":"assistant"}}`)))
		_, _ = w.Write(encodeEventStreamFrame(t, []byte(`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"hel"}}}`)))
		_, _ = w.Write(encodeEventStreamFrame(t, []byte(`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"lo"}}}`)))
		_, _ = w.Write(encodeEventStreamFrame(t, []byte(`{"messageStop":{"stopReason":"end_turn"}}`)))
		_, _ = w.Write(encodeEventStreamFrame(t, []byte(`{"metadata":{"usage":{"inputTokens":5,"outputTokens":2,"totalTokens":7}}}`)))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		BaseURL:         server.URL,
		Location:        "us-east-1",
		AccessKeyID:     "AKIAEXAMPLE",
		AccessKeySecret: "secret",
		Timeout:         time.Second,
	})
	adapter := NewChatAdapter(client, "bedrock/amazon.nova-2-lite-v1:0")

	stream, err := adapter.Stream(context.Background(), &modality.ChatRequest{
		Model: "bedrock/amazon.nova-2-lite-v1:0",
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
			t.Fatalf("stream chunk error = %v", chunk.Err)
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) != 5 {
		t.Fatalf("chunk count = %d", len(chunks))
	}
	if chunks[0].Choices[0].Delta.Role != "assistant" {
		t.Fatalf("role chunk = %#v", chunks[0].Choices[0])
	}
	if chunks[1].Choices[0].Delta.Content != "hel" || chunks[2].Choices[0].Delta.Content != "lo" {
		t.Fatalf("content chunks = %#v %#v", chunks[1].Choices[0], chunks[2].Choices[0])
	}
	if finish := chunks[3].Choices[0].FinishReason; finish == nil || *finish != "stop" {
		t.Fatalf("finish chunk = %#v", chunks[3].Choices[0])
	}
	if chunks[4].Usage == nil || chunks[4].Usage.TotalTokens != 7 {
		t.Fatalf("usage chunk = %#v", chunks[4].Usage)
	}
}

func encodeEventStreamFrame(t *testing.T, payload []byte) []byte {
	t.Helper()

	totalLength := 16 + len(payload)
	frame := make([]byte, totalLength)
	binary.BigEndian.PutUint32(frame[0:4], uint32(totalLength))
	binary.BigEndian.PutUint32(frame[4:8], 0)
	binary.BigEndian.PutUint32(frame[8:12], crc32.ChecksumIEEE(frame[0:8]))
	copy(frame[12:12+len(payload)], payload)
	binary.BigEndian.PutUint32(frame[len(frame)-4:], crc32.ChecksumIEEE(frame[:len(frame)-4]))
	return frame
}
