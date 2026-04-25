package minimax

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestMusicAdapterLyricsBusinessErrorReturnsProviderAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"base_resp":{"status_code":2049,"status_msg":"invalid api key"}}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "bad-key",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewMusicAdapter(client, "minimax/music-2.6")

	_, err := adapter.GenerateLyrics(context.Background(), &modality.MusicLyricsRequest{
		Model:  "minimax/music-2.6",
		Prompt: "hello",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "invalid api key" {
		t.Fatalf("unexpected error %q", got)
	}
}
