package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateMusicGenerationAndJobLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/music/generations":
			var payload MusicGenerationRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload.Mode == "async" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"job_id":"mus_123","status":"queued","model":"minimax/music-2.6","operation":"generate"}`))
				return
			}
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("music-bytes"))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/music/jobs/mus_123":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"job_id":"mus_123","status":"completed","result":{"download_url":"http://gateway/v1/music/jobs/mus_123/content","content_type":"audio/mpeg"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/music/jobs/mus_123/content":
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("music-bytes"))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/music/jobs/mus_123":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, WithAPIKey("secret"))

	syncResp, err := client.CreateMusicGeneration(context.Background(), &MusicGenerationRequest{
		Model:  "default-music",
		Prompt: "Make a synthwave hook",
	})
	if err != nil {
		t.Fatalf("CreateMusicGeneration(sync) error = %v", err)
	}
	if syncResp.Asset == nil || string(syncResp.Asset.Data) != "music-bytes" {
		t.Fatalf("unexpected sync response %#v", syncResp)
	}

	asyncResp, err := client.CreateMusicGeneration(context.Background(), &MusicGenerationRequest{
		Mode:   "async",
		Model:  "default-music",
		Prompt: "Make a synthwave hook",
	})
	if err != nil {
		t.Fatalf("CreateMusicGeneration(async) error = %v", err)
	}
	if asyncResp.Job == nil || asyncResp.Job.JobID != "mus_123" {
		t.Fatalf("unexpected async response %#v", asyncResp)
	}

	status, err := client.GetMusicJob(context.Background(), "mus_123")
	if err != nil {
		t.Fatalf("GetMusicJob() error = %v", err)
	}
	if status.Status != "completed" || status.Result == nil || status.Result.DownloadURL == "" {
		t.Fatalf("unexpected music status %#v", status)
	}

	asset, err := client.GetMusicJobContent(context.Background(), "mus_123")
	if err != nil {
		t.Fatalf("GetMusicJobContent() error = %v", err)
	}
	if string(asset.Data) != "music-bytes" {
		t.Fatalf("unexpected music asset %#v", asset)
	}

	if err := client.CancelMusicJob(context.Background(), "mus_123"); err != nil {
		t.Fatalf("CancelMusicJob() error = %v", err)
	}
}

func TestStreamMusicEditUsesMultipart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/music/edits" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if !strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Fatalf("expected multipart content type, got %q", r.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if !strings.Contains(string(body), "cover") || !strings.Contains(string(body), "source.mp3") {
			t.Fatalf("unexpected multipart body %q", string(body))
		}
		if !strings.Contains(string(body), "\"providers\":[\"minimax\"]") {
			t.Fatalf("missing routing payload in multipart body %q", string(body))
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("stream-bytes"))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, WithAPIKey("secret"))
	stream, err := client.StreamMusicEdit(context.Background(), &MusicEditRequest{
		Model:       "cover-music",
		Routing:     &RoutingOptions{Providers: []string{"minimax"}},
		Operation:   "cover",
		File:        []byte("audio-bytes"),
		Filename:    "source.mp3",
		ContentType: "audio/mpeg",
	})
	if err != nil {
		t.Fatalf("StreamMusicEdit() error = %v", err)
	}
	defer func() {
		_ = stream.Body.Close()
	}()
	payload, err := io.ReadAll(stream.Body)
	if err != nil {
		t.Fatalf("read stream body: %v", err)
	}
	if string(payload) != "stream-bytes" || stream.ContentType != "audio/mpeg" {
		t.Fatalf("unexpected stream %#v payload=%q", stream, string(payload))
	}
}
