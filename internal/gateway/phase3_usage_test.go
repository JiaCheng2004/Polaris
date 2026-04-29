package gateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
)

func TestPhase3UsageAggregatesMixedModalities(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/embeddings":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"object":"list",
				"data":[{"object":"embedding","index":0,"embedding":[0.1,0.2]}],
				"model":"text-embedding-3-small",
				"usage":{"prompt_tokens":8,"total_tokens":8}
			}`))
		case "/v1/images/generations":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"created":1744329600,
				"data":[{"url":"https://example.com/generated.png"}]
			}`))
		case "/v1/audio/speech":
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("ID3"))
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
		AllowedModels: []string{"openai/*"},
	}}
	cfg.Routing.Aliases = map[string]string{
		"default-embed": "openai/text-embedding-3-small",
		"default-image": "openai/gpt-image-1",
		"default-voice": "openai/tts-1",
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
			"gpt-image-1": {
				Modality:     modality.ModalityImage,
				Capabilities: []modality.Capability{modality.CapabilityGeneration},
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

	requests := []struct {
		path string
		body string
	}{
		{
			path: "/v1/embeddings",
			body: `{"model":"default-embed","input":"hello"}`,
		},
		{
			path: "/v1/images/generations",
			body: `{"model":"default-image","prompt":"a lighthouse"}`,
		},
		{
			path: "/v1/audio/speech",
			body: `{"model":"default-voice","input":"Hello","voice":"nova"}`,
		},
	}

	for _, request := range requests {
		req := httptest.NewRequest(http.MethodPost, request.path, strings.NewReader(request.body))
		req.Header.Set("Authorization", "Bearer secret")
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("expected %s to return 200, got %d body=%s", request.path, res.Code, res.Body.String())
		}
	}

	if err := requestLogger.Close(context.Background()); err != nil {
		t.Fatalf("requestLogger.Close() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/usage?group_by=model", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected usage endpoint 200, got %d body=%s", res.Code, res.Body.String())
	}

	var usage struct {
		TotalRequests       int64            `json:"total_requests"`
		TotalTokens         int64            `json:"total_tokens"`
		CostSourceBreakdown map[string]int64 `json:"cost_source_breakdown"`
		ByModel             []struct {
			Model   string  `json:"model"`
			CostUSD float64 `json:"cost_usd"`
		} `json:"by_model"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &usage); err != nil {
		t.Fatalf("decode usage response: %v", err)
	}
	if usage.TotalRequests != 3 {
		t.Fatalf("expected 3 requests, got %d", usage.TotalRequests)
	}
	if usage.TotalTokens != 8 {
		t.Fatalf("expected 8 tokens from embeddings only, got %d", usage.TotalTokens)
	}

	byModel := map[string]float64{}
	for _, item := range usage.ByModel {
		byModel[item.Model] = item.CostUSD
	}
	if len(byModel) != 3 {
		t.Fatalf("expected 3 model groups, got %#v", usage.ByModel)
	}
	if byModel["openai/text-embedding-3-small"] <= 0 {
		t.Fatalf("expected positive embedding cost, got %#v", usage.ByModel)
	}
	if byModel["openai/gpt-image-1"] <= 0 {
		t.Fatalf("expected positive image cost, got %#v", usage.ByModel)
	}
	if byModel["openai/tts-1"] <= 0 {
		t.Fatalf("expected positive voice cost, got %#v", usage.ByModel)
	}
	if usage.CostSourceBreakdown["table"] != 3 {
		t.Fatalf("expected table cost source for all requests, got %#v", usage.CostSourceBreakdown)
	}
}
