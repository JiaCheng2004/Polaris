package openai

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

func TestVideoAdapterGenerateStatusAndDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/videos":
			if got := r.Header.Get("Authorization"); got != "Bearer sk-openai" {
				t.Fatalf("unexpected auth header %q", got)
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode create request: %v", err)
			}
			if payload["model"] != "sora-2" || payload["seconds"] != "8" || payload["size"] != "1280x720" {
				t.Fatalf("unexpected create payload %#v", payload)
			}
			inputReference, ok := payload["input_reference"].(map[string]any)
			if !ok || !strings.HasPrefix(inputReference["image_url"].(string), "data:image/png;base64,") {
				t.Fatalf("unexpected input_reference %#v", payload["input_reference"])
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"video_123","status":"queued","model":"sora-2"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/videos/video_123":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"video_123","status":"completed","progress":100,"created_at":1712697600,"completed_at":1712697610,"expires_at":1712699999,"size":"1280x720","seconds":"8"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/videos/video_123/content":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video-bytes"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-openai",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewVideoAdapter(client, "openai/sora-2")

	job, err := adapter.Generate(context.Background(), &modality.VideoRequest{
		Model:       "openai/sora-2",
		Prompt:      "A rain-soaked cyberpunk alley",
		Duration:    8,
		AspectRatio: "16:9",
		Resolution:  "720p",
		FirstFrame:  "Zmlyc3QtZnJhbWU=",
		WithAudio:   true,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if job.JobID != "video_123" || job.Status != "queued" {
		t.Fatalf("unexpected job %#v", job)
	}

	status, err := adapter.GetStatus(context.Background(), "video_123")
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if status.Status != "completed" || status.Result == nil || status.Result.ContentType != "video/mp4" || status.Result.Width != 1280 || status.Result.Height != 720 {
		t.Fatalf("unexpected status %#v", status)
	}

	asset, err := adapter.Download(context.Background(), "video_123", status)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if string(asset.Data) != "video-bytes" || asset.ContentType != "video/mp4" {
		t.Fatalf("unexpected asset %#v", asset)
	}
}
