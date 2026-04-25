package google

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestEmbedAdapterEmbedSingle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-embedding-001:embedContent" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "google-key" {
			t.Fatalf("unexpected x-goog-api-key header %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "models/gemini-embedding-001" {
			t.Fatalf("unexpected request model %#v", payload["model"])
		}
		if payload["outputDimensionality"] != float64(768) {
			t.Fatalf("expected outputDimensionality 768, got %#v", payload["outputDimensionality"])
		}
		content := payload["content"].(map[string]any)
		parts := content["parts"].([]any)
		if len(parts) != 1 {
			t.Fatalf("expected 1 content part, got %d", len(parts))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"embedding":{"values":[1.5,2.25]},
			"usageMetadata":{"promptTokenCount":4,"totalTokenCount":4}
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "google-key",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewEmbedAdapter(client, "google/gemini-embedding-001")
	dimensions := 768

	response, err := adapter.Embed(context.Background(), &modality.EmbedRequest{
		Model:          "google/gemini-embedding-001",
		Input:          modality.NewSingleEmbedInput("hello"),
		Dimensions:     &dimensions,
		EncodingFormat: "base64",
	})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if response.Model != "google/gemini-embedding-001" {
		t.Fatalf("expected canonical model, got %q", response.Model)
	}
	if len(response.Data) != 1 {
		t.Fatalf("expected 1 embedding, got %d", len(response.Data))
	}
	expected := base64.StdEncoding.EncodeToString(float32Bytes([]float32{1.5, 2.25}))
	if response.Data[0].Embedding.Base64 != expected {
		t.Fatalf("unexpected base64 embedding %q", response.Data[0].Embedding.Base64)
	}
	if response.Usage.TotalTokens != 4 {
		t.Fatalf("expected usage total_tokens 4, got %d", response.Usage.TotalTokens)
	}
}

func TestEmbedAdapterEmbedBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-embedding-001:batchEmbedContents" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "google-key" {
			t.Fatalf("unexpected x-goog-api-key header %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requests := payload["requests"].([]any)
		if len(requests) != 2 {
			t.Fatalf("expected 2 batch requests, got %d", len(requests))
		}
		first := requests[0].(map[string]any)
		if first["model"] != "models/gemini-embedding-001" {
			t.Fatalf("unexpected request model %#v", first["model"])
		}
		if first["outputDimensionality"] != float64(768) {
			t.Fatalf("expected outputDimensionality 768, got %#v", first["outputDimensionality"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"embeddings":[
				{"values":[1.5,2.25]},
				{"values":[3.5,4.75]}
			]
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "google-key",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewEmbedAdapter(client, "google/gemini-embedding-001")
	dimensions := 768

	response, err := adapter.Embed(context.Background(), &modality.EmbedRequest{
		Model:          "google/gemini-embedding-001",
		Input:          modality.NewMultiEmbedInput("hello", "world"),
		Dimensions:     &dimensions,
		EncodingFormat: "base64",
	})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(response.Data) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(response.Data))
	}
	expected := base64.StdEncoding.EncodeToString(float32Bytes([]float32{1.5, 2.25}))
	if response.Data[0].Embedding.Base64 != expected {
		t.Fatalf("unexpected first base64 embedding %q", response.Data[0].Embedding.Base64)
	}
	if response.Usage.TotalTokens != 0 {
		t.Fatalf("expected zero token usage when batch provider usage is absent, got %d", response.Usage.TotalTokens)
	}
}

func float32Bytes(values []float32) []byte {
	buf := make([]byte, len(values)*4)
	for i, value := range values {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(value))
	}
	return buf
}
