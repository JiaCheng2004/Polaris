package gateway

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/JiaCheng2004/Polaris/internal/store/sqlite"
)

// BenchmarkGatewayOverhead measures the request-path cost of the full Gin
// middleware + handler stack. It is the pre-overhaul baseline referenced by the
// plan (§7.4); each frontier phase re-runs it (feature-off vs feature-on) and
// the CI benchstat gate fails a regression >10%.
//
//   - ErrorPath drives a request that fails resolution (missing model -> 400)
//     so it exercises recovery/requestid/tracing/runtime/bodylimit/cors/logger/
//     metrics/auth/ratelimit/budget/usage + handler bind+validate with zero
//     provider I/O — i.e. pure gateway overhead.
//   - ChatCompletion drives the full happy path including a loopback httptest
//     provider, so it also includes adapter translation + the mock round-trip.
func BenchmarkGatewayOverhead(b *testing.B) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-bench","object":"chat.completion","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()

	engine := benchEngine(b, goldenConfig(upstream.URL+"/v1", upstream.URL))

	cases := []struct {
		name string
		body string
		want int
	}{
		{name: "ErrorPath", body: `{"model":"openai/does-not-exist","messages":[{"role":"user","content":"ping"}]}`, want: http.StatusNotFound},
		{name: "ChatCompletion", body: `{"model":"openai/gpt-4o","messages":[{"role":"user","content":"ping"}]}`, want: http.StatusOK},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
				engine.ServeHTTP(rec, req)
				if rec.Code != tc.want {
					b.Fatalf("status = %d, want %d", rec.Code, tc.want)
				}
			}
		})
	}
}

func benchEngine(b *testing.B, cfg *config.Config) http.Handler {
	b.Helper()
	store, err := sqlite.New(config.StoreConfig{
		Driver:           "sqlite",
		DSN:              filepath.Join(b.TempDir(), "polaris.db"),
		MaxConnections:   1,
		LogRetentionDays: 30,
		LogBufferSize:    100,
		LogFlushInterval: time.Second,
	})
	if err != nil {
		b.Fatalf("sqlite.New: %v", err)
	}
	if err := store.Migrate(b.Context()); err != nil {
		b.Fatalf("migrate: %v", err)
	}
	registry, _, err := provider.New(cfg)
	if err != nil {
		b.Fatalf("provider.New: %v", err)
	}
	engine, err := NewEngine(Dependencies{
		Config:   cfg,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    store,
		Cache:    cache.NewMemory(),
		Registry: registry,
	})
	if err != nil {
		b.Fatalf("NewEngine: %v", err)
	}
	return engine
}
