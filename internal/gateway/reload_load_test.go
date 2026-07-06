package gateway

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/reliability"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
)

// TestReloadUnderLoad proves R10 + the §4.2 invariant: 1K concurrent mixed
// unary/streaming requests survive 50 hot-reload snapshot swaps + manager
// reconfigures with zero failed requests, no data race (run with -race), and no
// goroutine leak.
func TestReloadUnderLoad(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if bytes.Contains(b, []byte(`"stream":true`)) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"id\":\"c\",\"object\":\"chat.completion.chunk\",\"created\":1744329600,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"id\":\"c\",\"object\":\"chat.completion.chunk\",\"created\":1744329600,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c","object":"chat.completion","created":1744329600,"model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer mock.Close()

	cfg := testConfigWithProviderBaseURLs(t, map[string]string{"openai": mock.URL + "/v1"})
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{
		{Name: "k", KeyHash: middleware.HashAPIKey("secret"), RateLimit: "1000000/min", AllowedModels: []string{"openai/*"}},
	}

	registry, _, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New: %v", err)
	}
	holder := gwruntime.NewHolder(cfg, registry)
	mgr := reliability.NewManager(reliability.Config{}, nil)
	engine, err := NewEngine(Dependencies{
		Config:      cfg,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:       testSQLiteStore(t),
		Cache:       cache.NewMemory(),
		Registry:    registry,
		Runtime:     holder,
		Reliability: mgr,
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	before := runtime.NumGoroutine()

	// Continuously swap the snapshot + reconfigure the manager during the load.
	stop := make(chan struct{})
	var swapWG sync.WaitGroup
	swapWG.Add(1)
	go func() {
		defer swapWG.Done()
		for i := 0; i < 50; i++ {
			select {
			case <-stop:
				return
			default:
			}
			r2, _, e := provider.New(cfg)
			if e != nil {
				continue
			}
			holder.Swap(cfg, r2)
			mgr.Reconfigure(gwruntime.ReliabilityConfig(cfg))
			time.Sleep(time.Millisecond)
		}
	}()

	const total = 1000
	var failures atomic.Int32
	var wg sync.WaitGroup
	sem := make(chan struct{}, 64)
	for i := 0; i < total; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			body := `{"model":"openai/gpt-4o","messages":[{"role":"user","content":"hi"}]}`
			if i%3 == 0 {
				body = `{"model":"openai/gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer secret")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				failures.Add(1)
			}
		}(i)
	}
	wg.Wait()
	close(stop)
	swapWG.Wait()

	if f := failures.Load(); f != 0 {
		t.Fatalf("%d/%d requests failed during reload-under-load", f, total)
	}

	// Goroutine-leak check (goleak substitute): allow async loggers to settle.
	time.Sleep(300 * time.Millisecond)
	runtime.GC()
	after := runtime.NumGoroutine()
	if after > before+25 {
		t.Fatalf("possible goroutine leak: before=%d after=%d", before, after)
	}
}
