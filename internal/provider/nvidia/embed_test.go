package nvidia

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

func TestEmbedAdapterEmbed(t *testing.T) {
	t.Parallel()

	type observedRequest struct {
		Path    string
		Payload map[string]any
	}

	observed := observedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed.Path = r.URL.Path
		if got := r.Header.Get("Authorization"); got != "Bearer nv-key" {
			t.Fatalf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&observed.Payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{
					"object":    "embedding",
					"index":     0,
					"embedding": []float32{1, 2, 3},
				},
			},
			"model": "nvidia/llama-nemotron-embed-1b-v2",
			"usage": map[string]any{
				"prompt_tokens": 4,
				"total_tokens":  4,
			},
		})
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "nv-key",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewEmbedAdapter(client, "nvidia/nvidia/llama-nemotron-embed-1b-v2")

	resp, err := adapter.Embed(context.Background(), &modality.EmbedRequest{
		Model: "nvidia/nvidia/llama-nemotron-embed-1b-v2",
		Input: modality.NewSingleEmbedInput("hello"),
	})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	if observed.Path != "/v1/embeddings" {
		t.Fatalf("path = %q", observed.Path)
	}
	if observed.Payload["model"] != "nvidia/llama-nemotron-embed-1b-v2" {
		t.Fatalf("model payload = %#v", observed.Payload["model"])
	}
	if resp.Model != "nvidia/nvidia/llama-nemotron-embed-1b-v2" {
		t.Fatalf("model = %q", resp.Model)
	}
	if len(resp.Data) != 1 || len(resp.Data[0].Embedding.Float32) != 3 {
		t.Fatalf("response = %#v", resp)
	}
}
