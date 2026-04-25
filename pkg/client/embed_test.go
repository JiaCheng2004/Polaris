package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateEmbedding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		var payload EmbeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "default-embed" {
			t.Fatalf("unexpected model %q", payload.Model)
		}
		if got := payload.Input.Values(); len(got) != 2 || got[0] != "hello" || got[1] != "world" {
			t.Fatalf("unexpected input %#v", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object":"list",
			"data":[{"object":"embedding","index":0,"embedding":[0.1,0.2]}],
			"model":"openai/text-embedding-3-small",
			"usage":{"prompt_tokens":8,"total_tokens":8,"source":"provider_reported"}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	response, err := client.CreateEmbedding(context.Background(), &EmbeddingRequest{
		Model:          "default-embed",
		Input:          NewMultiEmbeddingInput("hello", "world"),
		EncodingFormat: "float",
	})
	if err != nil {
		t.Fatalf("CreateEmbedding() error = %v", err)
	}
	if response.Model != "openai/text-embedding-3-small" || response.Usage.TotalTokens != 8 {
		t.Fatalf("unexpected response %#v", response)
	}
	if response.Usage.Source != "provider_reported" {
		t.Fatalf("unexpected usage source %#v", response.Usage)
	}
	if len(response.Data) != 1 || len(response.Data[0].Embedding.Float32) != 2 {
		t.Fatalf("unexpected embedding payload %#v", response.Data)
	}
}
