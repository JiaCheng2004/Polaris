package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	gwmetrics "github.com/JiaCheng2004/Polaris/internal/gateway/metrics"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/JiaCheng2004/Polaris/internal/store/sqlite"
	"github.com/gin-gonic/gin"
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
	ids := make([]string, 0, len(response.Data))
	for _, raw := range response.Data {
		var item struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			t.Fatalf("decode model item: %v", err)
		}
		ids = append(ids, item.ID)
	}
	for _, expected := range []string{"openai/gpt-4o", "default-chat", "gpt-4o"} {
		if !slices.Contains(ids, expected) {
			t.Fatalf("expected model id %q in response, got %v", expected, ids)
		}
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

func TestGatewayRejectsOversizedRequestBody(t *testing.T) {
	cfg := testConfig(t)
	cfg.Server.MaxBodyBytes = 16
	engine := newTestEngine(t, cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"default-chat","input":"this body is too large"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected oversized body 413, got %d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"request_body_too_large"`) {
		t.Fatalf("expected request_body_too_large error, got %s", res.Body.String())
	}
}

func TestCORSAllowsConfiguredLocalhostOrigins(t *testing.T) {
	cfg := testConfig(t)
	cfg.Server.CORS = config.DefaultCORSConfig()
	engine := newTestEngine(t, cfg)

	req := httptest.NewRequest(http.MethodOptions, "/v1/models", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "GET")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected preflight 204, got %d body=%s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected localhost CORS origin, got %q", got)
	}

	req = httptest.NewRequest(http.MethodOptions, "/v1/models", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	res = httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected disallowed preflight 403, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestModelsEndpointIncludesPhase3Metadata(t *testing.T) {
	cfg := testConfig(t)
	cfg.Providers["openai"] = config.ProviderConfig{
		APIKey:  "sk-openai",
		BaseURL: "https://api.openai.com/v1",
		Timeout: time.Minute,
		Models: map[string]config.ModelConfig{
			"gpt-4o": {
				Modality:     modality.ModalityChat,
				Capabilities: []modality.Capability{modality.CapabilityStreaming},
			},
			"text-embedding-3-small": {
				Modality:   modality.ModalityEmbed,
				Dimensions: 1536,
			},
			"gpt-image-1": {
				Modality:      modality.ModalityImage,
				Capabilities:  []modality.Capability{modality.CapabilityGeneration, modality.CapabilityEditing},
				OutputFormats: []string{"png"},
			},
			"tts-1": {
				Modality:     modality.ModalityVoice,
				Capabilities: []modality.Capability{modality.CapabilityTTS},
				Voices:       []string{"nova"},
			},
		},
	}

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

	res := httptest.NewRecorder()
	engine.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("expected /v1/models 200, got %d body=%s", res.Code, res.Body.String())
	}

	var response struct {
		Data []provider.Model `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode models response: %v", err)
	}

	models := map[string]provider.Model{}
	for _, model := range response.Data {
		models[model.ID] = model
	}
	if models["openai/text-embedding-3-small"].Dimensions != 1536 {
		t.Fatalf("expected embedding dimensions metadata, got %#v", models["openai/text-embedding-3-small"])
	}
	if len(models["openai/gpt-image-1"].OutputFormats) != 1 || models["openai/gpt-image-1"].OutputFormats[0] != "png" {
		t.Fatalf("expected image output formats metadata, got %#v", models["openai/gpt-image-1"])
	}
	if len(models["openai/tts-1"].Voices) != 1 || models["openai/tts-1"].Voices[0] != "nova" {
		t.Fatalf("expected voice metadata, got %#v", models["openai/tts-1"])
	}
}

func TestEmbeddingsEndpointOpenAIAndUsage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream payload: %v", err)
		}
		if payload.Model != "text-embedding-3-small" {
			t.Fatalf("expected provider model text-embedding-3-small, got %q", payload.Model)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object":"list",
			"data":[{"object":"embedding","index":0,"embedding":[0.1,0.2,0.3]}],
			"model":"text-embedding-3-small",
			"usage":{"prompt_tokens":8,"total_tokens":8}
		}`))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"openai/*"},
	}}
	cfg.Routing.Aliases = map[string]string{
		"default-embed": "openai/text-embedding-3-small",
	}
	cfg.Providers["openai"] = config.ProviderConfig{
		APIKey:  "sk-openai",
		BaseURL: upstream.URL + "/v1",
		Timeout: time.Second,
		Models: map[string]config.ModelConfig{
			"text-embedding-3-small": {
				Modality:   modality.ModalityEmbed,
				Dimensions: 1536,
			},
		},
	}

	sqliteStore := testSQLiteStore(t)
	registry, warnings, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no registry warnings, got %v", warnings)
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

	body := strings.NewReader(`{"model":"default-embed","input":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}

	var response struct {
		Model string `json:"model"`
		Usage struct {
			TotalTokens int    `json:"total_tokens"`
			Source      string `json:"source"`
		} `json:"usage"`
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode embed response: %v", err)
	}
	if response.Model != "openai/text-embedding-3-small" {
		t.Fatalf("expected canonical model, got %q", response.Model)
	}
	if response.Usage.TotalTokens != 8 {
		t.Fatalf("expected total tokens 8, got %d", response.Usage.TotalTokens)
	}
	if response.Usage.Source != "provider_reported" {
		t.Fatalf("expected provider_reported usage source, got %#v", response.Usage)
	}
	if len(response.Data) != 1 || len(response.Data[0].Embedding) != 3 {
		t.Fatalf("unexpected embedding payload %#v", response.Data)
	}

	if err := requestLogger.Close(context.Background()); err != nil {
		t.Fatalf("requestLogger.Close() error = %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/usage?group_by=model&modality=embed", nil)
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
	if usage.TotalRequests != 1 || usage.TotalTokens != 8 {
		t.Fatalf("unexpected usage response %#v", usage)
	}
	if len(usage.ByModel) != 1 || usage.ByModel[0].Model != "openai/text-embedding-3-small" {
		t.Fatalf("unexpected usage by_model %#v", usage.ByModel)
	}
}

func TestEmbeddingsEndpointGoogle(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-embedding-001:embedContent" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"embedding":{"values":[1.5,2.25]}
		}`))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"google/*"},
	}}
	cfg.Routing.Aliases = map[string]string{}
	cfg.Providers = map[string]config.ProviderConfig{
		"google": {
			APIKey:  "google-key",
			BaseURL: upstream.URL,
			Timeout: time.Second,
			Models: map[string]config.ModelConfig{
				"gemini-embedding-001": {
					Modality:   modality.ModalityEmbed,
					Dimensions: 768,
				},
			},
		},
	}

	engine := newTestEngine(t, cfg)
	body := strings.NewReader(`{"model":"google/gemini-embedding-001","input":"hello","encoding_format":"base64"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}

	var response struct {
		Model string `json:"model"`
		Data  []struct {
			Embedding string `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode embed response: %v", err)
	}
	if response.Model != "google/gemini-embedding-001" {
		t.Fatalf("expected canonical model, got %q", response.Model)
	}
	if len(response.Data) != 1 || response.Data[0].Embedding == "" {
		t.Fatalf("expected base64 embedding output, got %#v", response.Data)
	}
}

func TestEmbeddingsEndpointGoogleBatch(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-embedding-001:batchEmbedContents" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		var payload struct {
			Requests []struct {
				Model string `json:"model"`
			} `json:"requests"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream payload: %v", err)
		}
		if len(payload.Requests) != 2 {
			t.Fatalf("expected 2 batch requests, got %d", len(payload.Requests))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"embeddings":[
				{"values":[1.5,2.25]},
				{"values":[3.5,4.75]}
			]
		}`))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"google/*"},
	}}
	cfg.Routing.Aliases = map[string]string{}
	cfg.Providers = map[string]config.ProviderConfig{
		"google": {
			APIKey:  "google-key",
			BaseURL: upstream.URL,
			Timeout: time.Second,
			Models: map[string]config.ModelConfig{
				"gemini-embedding-001": {
					Modality:   modality.ModalityEmbed,
					Dimensions: 768,
				},
			},
		},
	}

	engine := newTestEngine(t, cfg)
	body := strings.NewReader(`{"model":"google/gemini-embedding-001","input":["hello","world"],"encoding_format":"base64"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}

	var response struct {
		Data []struct {
			Embedding string `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode embed response: %v", err)
	}
	if len(response.Data) != 2 || response.Data[0].Embedding == "" || response.Data[1].Embedding == "" {
		t.Fatalf("expected 2 base64 embedding outputs, got %#v", response.Data)
	}
}

func TestEmbeddingCacheHitPreservesUsageOutcome(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object":"list",
			"data":[{"object":"embedding","index":0,"embedding":[0.1,0.2,0.3]}],
			"model":"text-embedding-3-small",
			"usage":{"prompt_tokens":8,"total_tokens":8,"source":"provider_reported"}
		}`))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"openai/*"},
	}}
	cfg.Cache.ResponseCache.Enabled = true
	cfg.Cache.ResponseCache.TTL = time.Hour
	cfg.Routing.Aliases = map[string]string{
		"default-embed": "openai/text-embedding-3-small",
	}
	cfg.Providers["openai"] = config.ProviderConfig{
		APIKey:  "sk-openai",
		BaseURL: upstream.URL + "/v1",
		Timeout: time.Second,
		Models: map[string]config.ModelConfig{
			"text-embedding-3-small": {
				Modality:   modality.ModalityEmbed,
				Dimensions: 1536,
			},
		},
	}

	sqliteStore := testSQLiteStore(t)
	registry, warnings, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no registry warnings, got %v", warnings)
	}
	requestLogger := store.NewAsyncRequestLogger(sqliteStore, slog.New(slog.NewTextHandler(io.Discard, nil)), store.NewLoggerConfig(10, 5*time.Millisecond))
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	engine, err := NewEngine(Dependencies{
		Config:        cfg,
		Logger:        logger,
		Store:         sqliteStore,
		Cache:         cache.NewMemory(),
		Registry:      registry,
		RequestLogger: requestLogger,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	for i, want := range []string{"miss", "hit"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"default-embed","input":"cache me"}`))
		req.Header.Set("Authorization", "Bearer secret")
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d body=%s", i+1, res.Code, res.Body.String())
		}
		if got := res.Header().Get("X-Polaris-Cache"); got != want {
			t.Fatalf("request %d: expected X-Polaris-Cache=%s, got %q", i+1, want, got)
		}
	}
	if upstreamCalls != 1 {
		t.Fatalf("expected one upstream call, got %d", upstreamCalls)
	}

	if err := requestLogger.Close(context.Background()); err != nil {
		t.Fatalf("requestLogger.Close() error = %v", err)
	}

	usage, err := sqliteStore.GetUsage(context.Background(), store.UsageFilter{Modality: modality.ModalityEmbed})
	if err != nil {
		t.Fatalf("GetUsage() error = %v", err)
	}
	if usage.TotalRequests != 2 || usage.TotalTokens != 16 {
		t.Fatalf("expected cached embed usage to count both requests, got %#v", usage)
	}
	if !containsStructuredLogLine(logBuf.String(), `"cache_status":"hit"`, `"token_source":"provider_reported"`, `"model":"openai/text-embedding-3-small"`) {
		t.Fatalf("expected cache-hit structured log with provider_reported token source, got logs:\n%s", logBuf.String())
	}
}

func TestSemanticChatCacheHitPreservesUsageOutcome(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-cache",
			"object":"chat.completion",
			"created":1744329600,
			"model":"gpt-4o",
			"choices":[{"index":0,"message":{"role":"assistant","content":"Cached hello"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13,"source":"provider_reported"}
		}`))
	}))
	defer upstream.Close()

	cfg := testConfigWithOpenAIBaseURL(t, upstream.URL+"/v1")
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"openai/*"},
	}}
	cfg.Cache.ResponseCache.Enabled = true
	cfg.Cache.ResponseCache.TTL = time.Hour
	cfg.Cache.ResponseCache.SimilarityThreshold = 0.95
	cfg.Cache.ResponseCache.MaxEntriesPerModel = 10

	sqliteStore := testSQLiteStore(t)
	registry, warnings, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no registry warnings, got %v", warnings)
	}
	requestLogger := store.NewAsyncRequestLogger(sqliteStore, slog.New(slog.NewTextHandler(io.Discard, nil)), store.NewLoggerConfig(10, 5*time.Millisecond))
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	engine, err := NewEngine(Dependencies{
		Config:        cfg,
		Logger:        logger,
		Store:         sqliteStore,
		Cache:         cache.NewMemory(),
		Registry:      registry,
		RequestLogger: requestLogger,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	requests := []string{
		`{"model":"default-chat","messages":[{"role":"user","content":"Hello, Polaris!"}]}`,
		`{"model":"default-chat","messages":[{"role":"user","content":"hello polaris"}]}`,
	}
	for i, body := range requests {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer secret")
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d body=%s", i+1, res.Code, res.Body.String())
		}
		want := "miss"
		if i == 1 {
			want = "hit"
		}
		if got := res.Header().Get("X-Polaris-Cache"); got != want {
			t.Fatalf("request %d: expected X-Polaris-Cache=%s, got %q", i+1, want, got)
		}
	}
	if upstreamCalls != 1 {
		t.Fatalf("expected one upstream call, got %d", upstreamCalls)
	}

	if err := requestLogger.Close(context.Background()); err != nil {
		t.Fatalf("requestLogger.Close() error = %v", err)
	}

	usage, err := sqliteStore.GetUsage(context.Background(), store.UsageFilter{Modality: modality.ModalityChat})
	if err != nil {
		t.Fatalf("GetUsage() error = %v", err)
	}
	if usage.TotalRequests != 2 || usage.TotalTokens != 26 {
		t.Fatalf("expected cached chat usage to count both requests, got %#v", usage)
	}
	if !containsStructuredLogLine(logBuf.String(), `"cache_status":"hit"`, `"token_source":"provider_reported"`, `"model":"openai/gpt-4o"`) {
		t.Fatalf("expected cache-hit structured log with provider_reported token source, got logs:\n%s", logBuf.String())
	}
}

func TestImageGenerationEndpointValidatesCapabilityBeforeAdapter(t *testing.T) {
	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"openai/*"},
	}}
	cfg.Routing.Aliases = map[string]string{}
	cfg.Providers["openai"] = config.ProviderConfig{
		APIKey:  "sk-openai",
		BaseURL: "https://api.openai.com/v1",
		Timeout: time.Minute,
		Models: map[string]config.ModelConfig{
			"gpt-image-1": {
				Modality:     modality.ModalityImage,
				Capabilities: []modality.Capability{modality.CapabilityGeneration, modality.CapabilityEditing},
			},
		},
	}

	engine := newTestEngine(t, cfg)
	body := strings.NewReader(`{"model":"openai/gpt-image-1","prompt":"a lighthouse","reference_images":["https://example.com/ref.png"]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"type":"capability_not_supported"`) {
		t.Fatalf("expected capability_not_supported error, got %s", res.Body.String())
	}
}

func TestImageGenerationEndpointOpenAI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"created":1744329600,
			"data":[{"url":"https://example.com/generated.png","revised_prompt":"A lighthouse at dusk"}]
		}`))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"openai/*"},
	}}
	cfg.Routing.Aliases = map[string]string{
		"default-image": "openai/gpt-image-1",
	}
	cfg.Providers["openai"] = config.ProviderConfig{
		APIKey:  "sk-openai",
		BaseURL: upstream.URL + "/v1",
		Timeout: time.Second,
		Models: map[string]config.ModelConfig{
			"gpt-image-1": {
				Modality:     modality.ModalityImage,
				Capabilities: []modality.Capability{modality.CapabilityGeneration, modality.CapabilityEditing},
			},
		},
	}

	engine := newTestEngine(t, cfg)
	body := strings.NewReader(`{"model":"default-image","prompt":"a lighthouse"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}

	var response struct {
		Created int64 `json:"created"`
		Data    []struct {
			URL           string `json:"url"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode image response: %v", err)
	}
	if response.Created != 1744329600 || len(response.Data) != 1 || response.Data[0].URL != "https://example.com/generated.png" {
		t.Fatalf("unexpected image response %#v", response)
	}
}

func TestImageGenerationEndpointOpenAIInlineImageOutput(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream payload: %v", err)
		}
		if _, ok := payload["response_format"]; ok {
			t.Fatalf("expected gpt-image request to omit response_format, got %#v", payload["response_format"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"created":1744329600,
			"data":[{"b64_json":"AQID","revised_prompt":"A lighthouse at dusk"}]
		}`))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"openai/*"},
	}}
	cfg.Routing.Aliases = map[string]string{
		"default-image": "openai/gpt-image-1",
	}
	cfg.Providers["openai"] = config.ProviderConfig{
		APIKey:  "sk-openai",
		BaseURL: upstream.URL + "/v1",
		Timeout: time.Second,
		Models: map[string]config.ModelConfig{
			"gpt-image-1": {
				Modality:     modality.ModalityImage,
				Capabilities: []modality.Capability{modality.CapabilityGeneration, modality.CapabilityEditing},
			},
		},
	}

	engine := newTestEngine(t, cfg)
	body := strings.NewReader(`{"model":"default-image","prompt":"a lighthouse"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}

	var response struct {
		Created int64 `json:"created"`
		Data    []struct {
			URL           string `json:"url"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode image response: %v", err)
	}
	if response.Created != 1744329600 || len(response.Data) != 1 || response.Data[0].URL != "data:image/png;base64,AQID" {
		t.Fatalf("unexpected image response %#v", response)
	}
}

func TestImageEditEndpointGoogle(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-3-pro-image-preview:generateContent" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidates":[{
				"index":0,
				"content":{"parts":[
					{"inlineData":{"mimeType":"image/png","data":"AQID"}}
				]}
			}]
		}`))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"google/*"},
	}}
	cfg.Routing.Aliases = map[string]string{}
	cfg.Providers = map[string]config.ProviderConfig{
		"google": {
			APIKey:  "google-key",
			BaseURL: upstream.URL,
			Timeout: time.Second,
			Models: map[string]config.ModelConfig{
				"nano-banana-pro": {
					Modality:     modality.ModalityImage,
					Capabilities: []modality.Capability{modality.CapabilityGeneration, modality.CapabilityEditing, modality.CapabilityMultiReference},
				},
			},
		},
	}

	engine := newTestEngine(t, cfg)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", "google/nano-banana-pro"); err != nil {
		t.Fatalf("WriteField(model) error = %v", err)
	}
	if err := writer.WriteField("prompt", "Make it brighter"); err != nil {
		t.Fatalf("WriteField(prompt) error = %v", err)
	}
	imageHeader := textproto.MIMEHeader{}
	imageHeader.Set("Content-Disposition", `form-data; name="image"; filename="input.png"`)
	imageHeader.Set("Content-Type", "image/png")
	fileWriter, err := writer.CreatePart(imageHeader)
	if err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}
	if _, err := fileWriter.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00, 0x00, 0x00, 0x0d}); err != nil {
		t.Fatalf("fileWriter.Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}

	var response struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode image response: %v", err)
	}
	if len(response.Data) != 1 || !strings.HasPrefix(response.Data[0].URL, "data:image/png;base64,") {
		t.Fatalf("unexpected image response %#v", response)
	}
}

func TestImageGenerationEndpointByteDanceAlias(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/images/generations" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "doubao-seedream-4.5" {
			t.Fatalf("unexpected model %#v", payload["model"])
		}
		images, ok := payload["image"].([]any)
		if !ok || len(images) != 2 {
			t.Fatalf("expected 2 reference images, got %#v", payload["image"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"created":1744329600,
			"data":[{"b64_json":"AQID"}]
		}`))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"bytedance/*"},
	}}
	cfg.Providers = map[string]config.ProviderConfig{
		"bytedance": {
			APIKey:  "ark-key",
			BaseURL: upstream.URL + "/api/v3",
			Timeout: time.Second,
			Models: map[string]config.ModelConfig{
				"seedream-4.5": {
					Modality:      modality.ModalityImage,
					Capabilities:  []modality.Capability{modality.CapabilityGeneration, modality.CapabilityEditing, modality.CapabilityMultiReference},
					OutputFormats: []string{"png", "jpeg"},
					Endpoint:      upstream.URL + "/api/v3",
				},
			},
		},
	}
	cfg.Routing.Aliases = map[string]string{
		"default-image": "bytedance/seedream-4.5",
	}

	engine := newTestEngine(t, cfg)
	body := strings.NewReader(`{"model":"default-image","prompt":"a poster","response_format":"b64_json","reference_images":["https://example.com/ref.png","ZmFrZS1iNjQ="]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}

	var response struct {
		Created int64 `json:"created"`
		Data    []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode image response: %v", err)
	}
	if response.Created != 1744329600 || len(response.Data) != 1 || response.Data[0].B64JSON != "AQID" {
		t.Fatalf("unexpected image response %#v", response)
	}
}

func TestImageEditEndpointQwen(t *testing.T) {
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/services/aigc/multimodal-generation/generation":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			messages := payload["input"].(map[string]any)["messages"].([]any)
			content := messages[0].(map[string]any)["content"].([]any)
			if len(content) != 2 {
				t.Fatalf("expected image + prompt content, got %#v", content)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"output":{
					"choices":[{
						"message":{
							"content":[
								{"image":"` + upstream.URL + `/result.png"}
							]
						}
					}]
				}
			}`))
		case "/result.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"qwen/*"},
	}}
	cfg.Routing.Aliases = map[string]string{}
	cfg.Providers = map[string]config.ProviderConfig{
		"qwen": {
			APIKey:  "qwen-key",
			BaseURL: upstream.URL + "/compatible-mode/v1",
			Timeout: time.Second,
			Models: map[string]config.ModelConfig{
				"qwen-image-2.0": {
					Modality:      modality.ModalityImage,
					Capabilities:  []modality.Capability{modality.CapabilityGeneration, modality.CapabilityEditing},
					OutputFormats: []string{"png"},
				},
			},
		},
	}

	engine := newTestEngine(t, cfg)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", "qwen/qwen-image-2.0"); err != nil {
		t.Fatalf("WriteField(model) error = %v", err)
	}
	if err := writer.WriteField("prompt", "Replace the background"); err != nil {
		t.Fatalf("WriteField(prompt) error = %v", err)
	}
	if err := writer.WriteField("response_format", "b64_json"); err != nil {
		t.Fatalf("WriteField(response_format) error = %v", err)
	}
	imageHeader := textproto.MIMEHeader{}
	imageHeader.Set("Content-Disposition", `form-data; name="image"; filename="input.png"`)
	imageHeader.Set("Content-Type", "image/png")
	fileWriter, err := writer.CreatePart(imageHeader)
	if err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}
	if _, err := fileWriter.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}); err != nil {
		t.Fatalf("fileWriter.Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}

	var response struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode image response: %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].B64JSON == "" {
		t.Fatalf("unexpected image response %#v", response)
	}
}

func TestAudioSpeechEndpointRejectsUnknownVoiceBeforeAdapter(t *testing.T) {
	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"openai/*"},
	}}
	cfg.Routing.Aliases = map[string]string{}
	cfg.Providers["openai"] = config.ProviderConfig{
		APIKey:  "sk-openai",
		BaseURL: "https://api.openai.com/v1",
		Timeout: time.Minute,
		Models: map[string]config.ModelConfig{
			"tts-1": {
				Modality:     modality.ModalityVoice,
				Capabilities: []modality.Capability{modality.CapabilityTTS},
				Voices:       []string{"nova"},
			},
		},
	}

	engine := newTestEngine(t, cfg)
	body := strings.NewReader(`{"model":"openai/tts-1","input":"Hello","voice":"echo"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"code":"unknown_voice"`) {
		t.Fatalf("expected unknown_voice error, got %s", res.Body.String())
	}
}

func TestAudioTranscriptionsEndpointRejectsUnsupportedFormatBeforeAdapter(t *testing.T) {
	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"openai/*"},
	}}
	cfg.Routing.Aliases = map[string]string{}
	cfg.Providers["openai"] = config.ProviderConfig{
		APIKey:  "sk-openai",
		BaseURL: "https://api.openai.com/v1",
		Timeout: time.Minute,
		Models: map[string]config.ModelConfig{
			"whisper-1": {
				Modality:     modality.ModalityVoice,
				Capabilities: []modality.Capability{modality.CapabilitySTT},
				Formats:      []string{"wav"},
			},
		},
	}

	engine := newTestEngine(t, cfg)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", "openai/whisper-1"); err != nil {
		t.Fatalf("WriteField(model) error = %v", err)
	}
	fileWriter, err := writer.CreateFormFile("file", "sample.mp3")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := fileWriter.Write([]byte("not-real-audio")); err != nil {
		t.Fatalf("fileWriter.Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", &body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"code":"unsupported_audio_format"`) {
		t.Fatalf("expected unsupported_audio_format error, got %s", res.Body.String())
	}
}

func TestAudioSpeechEndpointOpenAI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		var payload struct {
			Model          string `json:"model"`
			Voice          string `json:"voice"`
			ResponseFormat string `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream payload: %v", err)
		}
		if payload.Model != "tts-1" {
			t.Fatalf("expected provider model tts-1, got %q", payload.Model)
		}
		if payload.Voice != "nova" || payload.ResponseFormat != "wav" {
			t.Fatalf("unexpected payload %#v", payload)
		}

		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFFpolaris"))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"openai/*"},
	}}
	cfg.Routing.Aliases = map[string]string{
		"default-voice-tts": "openai/tts-1",
	}
	cfg.Providers["openai"] = config.ProviderConfig{
		APIKey:  "sk-openai",
		BaseURL: upstream.URL + "/v1",
		Timeout: time.Second,
		Models: map[string]config.ModelConfig{
			"tts-1": {
				Modality:     modality.ModalityVoice,
				Capabilities: []modality.Capability{modality.CapabilityTTS},
				Voices:       []string{"nova"},
			},
		},
	}

	engine := newTestEngine(t, cfg)
	body := strings.NewReader(`{"model":"default-voice-tts","input":"Hello","voice":"nova","response_format":"wav"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Content-Type"); got != "audio/wav" {
		t.Fatalf("unexpected Content-Type %q", got)
	}
	if got := res.Body.String(); got != "RIFFpolaris" {
		t.Fatalf("unexpected body %q", got)
	}
}

func TestAudioSpeechEndpointByteDance(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/tts/unidirectional/sse" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Api-Key"); got != "speech-key" {
			t.Fatalf("unexpected X-Api-Key header %q", got)
		}
		if got := r.Header.Get("X-Api-Resource-Id"); got != "seed-tts-2.0" {
			t.Fatalf("unexpected X-Api-Resource-Id header %q", got)
		}

		var payload struct {
			User struct {
				UID string `json:"uid"`
			} `json:"user"`
			ReqParams struct {
				Text    string `json:"text"`
				Speaker string `json:"speaker"`
				Audio   struct {
					Format     string `json:"format"`
					SampleRate int    `json:"sample_rate"`
				} `json:"audio_params"`
			} `json:"req_params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream payload: %v", err)
		}
		if strings.TrimSpace(payload.User.UID) == "" {
			t.Fatalf("expected non-empty uid")
		}
		if payload.ReqParams.Text != "Hello" || payload.ReqParams.Speaker != "zh_female_vv_uranus_bigtts" {
			t.Fatalf("unexpected req_params %#v", payload.ReqParams)
		}
		if payload.ReqParams.Audio.Format != "ogg_opus" || payload.ReqParams.Audio.SampleRate != 24000 {
			t.Fatalf("unexpected audio params %#v", payload.ReqParams.Audio)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: 352\n"))
		_, _ = w.Write([]byte("data: {\"code\":0,\"message\":\"\",\"data\":\"AQID\"}\n\n"))
		_, _ = w.Write([]byte("event: 152\n"))
		_, _ = w.Write([]byte("data: {\"code\":20000000,\"message\":\"OK\"}\n\n"))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"bytedance/*"},
	}}
	cfg.Routing.Aliases = map[string]string{
		"default-voice-tts": "bytedance/doubao-tts-2.0",
	}
	cfg.Providers = map[string]config.ProviderConfig{
		"bytedance": {
			SpeechAPIKey: "speech-key",
			Timeout:      time.Second,
			Models: map[string]config.ModelConfig{
				"doubao-tts-2.0": {
					Modality:     modality.ModalityVoice,
					Capabilities: []modality.Capability{modality.CapabilityTTS},
					Voices:       []string{"zh_female_vv_uranus_bigtts"},
					Endpoint:     upstream.URL + "/api/v3/tts/unidirectional/sse",
				},
			},
		},
	}

	engine := newTestEngine(t, cfg)
	body := strings.NewReader(`{"model":"default-voice-tts","input":"Hello","voice":"zh_female_vv_uranus_bigtts","response_format":"opus"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Content-Type"); got != "audio/ogg" {
		t.Fatalf("unexpected Content-Type %q", got)
	}
	if got := res.Body.Bytes(); len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("unexpected body %#v", got)
	}
}

func TestAudioTranscriptionsEndpointOpenAI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}
		if got := r.FormValue("model"); got != "whisper-1" {
			t.Fatalf("unexpected model %q", got)
		}
		if got := r.FormValue("response_format"); got != "verbose_json" {
			t.Fatalf("unexpected response_format %q", got)
		}
		if got := r.Form["timestamp_granularities[]"]; len(got) != 1 || got[0] != "segment" {
			t.Fatalf("unexpected timestamp_granularities %#v", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"text":"Hello, this is a transcription.",
			"language":"en",
			"duration":3.42,
			"segments":[{"id":0,"start":0.0,"end":3.42,"text":"Hello, this is a transcription."}]
		}`))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"openai/*"},
	}}
	cfg.Routing.Aliases = map[string]string{}
	cfg.Providers["openai"] = config.ProviderConfig{
		APIKey:  "sk-openai",
		BaseURL: upstream.URL + "/v1",
		Timeout: time.Second,
		Models: map[string]config.ModelConfig{
			"whisper-1": {
				Modality:     modality.ModalityVoice,
				Capabilities: []modality.Capability{modality.CapabilitySTT},
				Formats:      []string{"wav"},
			},
		},
	}

	engine := newTestEngine(t, cfg)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", "openai/whisper-1"); err != nil {
		t.Fatalf("WriteField(model) error = %v", err)
	}
	if err := writer.WriteField("response_format", "json"); err != nil {
		t.Fatalf("WriteField(response_format) error = %v", err)
	}
	fileWriter, err := writer.CreateFormFile("file", "sample.wav")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := fileWriter.Write([]byte("wav-bytes")); err != nil {
		t.Fatalf("fileWriter.Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", &body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}

	var response struct {
		Text     string `json:"text"`
		Language string `json:"language"`
		Segments []struct {
			Text string `json:"text"`
		} `json:"segments"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Text != "Hello, this is a transcription." || response.Language != "en" {
		t.Fatalf("unexpected response %#v", response)
	}
	if len(response.Segments) != 1 || response.Segments[0].Text != "Hello, this is a transcription." {
		t.Fatalf("unexpected segments %#v", response.Segments)
	}
}

func TestAudioTranscriptionsEndpointByteDance(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/auc/bigmodel/recognize/flash" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Api-Key"); got != "speech-key" {
			t.Fatalf("unexpected X-Api-Key header %q", got)
		}
		if got := r.Header.Get("X-Api-Resource-Id"); got != "volc.bigasr.auc_turbo" {
			t.Fatalf("unexpected X-Api-Resource-Id header %q", got)
		}
		if got := r.Header.Get("X-Api-Sequence"); got != "-1" {
			t.Fatalf("unexpected X-Api-Sequence header %q", got)
		}

		var payload struct {
			Request struct {
				ModelName string `json:"model_name"`
			} `json:"request"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream payload: %v", err)
		}
		if payload.Request.ModelName != "bigmodel" {
			t.Fatalf("unexpected model_name %q", payload.Request.ModelName)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Api-Status-Code", "20000000")
		_, _ = w.Write([]byte(`{
			"audio_info":{"duration":3000},
			"result":{
				"text":"Hello ByteDance",
				"utterances":[
					{"start_time":0,"end_time":1500,"text":"Hello"},
					{"start_time":1500,"end_time":3000,"text":"ByteDance"}
				]
			}
		}`))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"bytedance/*"},
	}}
	cfg.Routing.Aliases = map[string]string{
		"default-voice-stt": "bytedance/doubao-asr-2.0",
	}
	cfg.Providers = map[string]config.ProviderConfig{
		"bytedance": {
			SpeechAPIKey: "speech-key",
			Timeout:      time.Second,
			Models: map[string]config.ModelConfig{
				"doubao-asr-2.0": {
					Modality:     modality.ModalityVoice,
					Capabilities: []modality.Capability{modality.CapabilitySTT},
					Formats:      []string{"wav", "mp3"},
					Endpoint:     upstream.URL + "/api/v3/auc/bigmodel/recognize/flash",
				},
			},
		},
	}

	engine := newTestEngine(t, cfg)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", "default-voice-stt"); err != nil {
		t.Fatalf("WriteField(model) error = %v", err)
	}
	if err := writer.WriteField("response_format", "vtt"); err != nil {
		t.Fatalf("WriteField(response_format) error = %v", err)
	}
	fileWriter, err := writer.CreateFormFile("file", "sample.wav")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := fileWriter.Write([]byte("wav-bytes")); err != nil {
		t.Fatalf("fileWriter.Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", &body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Content-Type"); got != "text/vtt; charset=utf-8" {
		t.Fatalf("unexpected Content-Type %q", got)
	}
	expectedBody := "WEBVTT\n\n00:00:00.000 --> 00:00:01.500\nHello\n\n00:00:01.500 --> 00:00:03.000\nByteDance\n\n"
	if got := res.Body.String(); got != expectedBody {
		t.Fatalf("unexpected body %q", got)
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

func TestChatCompletionFallbackAndUsageRecording(t *testing.T) {
	primaryCalls := 0
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls++
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error","code":"provider_rate_limit"}}`))
	}))
	defer primary.Close()

	fallbackCalls := 0
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalls++
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode fallback payload: %v", err)
		}
		if payload.Model != "deepseek-chat" {
			t.Fatalf("expected provider model deepseek-chat, got %q", payload.Model)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"object":"chat.completion",
			"created":1744329600,
			"model":"deepseek-chat",
			"choices":[{"index":0,"message":{"role":"assistant","content":"Fallback hello"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}
		}`))
	}))
	defer fallback.Close()

	cfg := testConfigWithProviderBaseURLs(t, map[string]string{
		"openai":   primary.URL + "/v1",
		"deepseek": fallback.URL + "/v1",
	})
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{
		{
			Name:          "test-key",
			KeyHash:       middleware.HashAPIKey("secret"),
			RateLimit:     "10/min",
			AllowedModels: []string{"openai/*", "deepseek/*"},
		},
	}
	cfg.Routing.Fallbacks = []config.FallbackRule{
		{
			From: "openai/gpt-4o",
			To:   []string{"deepseek/deepseek-chat"},
			On:   []string{"rate_limit", "timeout", "server_error"},
		},
	}

	sqliteStore := testSQLiteStore(t)
	registry, warnings, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no registry warnings, got %v", warnings)
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
	if got := res.Header().Get("X-Polaris-Fallback"); got != "deepseek/deepseek-chat" {
		t.Fatalf("expected fallback header for deepseek, got %q", got)
	}
	if primaryCalls != 1 || fallbackCalls != 1 {
		t.Fatalf("expected 1 primary and 1 fallback call, got primary=%d fallback=%d", primaryCalls, fallbackCalls)
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
	if response.Model != "deepseek/deepseek-chat" {
		t.Fatalf("expected fallback model, got %q", response.Model)
	}
	if response.Usage.TotalTokens != 18 {
		t.Fatalf("expected total tokens 18, got %d", response.Usage.TotalTokens)
	}

	if err := requestLogger.Close(context.Background()); err != nil {
		t.Fatalf("requestLogger.Close() error = %v", err)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/v1/usage?group_by=model", nil)
	usageReq.Header.Set("Authorization", "Bearer secret")
	usageRes := httptest.NewRecorder()
	engine.ServeHTTP(usageRes, usageReq)
	if usageRes.Code != http.StatusOK {
		t.Fatalf("expected usage endpoint 200, got %d body=%s", usageRes.Code, usageRes.Body.String())
	}

	var usage struct {
		ByModel []struct {
			Model string `json:"model"`
		} `json:"by_model"`
	}
	if err := json.Unmarshal(usageRes.Body.Bytes(), &usage); err != nil {
		t.Fatalf("decode usage response: %v", err)
	}
	if len(usage.ByModel) != 1 || usage.ByModel[0].Model != "deepseek/deepseek-chat" {
		t.Fatalf("expected usage to record fallback model, got %#v", usage.ByModel)
	}
}

func TestChatFallbackStopsAtFirstSuccess(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error","code":"provider_rate_limit"}}`))
	}))
	defer primary.Close()

	deepseekCalls := 0
	deepseek := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deepseekCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"object":"chat.completion",
			"created":1744329600,
			"model":"deepseek-chat",
			"choices":[{"index":0,"message":{"role":"assistant","content":"DeepSeek won"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":8,"completion_tokens":4,"total_tokens":12}
		}`))
	}))
	defer deepseek.Close()

	xaiCalls := 0
	xai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xaiCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-x","object":"chat.completion","created":1744329600,"model":"grok-3","choices":[{"index":0,"message":{"role":"assistant","content":"xAI should not be reached"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer xai.Close()

	cfg := testConfigWithProviderBaseURLs(t, map[string]string{
		"openai":   primary.URL + "/v1",
		"deepseek": deepseek.URL + "/v1",
		"xai":      xai.URL + "/v1",
	})
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{
		{
			Name:          "test-key",
			KeyHash:       middleware.HashAPIKey("secret"),
			RateLimit:     "10/min",
			AllowedModels: []string{"openai/*", "deepseek/*", "xai/*"},
		},
	}
	cfg.Routing.Fallbacks = []config.FallbackRule{
		{
			From: "openai/gpt-4o",
			To:   []string{"deepseek/deepseek-chat", "xai/grok-3"},
			On:   []string{"rate_limit", "timeout", "server_error"},
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

	body := strings.NewReader(`{"model":"openai/gpt-4o","messages":[{"role":"user","content":"Hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected chat completion 200, got %d body=%s", res.Code, res.Body.String())
	}
	if deepseekCalls != 1 || xaiCalls != 0 {
		t.Fatalf("expected first fallback only, got deepseek=%d xai=%d", deepseekCalls, xaiCalls)
	}
}

func TestChatFallbackExhaustionReturnsFinalRetryableError(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"primary limited","type":"rate_limit_error","code":"provider_rate_limit"}}`))
	}))
	defer primary.Close()

	deepseek := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"deepseek unavailable","type":"server_error","code":"provider_server_error"}}`))
	}))
	defer deepseek.Close()

	xai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"xai limited","type":"rate_limit_error","code":"provider_rate_limit"}}`))
	}))
	defer xai.Close()

	cfg := testConfigWithProviderBaseURLs(t, map[string]string{
		"openai":   primary.URL + "/v1",
		"deepseek": deepseek.URL + "/v1",
		"xai":      xai.URL + "/v1",
	})
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{
		{
			Name:          "test-key",
			KeyHash:       middleware.HashAPIKey("secret"),
			RateLimit:     "10/min",
			AllowedModels: []string{"openai/*", "deepseek/*", "xai/*"},
		},
	}
	cfg.Routing.Fallbacks = []config.FallbackRule{
		{
			From: "openai/gpt-4o",
			To:   []string{"deepseek/deepseek-chat", "xai/grok-3"},
			On:   []string{"rate_limit", "timeout", "server_error"},
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

	body := strings.NewReader(`{"model":"openai/gpt-4o","messages":[{"role":"user","content":"Hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("expected final retryable status 429, got %d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "xai limited") {
		t.Fatalf("expected final fallback error body, got %s", res.Body.String())
	}
}

func TestChatDoesNotFallbackOnNonRetryableError(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request","type":"invalid_request_error","code":"provider_bad_request"}}`))
	}))
	defer primary.Close()

	fallbackCalls := 0
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalls++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1744329600,"model":"deepseek-chat","choices":[{"index":0,"message":{"role":"assistant","content":"should not happen"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer fallback.Close()

	cfg := testConfigWithProviderBaseURLs(t, map[string]string{
		"openai":   primary.URL + "/v1",
		"deepseek": fallback.URL + "/v1",
	})
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{
		{
			Name:          "test-key",
			KeyHash:       middleware.HashAPIKey("secret"),
			RateLimit:     "10/min",
			AllowedModels: []string{"openai/*", "deepseek/*"},
		},
	}
	cfg.Routing.Fallbacks = []config.FallbackRule{
		{
			From: "openai/gpt-4o",
			To:   []string{"deepseek/deepseek-chat"},
			On:   []string{"rate_limit", "timeout", "server_error"},
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

	body := strings.NewReader(`{"model":"openai/gpt-4o","messages":[{"role":"user","content":"Hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request without fallback, got %d body=%s", res.Code, res.Body.String())
	}
	if fallbackCalls != 0 {
		t.Fatalf("expected no fallback attempts, got %d", fallbackCalls)
	}
}

func TestChatStreamingFallbackBeforeStreamStarts(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error","code":"provider_rate_limit"}}`))
	}))
	defer primary.Close()

	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1744329600,\"model\":\"deepseek-chat\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1744329600,\"model\":\"deepseek-chat\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Fallback hello\"},\"finish_reason\":null}],\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":4,\"total_tokens\":13}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer fallback.Close()

	cfg := testConfigWithProviderBaseURLs(t, map[string]string{
		"openai":   primary.URL + "/v1",
		"deepseek": fallback.URL + "/v1",
	})
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{
		{
			Name:          "test-key",
			KeyHash:       middleware.HashAPIKey("secret"),
			RateLimit:     "10/min",
			AllowedModels: []string{"openai/*", "deepseek/*"},
		},
	}
	cfg.Routing.Fallbacks = []config.FallbackRule{
		{
			From: "openai/gpt-4o",
			To:   []string{"deepseek/deepseek-chat"},
			On:   []string{"rate_limit", "timeout", "server_error"},
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
	if got := res.Header().Get("X-Polaris-Fallback"); got != "deepseek/deepseek-chat" {
		t.Fatalf("expected fallback header for deepseek, got %q", got)
	}
	if !strings.Contains(res.Body.String(), `"model":"deepseek/deepseek-chat"`) {
		t.Fatalf("expected fallback model in SSE body, got %s", res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "Fallback hello") {
		t.Fatalf("expected fallback SSE content, got %s", res.Body.String())
	}
}

func TestChatAliasPermissionChecksResolvedModel(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1744329600,"model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()

	cfg := testConfigWithProviderBaseURLs(t, map[string]string{
		"openai": upstream.URL + "/v1",
	})
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{
		{
			Name:          "test-key",
			KeyHash:       middleware.HashAPIKey("secret"),
			RateLimit:     "10/min",
			AllowedModels: []string{"default-chat"},
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

	body := strings.NewReader(`{"model":"default-chat","messages":[{"role":"user","content":"Hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected alias request to be checked against canonical model, got %d body=%s", res.Code, res.Body.String())
	}
	if upstreamCalls != 0 {
		t.Fatalf("expected no upstream call on permission failure, got %d", upstreamCalls)
	}
}

func TestRuntimeSwapUpdatesAliasWithoutServerRestart(t *testing.T) {
	openAICalls := 0
	openAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		openAICalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1744329600,"model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"OpenAI"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer openAI.Close()

	deepseekCalls := 0
	deepseek := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deepseekCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-2","object":"chat.completion","created":1744329600,"model":"deepseek-chat","choices":[{"index":0,"message":{"role":"assistant","content":"DeepSeek"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer deepseek.Close()

	cfg := testConfigWithProviderBaseURLs(t, map[string]string{
		"openai":   openAI.URL + "/v1",
		"deepseek": deepseek.URL + "/v1",
	})
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{
		{
			Name:          "test-key",
			KeyHash:       middleware.HashAPIKey("secret"),
			RateLimit:     "10/min",
			AllowedModels: []string{"openai/*", "deepseek/*"},
		},
	}

	registry, _, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	runtimeHolder := gwruntime.NewHolder(cfg, registry)

	engine, err := NewEngine(Dependencies{
		Config:   cfg,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    testSQLiteStore(t),
		Cache:    cache.NewMemory(),
		Registry: registry,
		Runtime:  runtimeHolder,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	request := func() string {
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
		}
		if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return response.Model
	}

	if model := request(); model != "openai/gpt-4o" {
		t.Fatalf("expected initial alias to resolve to openai, got %q", model)
	}

	reloadedCfg := testConfigWithProviderBaseURLs(t, map[string]string{
		"openai":   openAI.URL + "/v1",
		"deepseek": deepseek.URL + "/v1",
	})
	reloadedCfg.Auth = cfg.Auth
	reloadedCfg.Routing.Aliases = map[string]string{
		"default-chat": "deepseek/deepseek-chat",
	}
	reloadedRegistry, _, err := provider.New(reloadedCfg)
	if err != nil {
		t.Fatalf("provider.New(reloaded) error = %v", err)
	}
	runtimeHolder.Swap(reloadedCfg, reloadedRegistry)

	if model := request(); model != "deepseek/deepseek-chat" {
		t.Fatalf("expected reloaded alias to resolve to deepseek, got %q", model)
	}
	if openAICalls != 1 || deepseekCalls != 1 {
		t.Fatalf("expected one request per provider after runtime swap, got openai=%d deepseek=%d", openAICalls, deepseekCalls)
	}
}

func TestRuntimeSwapUpdatesRateLimitWithoutServerRestart(t *testing.T) {
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

	registry, _, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	runtimeHolder := gwruntime.NewHolder(cfg, registry)

	engine, err := NewEngine(Dependencies{
		Config:   cfg,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    testSQLiteStore(t),
		Cache:    cache.NewMemory(),
		Registry: registry,
		Runtime:  runtimeHolder,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	request := func() int {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer secret")
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, req)
		return res.Code
	}

	if status := request(); status != http.StatusOK {
		t.Fatalf("expected first request 200, got %d", status)
	}
	if status := request(); status != http.StatusTooManyRequests {
		t.Fatalf("expected second request 429, got %d", status)
	}

	reloadedCfg := testConfig(t)
	reloadedCfg.Auth.Mode = config.AuthModeStatic
	reloadedCfg.Auth.StaticKeys = []config.StaticKeyConfig{
		{
			Name:          "test-key",
			KeyHash:       middleware.HashAPIKey("secret"),
			RateLimit:     "3/min",
			AllowedModels: []string{"*"},
		},
	}
	reloadedRegistry, _, err := provider.New(reloadedCfg)
	if err != nil {
		t.Fatalf("provider.New(reloaded) error = %v", err)
	}
	runtimeHolder.Swap(reloadedCfg, reloadedRegistry)

	if status := request(); status != http.StatusOK {
		t.Fatalf("expected third request to use reloaded limit and succeed, got %d", status)
	}
}

func TestMetricsRouteHonorsEnabledAndCustomPath(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		cfg := testConfig(t)
		cfg.Observability.Metrics.Enabled = false

		registry, _, err := provider.New(cfg)
		if err != nil {
			t.Fatalf("provider.New() error = %v", err)
		}
		engine, err := NewEngine(Dependencies{
			Config:   cfg,
			Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			Store:    testSQLiteStore(t),
			Cache:    cache.NewMemory(),
			Registry: registry,
			Metrics:  gwmetrics.NewRecorder(),
		})
		if err != nil {
			t.Fatalf("NewEngine() error = %v", err)
		}

		res := httptest.NewRecorder()
		engine.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		if res.Code != http.StatusNotFound {
			t.Fatalf("expected /metrics 404 when disabled, got %d", res.Code)
		}
	})

	t.Run("custom path", func(t *testing.T) {
		cfg := testConfig(t)
		cfg.Observability.Metrics.Enabled = true
		cfg.Observability.Metrics.Path = "/prometheus"

		registry, _, err := provider.New(cfg)
		if err != nil {
			t.Fatalf("provider.New() error = %v", err)
		}
		engine, err := NewEngine(Dependencies{
			Config:   cfg,
			Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			Store:    testSQLiteStore(t),
			Cache:    cache.NewMemory(),
			Registry: registry,
			Metrics:  gwmetrics.NewRecorder(),
		})
		if err != nil {
			t.Fatalf("NewEngine() error = %v", err)
		}

		notFound := httptest.NewRecorder()
		engine.ServeHTTP(notFound, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		if notFound.Code != http.StatusNotFound {
			t.Fatalf("expected default /metrics path to be unregistered, got %d", notFound.Code)
		}

		res := httptest.NewRecorder()
		engine.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/prometheus", nil))
		if res.Code != http.StatusOK {
			t.Fatalf("expected custom metrics path 200, got %d", res.Code)
		}
		if !strings.Contains(res.Body.String(), "polaris_requests_total") {
			t.Fatalf("expected metrics payload, got %s", res.Body.String())
		}
	})
}

func TestMetricsCaptureFailoverAndRateLimit(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error","code":"provider_rate_limit"}}`))
	}))
	defer primary.Close()

	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1744329600,"model":"deepseek-chat","choices":[{"index":0,"message":{"role":"assistant","content":"Fallback"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	}))
	defer fallback.Close()

	cfg := testConfigWithProviderBaseURLs(t, map[string]string{
		"openai":   primary.URL + "/v1",
		"deepseek": fallback.URL + "/v1",
	})
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{
		{
			Name:          "test-key",
			KeyHash:       middleware.HashAPIKey("secret"),
			RateLimit:     "2/min",
			AllowedModels: []string{"openai/*", "deepseek/*"},
		},
	}
	cfg.Routing.Fallbacks = []config.FallbackRule{
		{
			From: "openai/gpt-4o",
			To:   []string{"deepseek/deepseek-chat"},
			On:   []string{"rate_limit", "timeout", "server_error"},
		},
	}

	registry, _, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	recorder := gwmetrics.NewRecorder()
	engine, err := NewEngine(Dependencies{
		Config:   cfg,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    testSQLiteStore(t),
		Cache:    cache.NewMemory(),
		Registry: registry,
		Metrics:  recorder,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	chatBody := strings.NewReader(`{"model":"openai/gpt-4o","messages":[{"role":"user","content":"Hello"}]}`)
	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", chatBody)
	chatReq.Header.Set("Authorization", "Bearer secret")
	chatReq.Header.Set("Content-Type", "application/json")
	chatRes := httptest.NewRecorder()
	engine.ServeHTTP(chatRes, chatReq)
	if chatRes.Code != http.StatusOK {
		t.Fatalf("expected chat completion 200, got %d body=%s", chatRes.Code, chatRes.Body.String())
	}

	modelsReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	modelsReq.Header.Set("Authorization", "Bearer secret")
	modelsRes := httptest.NewRecorder()
	engine.ServeHTTP(modelsRes, modelsReq)
	if modelsRes.Code != http.StatusOK {
		t.Fatalf("expected first models request 200, got %d body=%s", modelsRes.Code, modelsRes.Body.String())
	}

	limitedReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	limitedReq.Header.Set("Authorization", "Bearer secret")
	limitedRes := httptest.NewRecorder()
	engine.ServeHTTP(limitedRes, limitedReq)
	if limitedRes.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second models request to hit rate limit, got %d body=%s", limitedRes.Code, limitedRes.Body.String())
	}

	metricsRes := httptest.NewRecorder()
	engine.ServeHTTP(metricsRes, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metricsRes.Code != http.StatusOK {
		t.Fatalf("expected metrics 200, got %d body=%s", metricsRes.Code, metricsRes.Body.String())
	}
	body := metricsRes.Body.String()

	if !strings.Contains(body, `polaris_requests_total{interface_family="chat_completions",modality="chat",model="deepseek/deepseek-chat",provider="deepseek",status="200"} 1`) {
		t.Fatalf("expected request counter for fallback-served chat, got %s", body)
	}
	if !strings.Contains(body, `polaris_tokens_total{direction="output",model="deepseek/deepseek-chat",provider="deepseek",token_source="provider_reported"} 5`) {
		t.Fatalf("expected token counter with provenance for fallback-served chat, got %s", body)
	}
	if !strings.Contains(body, `polaris_provider_errors_total{error_type="rate_limit_error",provider="openai"} 1`) {
		t.Fatalf("expected provider error counter for primary failure, got %s", body)
	}
	if !strings.Contains(body, `polaris_failovers_total{from_model="openai/gpt-4o",to_model="deepseek/deepseek-chat"} 1`) {
		t.Fatalf("expected failover counter, got %s", body)
	}
	if !strings.Contains(body, `polaris_rate_limit_hits_total{key_id="test-key"} 1`) {
		t.Fatalf("expected rate limit counter, got %s", body)
	}
}

func TestMetricsActiveStreamsGaugeTracksInFlightStreams(t *testing.T) {
	streamStarted := make(chan struct{})
	releaseStream := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			close(releaseStream)
		})
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1744329600,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		close(streamStarted)
		<-releaseStream
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1744329600,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	cfg := testConfigWithProviderBaseURLs(t, map[string]string{
		"openai": upstream.URL + "/v1",
	})
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{
		{
			Name:          "test-key",
			KeyHash:       middleware.HashAPIKey("secret"),
			RateLimit:     "10/min",
			AllowedModels: []string{"openai/*"},
		},
	}

	registry, _, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	recorder := gwmetrics.NewRecorder()
	engine, err := NewEngine(Dependencies{
		Config:   cfg,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    testSQLiteStore(t),
		Cache:    cache.NewMemory(),
		Registry: registry,
		Metrics:  recorder,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	server := httptest.NewServer(engine)
	defer server.Close()
	defer release()

	client := server.Client()
	streamDone := make(chan string, 1)
	go func() {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{"model":"openai/gpt-4o","stream":true,"messages":[{"role":"user","content":"Hello"}]}`))
		if err != nil {
			streamDone <- err.Error()
			return
		}
		req.Header.Set("Authorization", "Bearer secret")
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			streamDone <- err.Error()
			return
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		payload, err := io.ReadAll(resp.Body)
		if err != nil {
			streamDone <- err.Error()
			return
		}
		streamDone <- string(payload)
	}()

	select {
	case <-streamStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream to start")
	}

	readMetrics := func() string {
		t.Helper()

		metricsResp, err := client.Get(server.URL + "/metrics")
		if err != nil {
			t.Fatalf("GET /metrics error = %v", err)
		}
		bodyBytes, err := io.ReadAll(metricsResp.Body)
		if closeErr := metricsResp.Body.Close(); closeErr != nil {
			t.Fatalf("close metrics body: %v", closeErr)
		}
		if err != nil {
			t.Fatalf("read metrics body: %v", err)
		}
		return string(bodyBytes)
	}
	waitForMetric := func(expected string) string {
		t.Helper()

		deadline := time.Now().Add(2 * time.Second)
		var body string
		for {
			body = readMetrics()
			if strings.Contains(body, expected) {
				return body
			}
			if time.Now().After(deadline) {
				t.Fatalf("expected metrics to contain %q, got %s", expected, body)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	waitForMetric(`polaris_active_streams{model="openai/gpt-4o",provider="openai"} 1`)

	release()

	select {
	case payload := <-streamDone:
		if !strings.Contains(payload, "data: [DONE]") {
			t.Fatalf("expected completed stream payload, got %s", payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream completion")
	}

	waitForMetric(`polaris_active_streams{model="openai/gpt-4o",provider="openai"} 0`)
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
			Metrics: config.MetricsConfig{
				Enabled: true,
				Path:    "/metrics",
			},
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

func newTestEngine(t *testing.T, cfg *config.Config) *gin.Engine {
	t.Helper()

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
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    sqliteStore,
		Cache:    cache.NewMemory(),
		Registry: registry,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	return engine
}

func testConfigWithProviderBaseURLs(t *testing.T, baseURLs map[string]string) *config.Config {
	t.Helper()
	cfg := testConfig(t)
	cfg.Providers = map[string]config.ProviderConfig{}

	if baseURL := baseURLs["openai"]; baseURL != "" {
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
	}

	if baseURL := baseURLs["deepseek"]; baseURL != "" {
		cfg.Providers["deepseek"] = config.ProviderConfig{
			APIKey:  "sk-deepseek",
			BaseURL: baseURL,
			Timeout: time.Second,
			Models: map[string]config.ModelConfig{
				"deepseek-chat": {
					Modality: modality.ModalityChat,
					Capabilities: []modality.Capability{
						modality.CapabilityStreaming,
						modality.CapabilityFunctionCalling,
					},
				},
			},
		}
	}

	if baseURL := baseURLs["xai"]; baseURL != "" {
		cfg.Providers["xai"] = config.ProviderConfig{
			APIKey:  "sk-xai",
			BaseURL: baseURL,
			Timeout: time.Second,
			Models: map[string]config.ModelConfig{
				"grok-3": {
					Modality: modality.ModalityChat,
					Capabilities: []modality.Capability{
						modality.CapabilityStreaming,
						modality.CapabilityFunctionCalling,
					},
				},
			},
		}
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

func containsStructuredLogLine(logs string, needles ...string) bool {
	for _, line := range strings.Split(logs, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		matched := true
		for _, needle := range needles {
			if !strings.Contains(line, needle) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}
