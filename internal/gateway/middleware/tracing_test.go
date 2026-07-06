package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/config"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func genaiTestEngine(t *testing.T, enabled bool) (*gin.Engine, *tracetest.SpanRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	cfg := config.Default()
	cfg.Observability.Traces.GenAI = enabled
	holder := gwruntime.NewHolder(&cfg, nil)
	r := gin.New()
	r.Use(Tracing(), Runtime(holder))
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		SetRequestOutcome(c, RequestOutcome{
			Model: "openai/gpt-4o", Provider: "openai", Modality: modality.ModalityChat,
			PromptTokens: 5, CompletionTokens: 3, FinishReasons: []string{"stop"},
		})
		c.Status(http.StatusOK)
	})
	return r, rec
}

func spanAttr(attrs []attribute.KeyValue, key string) (attribute.Value, bool) {
	for _, a := range attrs {
		if string(a.Key) == key {
			return a.Value, true
		}
	}
	return attribute.Value{}, false
}

func TestGenAISemconvEmittedWhenEnabled(t *testing.T) {
	r, rec := genaiTestEngine(t, true)
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))

	spans := rec.Ended()
	if len(spans) == 0 {
		t.Fatal("no spans recorded")
	}
	attrs := spans[len(spans)-1].Attributes()
	if v, ok := spanAttr(attrs, "gen_ai.system"); !ok || v.AsString() != "openai" {
		t.Fatalf("gen_ai.system = %v (ok=%v)", v.AsString(), ok)
	}
	if v, ok := spanAttr(attrs, "gen_ai.request.model"); !ok || v.AsString() != "openai/gpt-4o" {
		t.Fatalf("gen_ai.request.model = %v (ok=%v)", v.AsString(), ok)
	}
	if v, ok := spanAttr(attrs, "gen_ai.usage.input_tokens"); !ok || v.AsInt64() != 5 {
		t.Fatalf("gen_ai.usage.input_tokens = %d (ok=%v)", v.AsInt64(), ok)
	}
	if v, ok := spanAttr(attrs, "gen_ai.operation.name"); !ok || v.AsString() != "chat" {
		t.Fatalf("gen_ai.operation.name = %v (ok=%v)", v.AsString(), ok)
	}
	if _, ok := spanAttr(attrs, "gen_ai.response.finish_reasons"); !ok {
		t.Fatal("gen_ai.response.finish_reasons missing")
	}
}

func TestGenAISemconvOmittedWhenDisabled(t *testing.T) {
	r, rec := genaiTestEngine(t, false)
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))

	spans := rec.Ended()
	if len(spans) == 0 {
		t.Fatal("no spans recorded")
	}
	attrs := spans[len(spans)-1].Attributes()
	if _, ok := spanAttr(attrs, "gen_ai.system"); ok {
		t.Fatal("gen_ai.* attributes emitted while disabled (privacy default)")
	}
	// The existing polaris.* attributes are still present.
	if _, ok := spanAttr(attrs, "polaris.model"); !ok {
		t.Fatal("polaris.model missing")
	}
}
