package bytedance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestImageAdapterGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/images/generations" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer ark-key" {
			t.Fatalf("unexpected Authorization header %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "doubao-seedream-4.5" {
			t.Fatalf("expected provider model, got %#v", payload["model"])
		}
		images, ok := payload["image"].([]any)
		if !ok || len(images) != 2 {
			t.Fatalf("expected 2 reference images, got %#v", payload["image"])
		}
		if payload["sequential_image_generation"] != "auto" {
			t.Fatalf("expected sequential_image_generation auto, got %#v", payload["sequential_image_generation"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"created":1744329600,
			"data":[{"b64_json":"AQID"},{"b64_json":"BAUG"}]
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "ark-key",
		BaseURL: server.URL + "/api/v3",
		Timeout: time.Second,
	})
	adapter := NewImageAdapter(client, "bytedance/seedream-4.5", server.URL+"/api/v3")

	response, err := adapter.Generate(context.Background(), &modality.ImageRequest{
		Model:           "bytedance/seedream-4.5",
		Prompt:          "A lantern festival",
		N:               2,
		ResponseFormat:  "b64_json",
		ReferenceImages: []string{"https://example.com/ref-1.png", "ZmFrZS1iNjQ="},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if response.Created != 1744329600 || len(response.Data) != 2 || response.Data[0].B64JSON != "AQID" {
		t.Fatalf("unexpected image response %#v", response)
	}
}

func TestImageAdapterGenerateMapsCurrentModelName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "doubao-seedream-5-0-lite-260128" {
			t.Fatalf("expected current provider model, got %#v", payload["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://example.com/seedream-5.png"}]}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "ark-key",
		BaseURL: server.URL + "/api/v3",
		Timeout: time.Second,
	})
	adapter := NewImageAdapter(client, "bytedance/doubao-seedream-5.0-lite", server.URL+"/api/v3")

	response, err := adapter.Generate(context.Background(), &modality.ImageRequest{
		Model:          "bytedance/doubao-seedream-5.0-lite",
		Prompt:         "A futuristic city skyline",
		ResponseFormat: "url",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].URL != "https://example.com/seedream-5.png" {
		t.Fatalf("unexpected image response %#v", response)
	}
}

func TestImageAdapterEdit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/images/generations" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		images, ok := payload["image"].([]any)
		if !ok || len(images) != 2 {
			t.Fatalf("expected image+mask reference array, got %#v", payload["image"])
		}
		if !strings.HasPrefix(images[0].(string), "data:image/png;base64,") {
			t.Fatalf("expected data URI image, got %#v", images[0])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data":[{"url":"https://example.com/seedream.png"}]
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "ark-key",
		BaseURL: server.URL + "/api/v3",
		Timeout: time.Second,
	})
	adapter := NewImageAdapter(client, "bytedance/seedream-4.5", server.URL+"/api/v3")

	response, err := adapter.Edit(context.Background(), &modality.ImageEditRequest{
		Model:          "bytedance/seedream-4.5",
		Prompt:         "Turn this into a poster",
		Image:          []byte("image-bytes"),
		ImageFilename:  "input.png",
		ImageType:      "image/png",
		Mask:           []byte("mask-bytes"),
		MaskFilename:   "mask.png",
		MaskType:       "image/png",
		ResponseFormat: "url",
	})
	if err != nil {
		t.Fatalf("Edit() error = %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].URL != "https://example.com/seedream.png" {
		t.Fatalf("unexpected image response %#v", response)
	}
}

func TestImageAdapterMapsRateLimitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"too many requests","code":"rate_limit"}}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "ark-key",
		BaseURL: server.URL + "/api/v3",
		Timeout: time.Second,
	})
	adapter := NewImageAdapter(client, "bytedance/seedream-4.5", server.URL+"/api/v3")

	_, err := adapter.Generate(context.Background(), &modality.ImageRequest{
		Model:  "bytedance/seedream-4.5",
		Prompt: "hello",
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
