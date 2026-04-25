package replicate

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

func TestVideoAdapterGenerateUsesPredictionsAPI(t *testing.T) {
	t.Parallel()

	type observedRequest struct {
		Path          string
		Authorization string
		Input         map[string]any
	}
	observed := observedRequest{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed.Path = r.URL.Path
		observed.Authorization = r.Header.Get("Authorization")
		var payload predictionCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		observed.Input = payload.Input
		_ = json.NewEncoder(w).Encode(predictionResponse{
			ID:     "pred_123",
			Status: "starting",
		})
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "r8_test",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewVideoAdapter(client, "replicate/minimax/video-01")

	job, err := adapter.Generate(context.Background(), &modality.VideoRequest{
		Model:       "replicate/minimax/video-01",
		Prompt:      "A paper airplane crossing a sunset skyline",
		FirstFrame:  "https://example.com/frame.png",
		Duration:    6,
		AspectRatio: "16:9",
		Resolution:  "720p",
		WithAudio:   true,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if observed.Path != "/models/minimax/video-01/predictions" {
		t.Fatalf("path = %q", observed.Path)
	}
	if observed.Authorization != "Bearer r8_test" {
		t.Fatalf("authorization = %q", observed.Authorization)
	}
	if observed.Input["prompt"] != "A paper airplane crossing a sunset skyline" {
		t.Fatalf("input prompt = %#v", observed.Input["prompt"])
	}
	if observed.Input["image"] != "https://example.com/frame.png" {
		t.Fatalf("input image = %#v", observed.Input["image"])
	}
	if job.JobID != "pred_123" || job.Status != "queued" {
		t.Fatalf("job = %#v", job)
	}
}

func TestVideoAdapterGetStatusParsesPrediction(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(predictionResponse{
			ID:          "pred_123",
			Status:      "succeeded",
			CreatedAt:   "2026-04-22T00:00:00Z",
			CompletedAt: "2026-04-22T00:01:00Z",
			Output:      []any{"https://replicate.delivery/video.mp4"},
		})
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "r8_test",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewVideoAdapter(client, "replicate/minimax/video-01")

	status, err := adapter.GetStatus(context.Background(), "pred_123")
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if status.Status != "completed" {
		t.Fatalf("status = %#v", status)
	}
	if status.Result == nil || status.Result.VideoURL != "https://replicate.delivery/video.mp4" {
		t.Fatalf("result = %#v", status.Result)
	}
	if status.ExpiresAt <= status.CompletedAt {
		t.Fatalf("expires_at = %d completed_at = %d", status.ExpiresAt, status.CompletedAt)
	}
}

func TestVideoAdapterDownloadIncludesAuthorization(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer r8_test" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "video/webm")
		_, _ = w.Write([]byte("video-bytes"))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "r8_test",
		BaseURL: "https://api.replicate.com/v1",
		Timeout: time.Second,
	})
	adapter := NewVideoAdapter(client, "replicate/minimax/video-01")

	asset, err := adapter.Download(context.Background(), "pred_123", &modality.VideoStatus{
		Result: &modality.VideoResult{VideoURL: server.URL},
	})
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if string(asset.Data) != "video-bytes" || asset.ContentType != "video/webm" {
		t.Fatalf("asset = %#v", asset)
	}
}

func TestParseProviderModelRejectsInvalidID(t *testing.T) {
	t.Parallel()

	if _, _, err := parseProviderModel("video-01"); err == nil || !strings.Contains(err.Error(), "owner/model") {
		t.Fatalf("expected owner/model error, got %v", err)
	}
}
