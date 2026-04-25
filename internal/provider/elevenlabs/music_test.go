package elevenlabs

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestMusicAdapterGeneratePlanAndStems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/music":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode generate payload: %v", err)
			}
			if payload["prompt"] != "Make a synthwave hook" || payload["model_id"] != "music_v1" {
				t.Fatalf("unexpected generate payload %#v", payload)
			}
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Header().Set("song-id", "song_123")
			_, _ = w.Write([]byte("music-bytes"))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/music/plan":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode plan payload: %v", err)
			}
			if payload["prompt"] != "Make a synthwave hook" {
				t.Fatalf("unexpected plan payload %#v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sections":[{"name":"intro"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/music/stem-separation":
			if !strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
				t.Fatalf("expected multipart stems request, got %q", r.Header.Get("Content-Type"))
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read stems body: %v", err)
			}
			if !strings.Contains(string(body), "source.mp3") {
				t.Fatalf("unexpected stems payload %q", string(body))
			}
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write([]byte("zip-bytes"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "eleven-key",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewMusicAdapter(client, "elevenlabs/music_v1")

	generated, err := adapter.Generate(context.Background(), &modality.MusicGenerationRequest{
		Model:        "elevenlabs/music_v1",
		Prompt:       "Make a synthwave hook",
		OutputFormat: "mp3",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if generated == nil || generated.Asset == nil || generated.SongID != "song_123" || string(generated.Asset.Data) != "music-bytes" {
		t.Fatalf("unexpected generate result %#v", generated)
	}

	plan, err := adapter.CreatePlan(context.Background(), &modality.MusicPlanRequest{
		Model:  "elevenlabs/music_v1",
		Prompt: "Make a synthwave hook",
	})
	if err != nil {
		t.Fatalf("CreatePlan() error = %v", err)
	}
	if string(plan.Plan) != `{"sections":[{"name":"intro"}]}` {
		t.Fatalf("unexpected plan %#v", plan)
	}

	stems, err := adapter.SeparateStems(context.Background(), &modality.MusicStemRequest{
		Model:       "elevenlabs/music_v1",
		File:        []byte("audio-bytes"),
		Filename:    "source.mp3",
		ContentType: "audio/mpeg",
	})
	if err != nil {
		t.Fatalf("SeparateStems() error = %v", err)
	}
	if stems == nil || stems.Asset == nil || stems.Asset.ContentType != "application/zip" || string(stems.Asset.Data) != "zip-bytes" {
		t.Fatalf("unexpected stems %#v", stems)
	}
}
