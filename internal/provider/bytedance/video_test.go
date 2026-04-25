package bytedance

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

func TestVideoAdapterGenerateAndNormalizeJob(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if r.URL.Path != "/api/v3/contents/generations/tasks" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer ark-key" {
			t.Fatalf("unexpected Authorization header %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "doubao-seedance-2-0-260128" {
			t.Fatalf("unexpected model %v", payload["model"])
		}
		if payload["ratio"] != "16:9" {
			t.Fatalf("unexpected ratio %v", payload["ratio"])
		}
		if payload["duration"] != float64(8) {
			t.Fatalf("unexpected duration %v", payload["duration"])
		}
		if payload["generate_audio"] != true {
			t.Fatalf("unexpected generate_audio %v", payload["generate_audio"])
		}
		content, ok := payload["content"].([]any)
		if !ok || len(content) != 6 {
			t.Fatalf("unexpected content %#v", payload["content"])
		}
		firstFrame := content[1].(map[string]any)
		if firstFrame["role"] != "first_frame" || firstFrame["type"] != "image_url" {
			t.Fatalf("unexpected first frame %#v", firstFrame)
		}
		lastFrame := content[2].(map[string]any)
		if lastFrame["role"] != "last_frame" || lastFrame["type"] != "image_url" {
			t.Fatalf("unexpected last frame %#v", lastFrame)
		}
		referenceImage := content[3].(map[string]any)
		if referenceImage["role"] != "reference_image" || referenceImage["type"] != "image_url" {
			t.Fatalf("unexpected reference image %#v", referenceImage)
		}
		referenceVideo := content[4].(map[string]any)
		if referenceVideo["role"] != "reference_video" || referenceVideo["type"] != "video_url" {
			t.Fatalf("unexpected reference video %#v", referenceVideo)
		}
		audio := content[5].(map[string]any)
		if audio["role"] != "reference_audio" || audio["type"] != "audio_url" {
			t.Fatalf("unexpected audio reference %#v", audio)
		}
		audioURL, ok := audio["audio_url"].(map[string]any)
		if !ok || audioURL["url"] != "data:audio/wav;base64,QVVESU8=" {
			t.Fatalf("unexpected audio_url payload %#v", audio["audio_url"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"task_123","status":"submitted","estimated_time":42}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "ark-key",
		BaseURL: server.URL + "/api/v3",
		Timeout: time.Second,
	})
	adapter := NewVideoAdapter(client, "bytedance/seedance-2.0", server.URL+"/api/v3")

	job, err := adapter.Generate(context.Background(), &modality.VideoRequest{
		Model:           "bytedance/seedance-2.0",
		Prompt:          "A crane shot over a neon city",
		Duration:        8,
		AspectRatio:     "16:9",
		Resolution:      "720p",
		FirstFrame:      "https://example.com/frame.png",
		LastFrame:       "asset://ending_frame",
		ReferenceImages: []string{"YmFzZTY0LWRhdGE="},
		ReferenceVideos: []string{"asset://reference_video"},
		Audio:           "QVVESU8=",
		WithAudio:       true,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if job.JobID != "task_123" || job.Status != "queued" || job.EstimatedTime != 42 {
		t.Fatalf("unexpected job %#v", job)
	}
}

func TestVideoAdapterGenerateMapsCurrentModelName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "doubao-seedance-2-0-260128" {
			t.Fatalf("unexpected model %v", payload["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"task_456","status":"submitted"}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "ark-key",
		BaseURL: server.URL + "/api/v3",
		Timeout: time.Second,
	})
	adapter := NewVideoAdapter(client, "bytedance/doubao-seedance-2.0", server.URL+"/api/v3")

	job, err := adapter.Generate(context.Background(), &modality.VideoRequest{
		Model:       "bytedance/doubao-seedance-2.0",
		Prompt:      "A cinematic river scene",
		Duration:    4,
		AspectRatio: "16:9",
		Resolution:  "720p",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if job.JobID != "task_456" || job.Status != "queued" {
		t.Fatalf("unexpected job %#v", job)
	}
}

func TestBuildVideoContentSupportsStructuredParityInputs(t *testing.T) {
	content := buildVideoContent(&modality.VideoRequest{
		Prompt:          "Prompt",
		FirstFrame:      "asset://start_frame",
		LastFrame:       "https://example.com/end.png",
		ReferenceImages: []string{"data:image/png;base64,AAA="},
		ReferenceVideos: []string{"asset://video_123"},
		Audio:           "asset://audio_123",
	})

	if len(content) != 6 {
		t.Fatalf("expected 6 content items, got %#v", content)
	}
	if content[1].ImageURL == nil || content[1].ImageURL.URL != "asset://start_frame" {
		t.Fatalf("unexpected first frame %#v", content[1])
	}
	if content[2].ImageURL == nil || content[2].ImageURL.URL != "https://example.com/end.png" {
		t.Fatalf("unexpected last frame %#v", content[2])
	}
	if content[4].VideoURL == nil || content[4].VideoURL.URL != "asset://video_123" {
		t.Fatalf("unexpected reference video %#v", content[4])
	}
	if content[5].AudioURL == nil || content[5].AudioURL.URL != "asset://audio_123" {
		t.Fatalf("unexpected reference audio %#v", content[5])
	}
}

func TestVideoAdapterGetStatusNormalizesCompletedPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if r.URL.Path != "/api/v3/contents/generations/tasks/task_123" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"task_123",
			"status":"succeeded",
			"progress":100,
			"content":{
				"video_url":"https://cdn.example.com/video.mp4",
				"audio_url":"https://cdn.example.com/audio.mp3",
				"duration":8,
				"width":1920,
				"height":1080
			}
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "ark-key",
		BaseURL: server.URL + "/api/v3",
		Timeout: time.Second,
	})
	adapter := NewVideoAdapter(client, "bytedance/seedance-2.0", server.URL+"/api/v3")

	status, err := adapter.GetStatus(context.Background(), "task_123")
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if status.Status != "completed" || status.Progress != 1 {
		t.Fatalf("unexpected status %#v", status)
	}
	if status.Result == nil || status.Result.VideoURL != "https://cdn.example.com/video.mp4" || status.Result.AudioURL != "https://cdn.example.com/audio.mp3" {
		t.Fatalf("unexpected result %#v", status.Result)
	}
}

func TestVideoAdapterGetStatusMapsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"task_123",
			"status":"failed",
			"error":{"type":"provider_error","code":"task_failed","message":"render failed"}
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "ark-key",
		BaseURL: server.URL + "/api/v3",
		Timeout: time.Second,
	})
	adapter := NewVideoAdapter(client, "bytedance/seedance-2.0", server.URL+"/api/v3")

	status, err := adapter.GetStatus(context.Background(), "task_123")
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if status.Status != "failed" || status.Error == nil || status.Error.Code != "task_failed" {
		t.Fatalf("unexpected failure status %#v", status)
	}
}

func TestVideoAdapterCancelMapsErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/contents/generations/tasks/task_done":
			w.WriteHeader(http.StatusConflict)
		case "/api/v3/contents/generations/tasks/task_missing":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "ark-key",
		BaseURL: server.URL + "/api/v3",
		Timeout: time.Second,
	})
	adapter := NewVideoAdapter(client, "bytedance/seedance-2.0", server.URL+"/api/v3")

	if err := adapter.Cancel(context.Background(), "task_ok"); err != nil {
		t.Fatalf("Cancel(task_ok) error = %v", err)
	}

	err := adapter.Cancel(context.Background(), "task_done")
	var apiErr *httputil.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Status != http.StatusConflict || apiErr.Code != "job_immutable" {
		t.Fatalf("unexpected conflict error %#v", apiErr)
	}

	err = adapter.Cancel(context.Background(), "task_missing")
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Status != http.StatusNotFound || apiErr.Code != "job_not_found" {
		t.Fatalf("unexpected not found error %#v", apiErr)
	}
}
