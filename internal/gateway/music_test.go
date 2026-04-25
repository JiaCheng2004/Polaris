package gateway

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
)

func TestMusicGenerationLifecycleAndUsage(t *testing.T) {
	audioHex := hex.EncodeToString([]byte("music-bytes"))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/music_generation":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"audio":"` + audioHex + `","status":0},"extra_info":{"music_duration":120000,"music_sample_rate":44100,"bitrate":128,"music_size":11}}`))
		case "/v1/lyrics_generation":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"song_title":"Skyline","style_tags":"pop","lyrics":"hello world"}`))
		default:
			t.Fatalf("unexpected upstream %s %s", r.Method, r.URL.Path)
		}
	}))
	defer upstream.Close()

	cfg := testMusicConfig(t, upstream.URL)
	sqliteStore := testSQLiteStore(t)
	registry, warnings, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	requestLogger := store.NewAsyncRequestLogger(sqliteStore, slog.New(slog.NewTextHandler(io.Discard, nil)), store.NewLoggerConfig(10, 5*time.Millisecond))

	engine, err := NewEngine(Dependencies{
		Config:        cfg,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:         sqliteStore,
		Cache:         cache.NewMemory(),
		Registry:      registry,
		RequestLogger: requestLogger,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	submitReq := httptest.NewRequest(http.MethodPost, "/v1/music/generations", strings.NewReader(`{"model":"default-music","mode":"async","prompt":"Make a synthwave hook","output_format":"mp3"}`))
	submitReq.Header.Set("Authorization", "Bearer alpha-key")
	submitReq.Header.Set("Content-Type", "application/json")
	submitRes := httptest.NewRecorder()
	engine.ServeHTTP(submitRes, submitReq)
	if submitRes.Code != http.StatusOK {
		t.Fatalf("expected submit 200, got %d body=%s", submitRes.Code, submitRes.Body.String())
	}

	var submitBody struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(submitRes.Body.Bytes(), &submitBody); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	if !strings.HasPrefix(submitBody.JobID, "mus_") {
		t.Fatalf("unexpected submit response %#v", submitBody)
	}

	var getBody struct {
		Status string `json:"status"`
		Result struct {
			DownloadURL string `json:"download_url"`
		} `json:"result"`
	}
	for i := 0; i < 20; i++ {
		getReq := httptest.NewRequest(http.MethodGet, "/v1/music/jobs/"+submitBody.JobID, nil)
		getReq.Header.Set("Authorization", "Bearer alpha-key")
		getRes := httptest.NewRecorder()
		engine.ServeHTTP(getRes, getReq)
		if getRes.Code != http.StatusOK {
			t.Fatalf("expected get 200, got %d body=%s", getRes.Code, getRes.Body.String())
		}
		if err := json.Unmarshal(getRes.Body.Bytes(), &getBody); err != nil {
			t.Fatalf("decode get response: %v", err)
		}
		if getBody.Status == "completed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if getBody.Status != "completed" || getBody.Result.DownloadURL == "" {
		t.Fatalf("unexpected get response %#v", getBody)
	}

	contentReq := httptest.NewRequest(http.MethodGet, "/v1/music/jobs/"+submitBody.JobID+"/content", nil)
	contentReq.Header.Set("Authorization", "Bearer alpha-key")
	contentRes := httptest.NewRecorder()
	engine.ServeHTTP(contentRes, contentReq)
	if contentRes.Code != http.StatusOK || contentRes.Body.String() != "music-bytes" {
		t.Fatalf("expected content 200 with bytes, got %d body=%s", contentRes.Code, contentRes.Body.String())
	}

	lyricsReq := httptest.NewRequest(http.MethodPost, "/v1/music/lyrics", strings.NewReader(`{"model":"default-music","prompt":"Write a chorus"}`))
	lyricsReq.Header.Set("Authorization", "Bearer alpha-key")
	lyricsReq.Header.Set("Content-Type", "application/json")
	lyricsRes := httptest.NewRecorder()
	engine.ServeHTTP(lyricsRes, lyricsReq)
	if lyricsRes.Code != http.StatusOK {
		t.Fatalf("expected lyrics 200, got %d body=%s", lyricsRes.Code, lyricsRes.Body.String())
	}

	if err := requestLogger.Close(context.Background()); err != nil {
		t.Fatalf("requestLogger.Close() error = %v", err)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/v1/usage?group_by=model&modality=music", nil)
	usageReq.Header.Set("Authorization", "Bearer alpha-key")
	usageRes := httptest.NewRecorder()
	engine.ServeHTTP(usageRes, usageReq)
	if usageRes.Code != http.StatusOK {
		t.Fatalf("expected usage 200, got %d body=%s", usageRes.Code, usageRes.Body.String())
	}
	var usageBody struct {
		TotalRequests int64 `json:"total_requests"`
	}
	if err := json.Unmarshal(usageRes.Body.Bytes(), &usageBody); err != nil {
		t.Fatalf("decode usage response: %v", err)
	}
	if usageBody.TotalRequests != 2 {
		t.Fatalf("expected async completion plus lyrics request to be logged, got %#v", usageBody)
	}
}

func TestMusicSyncTimeoutSuggestsAsync(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"audio":"00","status":0}}`))
	}))
	defer upstream.Close()

	cfg := testMusicConfig(t, upstream.URL)
	cfg.Providers["minimax"] = providerWithTimeout(cfg.Providers["minimax"], 20*time.Millisecond)

	engine := newMusicTestEngine(t, cfg)
	req := httptest.NewRequest(http.MethodPost, "/v1/music/generations", strings.NewReader(`{"model":"default-music","prompt":"slow synthwave hook","output_format":"mp3"}`))
	req.Header.Set("Authorization", "Bearer alpha-key")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d body=%s", res.Code, res.Body.String())
	}

	var body struct {
		Error struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode timeout response: %v", err)
	}
	if body.Error.Type != "timeout_error" || body.Error.Code != "provider_timeout" {
		t.Fatalf("unexpected timeout error %#v", body)
	}
	if !strings.Contains(body.Error.Message, "mode=async") {
		t.Fatalf("expected async hint in timeout message, got %q", body.Error.Message)
	}
}

func TestMusicAsyncTimeoutPreservesNormalizedError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"audio":"00","status":0}}`))
	}))
	defer upstream.Close()

	cfg := testMusicConfig(t, upstream.URL)
	cfg.Providers["minimax"] = providerWithTimeout(cfg.Providers["minimax"], 20*time.Millisecond)

	engine := newMusicTestEngine(t, cfg)
	submitReq := httptest.NewRequest(http.MethodPost, "/v1/music/generations", strings.NewReader(`{"model":"default-music","mode":"async","prompt":"slow synthwave hook","output_format":"mp3"}`))
	submitReq.Header.Set("Authorization", "Bearer alpha-key")
	submitReq.Header.Set("Content-Type", "application/json")
	submitRes := httptest.NewRecorder()
	engine.ServeHTTP(submitRes, submitReq)
	if submitRes.Code != http.StatusOK {
		t.Fatalf("expected submit 200, got %d body=%s", submitRes.Code, submitRes.Body.String())
	}

	var submitBody struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(submitRes.Body.Bytes(), &submitBody); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}

	var getBody struct {
		Status string `json:"status"`
		Error  struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	for i := 0; i < 30; i++ {
		getReq := httptest.NewRequest(http.MethodGet, "/v1/music/jobs/"+submitBody.JobID, nil)
		getReq.Header.Set("Authorization", "Bearer alpha-key")
		getRes := httptest.NewRecorder()
		engine.ServeHTTP(getRes, getReq)
		if getRes.Code != http.StatusOK {
			t.Fatalf("expected get 200, got %d body=%s", getRes.Code, getRes.Body.String())
		}
		if err := json.Unmarshal(getRes.Body.Bytes(), &getBody); err != nil {
			t.Fatalf("decode get response: %v", err)
		}
		if getBody.Status == "failed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if getBody.Status != "failed" {
		t.Fatalf("expected failed async job, got %#v", getBody)
	}
	if getBody.Error.Type != "timeout_error" || getBody.Error.Code != "provider_timeout" {
		t.Fatalf("unexpected async timeout error %#v", getBody)
	}
	if !strings.Contains(getBody.Error.Message, "mode=async") {
		t.Fatalf("expected async hint in timeout message, got %q", getBody.Error.Message)
	}
}

func TestMusicSyncGenerationCacheHeaders(t *testing.T) {
	audioHex := hex.EncodeToString([]byte("music-bytes"))
	var upstreamCalls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&upstreamCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"audio":"` + audioHex + `","status":0},"extra_info":{"music_duration":120000,"music_sample_rate":44100,"bitrate":128,"music_size":11}}`))
	}))
	defer upstream.Close()

	cfg := testMusicConfig(t, upstream.URL)
	cfg.Cache.ResponseCache.Enabled = true
	cfg.Cache.ResponseCache.TTL = time.Hour
	engine := newMusicTestEngine(t, cfg)

	for i, want := range []string{"miss", "hit"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/music/generations", strings.NewReader(`{"model":"default-music","prompt":"Make a synthwave hook","output_format":"mp3"}`))
		req.Header.Set("Authorization", "Bearer alpha-key")
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d body=%s", i+1, res.Code, res.Body.String())
		}
		if got := res.Header().Get("X-Polaris-Cache"); got != want {
			t.Fatalf("request %d: expected X-Polaris-Cache=%s, got %q", i+1, want, got)
		}
		if res.Body.String() != "music-bytes" {
			t.Fatalf("request %d: unexpected body %q", i+1, res.Body.String())
		}
	}
	if got := atomic.LoadInt32(&upstreamCalls); got != 1 {
		t.Fatalf("expected one upstream generation call, got %d", got)
	}
}

func testMusicConfig(t *testing.T, baseURL string) *config.Config {
	t.Helper()
	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "alpha",
		KeyHash:       middleware.HashAPIKey("alpha-key"),
		RateLimit:     "10/min",
		AllowedModels: []string{"minimax/*"},
	}}
	cfg.Providers = map[string]config.ProviderConfig{
		"minimax": {
			APIKey:  "minimax-key",
			BaseURL: baseURL,
			Timeout: time.Second,
			Models: map[string]config.ModelConfig{
				"music-2.6": {
					Modality:      modality.ModalityMusic,
					Capabilities:  []modality.Capability{modality.CapabilityMusicGeneration, modality.CapabilityLyricsGeneration, modality.CapabilityInstrumental},
					OutputFormats: []string{"mp3", "wav"},
					MinDurationMs: 10000,
					MaxDurationMs: 180000,
					SampleRatesHz: []int{44100},
				},
			},
		},
	}
	cfg.Routing.Aliases = map[string]string{
		"default-music": "minimax/music-2.6",
	}
	return cfg
}

func providerWithTimeout(cfg config.ProviderConfig, timeout time.Duration) config.ProviderConfig {
	cfg.Timeout = timeout
	return cfg
}

func newMusicTestEngine(t *testing.T, cfg *config.Config) http.Handler {
	t.Helper()
	sqliteStore := testSQLiteStore(t)
	registry, warnings, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	requestLogger := store.NewAsyncRequestLogger(sqliteStore, slog.New(slog.NewTextHandler(io.Discard, nil)), store.NewLoggerConfig(10, 5*time.Millisecond))
	t.Cleanup(func() {
		_ = requestLogger.Close(context.Background())
	})

	engine, err := NewEngine(Dependencies{
		Config:        cfg,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:         sqliteStore,
		Cache:         cache.NewMemory(),
		Registry:      registry,
		RequestLogger: requestLogger,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	return engine
}
