package google

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestImageAdapterGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-2.5-flash-image:generateContent" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "google-key" {
			t.Fatalf("unexpected x-goog-api-key header %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		generationConfig := payload["generationConfig"].(map[string]any)
		modalities := generationConfig["responseModalities"].([]any)
		if len(modalities) != 2 || modalities[1] != "IMAGE" {
			t.Fatalf("unexpected response modalities %#v", modalities)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidates":[{
				"index":0,
				"content":{"parts":[
					{"text":"A watercolor lighthouse at dusk"},
					{"inlineData":{"mimeType":"image/png","data":"AQID"}}
				]}
			}]
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "google-key",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewImageAdapter(client, "google/nano-banana-2")

	response, err := adapter.Generate(context.Background(), &modality.ImageRequest{
		Model:          "google/nano-banana-2",
		Prompt:         "A lighthouse at dusk",
		ResponseFormat: "url",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(response.Data) != 1 {
		t.Fatalf("expected 1 image item, got %#v", response.Data)
	}
	if !strings.HasPrefix(response.Data[0].URL, "data:image/png;base64,") {
		t.Fatalf("expected data URI image response, got %q", response.Data[0].URL)
	}
	if response.Data[0].RevisedPrompt != "A watercolor lighthouse at dusk" {
		t.Fatalf("unexpected revised prompt %q", response.Data[0].RevisedPrompt)
	}
}

func TestImageAdapterEditWithReferences(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-3-pro-image-preview:generateContent" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		contents := payload["contents"].([]any)
		parts := contents[0].(map[string]any)["parts"].([]any)
		if len(parts) < 2 {
			t.Fatalf("expected prompt + image parts, got %#v", parts)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidates":[{
				"index":0,
				"content":{"parts":[
					{"inlineData":{"mimeType":"image/png","data":"BAUG"}}
				]}
			}]
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "google-key",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewImageAdapter(client, "google/nano-banana-pro")

	response, err := adapter.Edit(context.Background(), &modality.ImageEditRequest{
		Model:          "google/nano-banana-pro",
		Prompt:         "Make it brighter",
		Image:          []byte("png-bytes"),
		ImageFilename:  "input.png",
		ImageType:      "image/png",
		ResponseFormat: "b64_json",
	})
	if err != nil {
		t.Fatalf("Edit() error = %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].B64JSON != "BAUG" {
		t.Fatalf("unexpected image response %#v", response)
	}
}
