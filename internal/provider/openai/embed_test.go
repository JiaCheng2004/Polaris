package openai

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

func TestEmbedAdapterEmbed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("unexpected Authorization header %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "text-embedding-3-small" {
			t.Fatalf("expected stripped provider model, got %#v", payload["model"])
		}
		if payload["encoding_format"] != "base64" {
			t.Fatalf("expected encoding_format=base64, got %#v", payload["encoding_format"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object":"list",
			"data":[
				{"object":"embedding","index":0,"embedding":"AQID"},
				{"object":"embedding","index":1,"embedding":"BAUG"}
			],
			"model":"text-embedding-3-small",
			"usage":{"prompt_tokens":8,"total_tokens":8}
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewEmbedAdapter(client, "openai/text-embedding-3-small")

	response, err := adapter.Embed(context.Background(), &modality.EmbedRequest{
		Model:          "openai/text-embedding-3-small",
		Input:          modality.NewMultiEmbedInput("hello", "world"),
		EncodingFormat: "base64",
	})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if response.Model != "openai/text-embedding-3-small" {
		t.Fatalf("expected canonical model, got %q", response.Model)
	}
	if len(response.Data) != 2 || response.Data[0].Embedding.Base64 != "AQID" || response.Data[1].Embedding.Base64 != "BAUG" {
		t.Fatalf("unexpected embeddings %#v", response.Data)
	}
	if response.Usage.TotalTokens != 8 {
		t.Fatalf("expected total tokens 8, got %d", response.Usage.TotalTokens)
	}
}

func TestEmbedAdapterMapsRateLimitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"too many requests","type":"rate_limit_error","code":"rate_limit"}}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewEmbedAdapter(client, "openai/text-embedding-3-small")

	_, err := adapter.Embed(context.Background(), &modality.EmbedRequest{
		Model: "openai/text-embedding-3-small",
		Input: modality.NewSingleEmbedInput("hello"),
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	var apiErr *httputil.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Status != http.StatusTooManyRequests || apiErr.Type != "rate_limit_error" {
		t.Fatalf("unexpected api error %#v", apiErr)
	}
}
