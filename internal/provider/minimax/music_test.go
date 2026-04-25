package minimax

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestMusicAdapterGenerateAndLyrics(t *testing.T) {
	audioHex := hex.EncodeToString([]byte("music-bytes"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/music_generation":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode generate payload: %v", err)
			}
			if payload["model"] != "music-2.6" || payload["output_format"] != "hex" {
				t.Fatalf("unexpected generate payload %#v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"audio":"` + audioHex + `","status":0},"extra_info":{"music_duration":120000,"music_sample_rate":44100,"bitrate":128,"music_size":11}}`))
		case "/v1/lyrics_generation":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode lyrics payload: %v", err)
			}
			if payload["prompt"] != "Write a pop chorus" {
				t.Fatalf("unexpected lyrics payload %#v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"song_title":"Skyline","style_tags":"pop","lyrics":"hello world"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "minimax-key",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewMusicAdapter(client, "minimax/music-2.6")

	result, err := adapter.Generate(context.Background(), &modality.MusicGenerationRequest{
		Model:        "minimax/music-2.6",
		Prompt:       "Write a pop chorus",
		OutputFormat: "mp3",
		SampleRateHz: 44100,
		Bitrate:      128,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result == nil || result.Asset == nil || string(result.Asset.Data) != "music-bytes" || result.DurationMS != 120000 || result.SampleRateHz != 44100 {
		t.Fatalf("unexpected result %#v", result)
	}

	lyrics, err := adapter.GenerateLyrics(context.Background(), &modality.MusicLyricsRequest{
		Model:  "minimax/music-2.6",
		Prompt: "Write a pop chorus",
	})
	if err != nil {
		t.Fatalf("GenerateLyrics() error = %v", err)
	}
	if lyrics.Title != "Skyline" || lyrics.StyleTags != "pop" || lyrics.Lyrics != "hello world" {
		t.Fatalf("unexpected lyrics %#v", lyrics)
	}
}

func TestNewClientUsesGlobalDefaultBaseURL(t *testing.T) {
	client := NewClient(config.ProviderConfig{
		APIKey: "minimax-key",
	})
	if client.baseURL != "https://api.minimax.io" {
		t.Fatalf("expected global default base URL, got %q", client.baseURL)
	}
}
