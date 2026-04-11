package gateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/JiaCheng2004/Polaris/internal/store/sqlite"
)

func TestHealthReadyAndModelsEndpoints(t *testing.T) {
	cfg := testConfig(t)
	sqliteStore := testSQLiteStore(t)
	registry, warnings, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no registry warnings, got %v", warnings)
	}

	engine, err := NewEngine(Dependencies{
		Config:   cfg,
		Store:    sqliteStore,
		Cache:    cache.NewMemory(),
		Registry: registry,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	health := httptest.NewRecorder()
	engine.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("expected /health 200, got %d", health.Code)
	}

	ready := httptest.NewRecorder()
	engine.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("expected /ready 200, got %d body=%s", ready.Code, ready.Body.String())
	}

	models := httptest.NewRecorder()
	engine.ServeHTTP(models, httptest.NewRequest(http.MethodGet, "/v1/models?include_aliases=true", nil))
	if models.Code != http.StatusOK {
		t.Fatalf("expected /v1/models 200, got %d body=%s", models.Code, models.Body.String())
	}

	var response struct {
		Object string            `json:"object"`
		Data   []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(models.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode models response: %v", err)
	}
	if response.Object != "list" {
		t.Fatalf("expected object=list, got %q", response.Object)
	}
	if len(response.Data) != 2 {
		t.Fatalf("expected 2 model records including alias, got %d", len(response.Data))
	}
}

func TestStaticAuthAndRateLimit(t *testing.T) {
	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{
		{
			Name:          "test-key",
			KeyHash:       middleware.HashAPIKey("secret"),
			RateLimit:     "1/min",
			AllowedModels: []string{"*"},
		},
	}

	sqliteStore := testSQLiteStore(t)
	registry, _, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}

	engine, err := NewEngine(Dependencies{
		Config:   cfg,
		Store:    sqliteStore,
		Cache:    cache.NewMemory(),
		Registry: registry,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized without bearer token, got %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res = httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected first authorized request 200, got %d body=%s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res = httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second authorized request 429, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestChatCompletionAndUsageEndpoints(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream payload: %v", err)
		}
		if payload.Model != "gpt-4o" {
			t.Fatalf("expected provider model gpt-4o, got %q", payload.Model)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"object":"chat.completion",
			"created":1744329600,
			"model":"gpt-4o",
			"choices":[{"index":0,"message":{"role":"assistant","content":"Hello from Polaris"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
		}`))
	}))
	defer upstream.Close()

	cfg := testConfigWithOpenAIBaseURL(t, upstream.URL+"/v1")
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{
		{
			Name:          "test-key",
			KeyHash:       middleware.HashAPIKey("secret"),
			RateLimit:     "10/min",
			AllowedModels: []string{"*"},
		},
	}

	sqliteStore := testSQLiteStore(t)
	registry, _, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
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

	body := strings.NewReader(`{"model":"default-chat","messages":[{"role":"user","content":"Hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected chat completion 200, got %d body=%s", res.Code, res.Body.String())
	}

	var response struct {
		Model string `json:"model"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode chat response: %v", err)
	}
	if response.Model != "openai/gpt-4o" {
		t.Fatalf("expected canonical model, got %q", response.Model)
	}
	if response.Usage.TotalTokens != 15 {
		t.Fatalf("expected total tokens 15, got %d", response.Usage.TotalTokens)
	}

	if err := requestLogger.Close(context.Background()); err != nil {
		t.Fatalf("requestLogger.Close() error = %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/usage?group_by=model", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res = httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected usage endpoint 200, got %d body=%s", res.Code, res.Body.String())
	}

	var usage struct {
		TotalRequests int64 `json:"total_requests"`
		TotalTokens   int64 `json:"total_tokens"`
		ByModel       []struct {
			Model string `json:"model"`
		} `json:"by_model"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &usage); err != nil {
		t.Fatalf("decode usage response: %v", err)
	}
	if usage.TotalRequests != 1 || usage.TotalTokens != 15 {
		t.Fatalf("unexpected usage response %#v", usage)
	}
	if len(usage.ByModel) != 1 || usage.ByModel[0].Model != "openai/gpt-4o" {
		t.Fatalf("unexpected usage by_model %#v", usage.ByModel)
	}
}

func TestChatStreamingMidstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1744329600,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"error\":{\"message\":\"upstream exploded\"}}\n\n"))
	}))
	defer upstream.Close()

	cfg := testConfigWithOpenAIBaseURL(t, upstream.URL+"/v1")
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{
		{
			Name:          "test-key",
			KeyHash:       middleware.HashAPIKey("secret"),
			RateLimit:     "10/min",
			AllowedModels: []string{"*"},
		},
	}

	sqliteStore := testSQLiteStore(t)
	registry, _, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}

	engine, err := NewEngine(Dependencies{
		Config:   cfg,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    sqliteStore,
		Cache:    cache.NewMemory(),
		Registry: registry,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	body := strings.NewReader(`{"model":"openai/gpt-4o","stream":true,"messages":[{"role":"user","content":"Hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected streaming response status 200, got %d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `data: {"id":"chatcmpl-1"`) {
		t.Fatalf("expected first SSE chunk, got %s", res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"type":"provider_error"`) {
		t.Fatalf("expected provider_error SSE frame, got %s", res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "data: [DONE]") {
		t.Fatalf("expected DONE sentinel, got %s", res.Body.String())
	}
}

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		Server: config.ServerConfig{
			Host:            "127.0.0.1",
			Port:            8080,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			ShutdownTimeout: 5 * time.Second,
		},
		Auth: config.AuthConfig{
			Mode: config.AuthModeNone,
		},
		Cache: config.CacheConfig{
			Driver: "memory",
			RateLimit: config.RateLimitConfig{
				Enabled: true,
				Default: "10/min",
				Window:  "sliding",
			},
		},
		Providers: map[string]config.ProviderConfig{
			"openai": {
				APIKey:  "sk-openai",
				BaseURL: "https://api.openai.com/v1",
				Timeout: time.Minute,
				Models: map[string]config.ModelConfig{
					"gpt-4o": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityJSONMode},
					},
				},
			},
		},
		Routing: config.RoutingConfig{
			Aliases: map[string]string{
				"default-chat": "openai/gpt-4o",
			},
		},
		Observability: config.ObservabilityConfig{
			Logging: config.LoggingConfig{
				Level:  "error",
				Format: "json",
			},
		},
	}
}

func testConfigWithOpenAIBaseURL(t *testing.T, baseURL string) *config.Config {
	t.Helper()
	cfg := testConfig(t)
	cfg.Providers["openai"] = config.ProviderConfig{
		APIKey:  "sk-openai",
		BaseURL: baseURL,
		Timeout: time.Second,
		Models: map[string]config.ModelConfig{
			"gpt-4o": {
				Modality: modality.ModalityChat,
				Capabilities: []modality.Capability{
					modality.CapabilityStreaming,
					modality.CapabilityFunctionCalling,
					modality.CapabilityVision,
					modality.CapabilityJSONMode,
				},
				MaxOutputTokens: 16384,
			},
		},
	}
	cfg.Routing.Aliases = map[string]string{
		"default-chat": "openai/gpt-4o",
	}
	return cfg
}

func testSQLiteStore(t *testing.T) *sqlite.Store {
	t.Helper()
	sqliteStore, err := sqlite.New(config.StoreConfig{
		Driver:           "sqlite",
		DSN:              filepath.Join(t.TempDir(), "polaris.db"),
		MaxConnections:   1,
		LogRetentionDays: 30,
		LogBufferSize:    10,
		LogFlushInterval: time.Second,
	})
	if err != nil {
		t.Fatalf("sqlite.New() error = %v", err)
	}
	if err := sqliteStore.Migrate(t.Context()); err != nil {
		t.Fatalf("sqliteStore.Migrate() error = %v", err)
	}
	return sqliteStore
}
