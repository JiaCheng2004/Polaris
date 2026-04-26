package contracttest

import (
	"context"
	"encoding/binary"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/safeconv"
)

type NativeChatAdapterFactory func(cfg config.ProviderConfig, model string) modality.ChatAdapter

func RunBedrockNativeChatSuite(t *testing.T, canonicalModel string, wireModel string, factory NativeChatAdapterFactory) {
	t.Helper()

	t.Run("complete", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/model/"+wireModel+"/converse" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
				t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
			}
			if r.Header.Get("X-Amz-Date") == "" {
				t.Fatal("missing X-Amz-Date")
			}
			_, _ = w.Write([]byte(`{
				"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}},
				"stopReason":"end_turn",
				"usage":{"inputTokens":3,"outputTokens":2,"totalTokens":5}
			}`))
		}))
		defer server.Close()

		adapter := factory(config.ProviderConfig{
			BaseURL:         server.URL,
			Location:        "us-east-1",
			AccessKeyID:     "AKIAEXAMPLE",
			AccessKeySecret: "secret",
			Timeout:         time.Second,
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
			w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
			_, _ = w.Write(encodeBedrockEventFrame(t, []byte(`{"messageStart":{"role":"assistant"}}`)))
			_, _ = w.Write(encodeBedrockEventFrame(t, []byte(`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"hel"}}}`)))
			_, _ = w.Write(encodeBedrockEventFrame(t, []byte(`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"lo"}}}`)))
			_, _ = w.Write(encodeBedrockEventFrame(t, []byte(`{"messageStop":{"stopReason":"end_turn"}}`)))
			_, _ = w.Write(encodeBedrockEventFrame(t, []byte(`{"metadata":{"usage":{"inputTokens":3,"outputTokens":2,"totalTokens":5}}}`)))
		}))
		defer server.Close()

		adapter := factory(config.ProviderConfig{
			BaseURL:         server.URL,
			Location:        "us-east-1",
			AccessKeyID:     "AKIAEXAMPLE",
			AccessKeySecret: "secret",
			Timeout:         time.Second,
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
		if len(chunks) != 5 {
			t.Fatalf("chunk count = %d", len(chunks))
		}
		if chunks[0].Choices[0].Delta.Role != "assistant" {
			t.Fatalf("role chunk = %#v", chunks[0].Choices[0])
		}
		if chunks[4].Usage == nil || chunks[4].Usage.TotalTokens != 5 {
			t.Fatalf("usage chunk = %#v", chunks[4].Usage)
		}
	})
}

func encodeBedrockEventFrame(t *testing.T, payload []byte) []byte {
	t.Helper()

	totalLength := 16 + len(payload)
	totalLengthUint32, err := safeconv.Uint32FromInt("bedrock event frame length", totalLength)
	if err != nil {
		t.Fatalf("encode bedrock event frame: %v", err)
	}
	frame := make([]byte, totalLength)
	binary.BigEndian.PutUint32(frame[0:4], totalLengthUint32)
	binary.BigEndian.PutUint32(frame[4:8], 0)
	binary.BigEndian.PutUint32(frame[8:12], crc32.ChecksumIEEE(frame[0:8]))
	copy(frame[12:12+len(payload)], payload)
	binary.BigEndian.PutUint32(frame[len(frame)-4:], crc32.ChecksumIEEE(frame[:len(frame)-4]))
	return frame
}
