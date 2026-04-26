package gateway

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTracingRecordsProviderAndCacheSpans(t *testing.T) {
	recorder, shutdown := installSpanRecorder(t)
	defer shutdown()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1744329600,"model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
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

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"openai/gpt-4o","messages":[{"role":"user","content":"Hello"}]}`))
		req.Header.Set("Authorization", "Bearer secret")
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("expected request %d to succeed, got %d body=%s", i+1, res.Code, res.Body.String())
		}
	}

	spans := recorder.Ended()
	if !hasSpanWithAttributes(spans, "provider.http", map[string]string{
		"polaris.provider": "openai",
	}) {
		t.Fatalf("expected provider.http span with openai provider, got %#v", spanSummaries(spans))
	}
	if !hasSpanWithAttributes(spans, "cache.lookup", map[string]string{
		"polaris.cache_status": "hit",
		"polaris.cache_kind":   "semantic",
	}) {
		t.Fatalf("expected cache.lookup hit span for semantic cache, got %#v", spanSummaries(spans))
	}
	if !hasSpanWithAttributes(spans, "policy.resolve_chat_target", map[string]string{
		"polaris.model": "openai/gpt-4o",
	}) {
		t.Fatalf("expected policy.resolve_chat_target span for resolved model, got %#v", spanSummaries(spans))
	}
}

func TestTracingRecordsMCPLocalToolsetSpans(t *testing.T) {
	recorder, shutdown := installSpanRecorder(t)
	defer shutdown()

	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeNone
	cfg.MCP.Enabled = true
	cfg.Tools.Enabled = true

	sqliteStore := testSQLiteStore(t)
	now := time.Now().UTC()
	if err := sqliteStore.CreateToolDefinition(context.Background(), store.ToolDefinition{
		ID:             "tool-math-add",
		Name:           "math.add",
		Description:    "Add two numbers",
		Implementation: "math.add",
		InputSchema:    `{"type":"object","properties":{"a":{"type":"number"},"b":{"type":"number"}},"required":["a","b"]}`,
		Enabled:        true,
		CreatedAt:      now,
	}); err != nil {
		t.Fatalf("CreateToolDefinition() error = %v", err)
	}
	if err := sqliteStore.CreateToolset(context.Background(), store.Toolset{
		ID:          "toolset-local",
		Name:        "local",
		Description: "Local tools",
		ToolIDs:     []string{"tool-math-add"},
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("CreateToolset() error = %v", err)
	}
	if err := sqliteStore.CreateMCPBinding(context.Background(), store.MCPBinding{
		ID:        "binding-local",
		Name:      "local binding",
		Kind:      store.MCPBindingKindLocalToolset,
		ToolsetID: "toolset-local",
		Enabled:   true,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateMCPBinding() error = %v", err)
	}

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

	req := httptest.NewRequest(http.MethodPost, "/mcp/binding-local", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"math.add","arguments":{"a":2,"b":3}}}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected MCP tool call 200, got %d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"text":"5"`) {
		t.Fatalf("expected MCP tool response to contain result text, got %s", res.Body.String())
	}

	spans := recorder.Ended()
	if !hasSpanWithAttributes(spans, "mcp.broker", map[string]string{
		"polaris.mcp_binding_id":   "binding-local",
		"polaris.mcp_binding_kind": string(store.MCPBindingKindLocalToolset),
	}) {
		t.Fatalf("expected mcp.broker span, got %#v", spanSummaries(spans))
	}
	if !hasSpanWithAttributes(spans, "mcp.local_toolset", map[string]string{
		"polaris.toolset_id": "toolset-local",
	}) {
		t.Fatalf("expected mcp.local_toolset span, got %#v", spanSummaries(spans))
	}
	if !hasSpanWithAttributes(spans, "tool.execute", map[string]string{
		"polaris.tool_name": "math.add",
	}) {
		t.Fatalf("expected tool.execute span, got %#v", spanSummaries(spans))
	}
}

func installSpanRecorder(t *testing.T) (*tracetest.SpanRecorder, func()) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
	)
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)

	return recorder, func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previous)
	}
}

func hasSpanWithAttributes(spans []sdktrace.ReadOnlySpan, name string, want map[string]string) bool {
	for _, span := range spans {
		if span.Name() != name {
			continue
		}
		if spanHasAttributes(span, want) {
			return true
		}
	}
	return false
}

func spanHasAttributes(span sdktrace.ReadOnlySpan, want map[string]string) bool {
	attrs := map[string]string{}
	for _, attr := range span.Attributes() {
		attrs[string(attr.Key)] = fmt.Sprint(attr.Value.AsInterface())
	}
	for key, value := range want {
		if attrs[key] != value {
			return false
		}
	}
	return true
}

func spanSummaries(spans []sdktrace.ReadOnlySpan) []string {
	out := make([]string, 0, len(spans))
	for _, span := range spans {
		out = append(out, span.Name())
	}
	return out
}
