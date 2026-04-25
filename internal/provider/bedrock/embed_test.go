package bedrock

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestEmbedAdapterEmbed(t *testing.T) {
	t.Parallel()

	type observedRequest struct {
		Path    string
		Payload embedRequest
	}

	observed := observedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed.Path = r.URL.Path
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "AWS4-HMAC-SHA256 ") {
			t.Fatalf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&observed.Payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(embedResponse{
			Embedding:           []float32{0.25, -0.5, 1.25},
			InputTextTokenCount: 7,
		})
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		BaseURL:         server.URL,
		Location:        "us-east-1",
		AccessKeyID:     "AKIAEXAMPLE",
		AccessKeySecret: "secret",
		Timeout:         time.Second,
	})
	adapter := NewEmbedAdapter(client, "bedrock/amazon.titan-embed-text-v2:0")
	dimensions := 512

	resp, err := adapter.Embed(context.Background(), &modality.EmbedRequest{
		Model:          "bedrock/amazon.titan-embed-text-v2:0",
		Input:          modality.NewSingleEmbedInput("hello"),
		Dimensions:     &dimensions,
		EncodingFormat: "base64",
	})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	if observed.Path != "/model/amazon.titan-embed-text-v2:0/invoke" {
		t.Fatalf("path = %q", observed.Path)
	}
	if observed.Payload.InputText != "hello" {
		t.Fatalf("inputText = %q", observed.Payload.InputText)
	}
	if observed.Payload.Dimensions == nil || *observed.Payload.Dimensions != 512 {
		t.Fatalf("dimensions = %#v", observed.Payload.Dimensions)
	}
	if !observed.Payload.Normalize {
		t.Fatal("expected normalize=true")
	}
	if resp.Model != "bedrock/amazon.titan-embed-text-v2:0" {
		t.Fatalf("model = %q", resp.Model)
	}
	if resp.Usage.PromptTokens != 7 || resp.Usage.TotalTokens != 7 {
		t.Fatalf("usage = %#v", resp.Usage)
	}
	if len(resp.Data) != 1 || resp.Data[0].Embedding.Base64 == "" {
		t.Fatalf("response = %#v", resp)
	}
	if _, err := base64.StdEncoding.DecodeString(resp.Data[0].Embedding.Base64); err != nil {
		t.Fatalf("decode base64 embedding: %v", err)
	}
}

func TestEmbedAdapterRejectsUnsupportedDimensions(t *testing.T) {
	t.Parallel()

	client := NewClient(config.ProviderConfig{
		BaseURL:         "https://example.com",
		Location:        "us-east-1",
		AccessKeyID:     "AKIAEXAMPLE",
		AccessKeySecret: "secret",
		Timeout:         time.Second,
	})
	adapter := NewEmbedAdapter(client, "bedrock/amazon.titan-embed-text-v2:0")
	dimensions := 384

	if _, err := adapter.Embed(context.Background(), &modality.EmbedRequest{
		Model:      "bedrock/amazon.titan-embed-text-v2:0",
		Input:      modality.NewSingleEmbedInput("hello"),
		Dimensions: &dimensions,
	}); err == nil {
		t.Fatal("expected dimensions error")
	}
}
