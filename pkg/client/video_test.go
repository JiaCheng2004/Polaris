package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateVideoGeneration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/video/generations" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var payload VideoGenerationRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.LastFrame != "https://example.com/end.png" || len(payload.ReferenceVideos) != 1 || payload.Audio != "asset://audio_123" {
			t.Fatalf("unexpected payload %#v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"job_id":"vid_123","status":"queued","estimated_time":12,"model":"bytedance/seedance-2.0"}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, WithAPIKey("secret"))
	job, err := client.CreateVideoGeneration(context.Background(), &VideoGenerationRequest{
		Model:      "default-video",
		Prompt:     "A cinematic skyline",
		Duration:   8,
		Resolution: "720p",
		WithAudio:  true,
		FirstFrame: "https://example.com/frame.png",
		LastFrame:  "https://example.com/end.png",
		ReferenceVideos: []string{
			"asset://video_123",
		},
		Audio: "asset://audio_123",
	})
	if err != nil {
		t.Fatalf("CreateVideoGeneration() error = %v", err)
	}
	if job.JobID != "vid_123" || job.Status != "queued" || job.Model != "bytedance/seedance-2.0" {
		t.Fatalf("unexpected job %#v", job)
	}
}

func TestGetAndCancelVideoGeneration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/video/generations/vid_123":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"job_id":"vid_123","status":"completed","progress":1,"created_at":1712697600,"completed_at":1712697612,"result":{"video_url":"https://cdn.example.com/video.mp4","download_url":"http://gateway/v1/video/generations/vid_123/content","content_type":"video/mp4"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/video/generations/vid_123/content":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video-bytes"))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/video/generations/vid_123":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, WithAPIKey("secret"))

	status, err := client.GetVideoGeneration(context.Background(), "vid_123")
	if err != nil {
		t.Fatalf("GetVideoGeneration() error = %v", err)
	}
	if status.Status != "completed" || status.Result == nil || status.Result.VideoURL == "" || status.Result.DownloadURL == "" || status.CreatedAt == 0 || status.CompletedAt == 0 {
		t.Fatalf("unexpected status %#v", status)
	}

	asset, err := client.GetVideoGenerationContent(context.Background(), "vid_123")
	if err != nil {
		t.Fatalf("GetVideoGenerationContent() error = %v", err)
	}
	if string(asset.Data) != "video-bytes" || asset.ContentType != "video/mp4" {
		t.Fatalf("unexpected video asset %#v", asset)
	}

	if err := client.CancelVideoGeneration(context.Background(), "vid_123"); err != nil {
		t.Fatalf("CancelVideoGeneration() error = %v", err)
	}
}
