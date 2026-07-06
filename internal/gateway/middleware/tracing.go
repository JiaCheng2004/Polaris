package middleware

import (
	"net/http"

	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

func Tracing() gin.HandlerFunc {
	tracer := otel.Tracer("polaris/http")
	return func(c *gin.Context) {
		propagator := otel.GetTextMapPropagator()
		ctx := propagator.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		spanName := c.Request.Method + " " + c.FullPath()
		if c.FullPath() == "" {
			spanName = c.Request.Method + " " + c.Request.URL.Path
		}
		ctx, span := tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()

		c.Request = c.Request.WithContext(ctx)
		traceID := span.SpanContext().TraceID().String()
		if traceID != "" {
			SetTraceID(c, traceID)
			c.Writer.Header().Set("X-Trace-ID", traceID)
		}
		span.SetAttributes(
			semconv.HTTPRequestMethodKey.String(c.Request.Method),
			attribute.String("http.route", c.FullPath()),
			attribute.String("http.target", c.Request.URL.Path),
		)
		c.Next()
		status := c.Writer.Status()
		auth := GetAuthContext(c)
		outcome, _ := GetRequestOutcome(c)
		cacheStatus := firstNonEmpty(outcome.CacheStatus, c.Writer.Header().Get("X-Polaris-Cache"))
		interfaceFamily := firstNonEmpty(outcome.InterfaceFamily, interfaceFamilyFromPath(c.FullPath()))
		span.SetAttributes(semconv.HTTPResponseStatusCode(status))
		if auth.ProjectID != "" {
			span.SetAttributes(attribute.String("polaris.project_id", auth.ProjectID))
		}
		if interfaceFamily != "" {
			span.SetAttributes(attribute.String("polaris.interface_family", interfaceFamily))
		}
		if outcome.Model != "" {
			span.SetAttributes(attribute.String("polaris.model", outcome.Model))
		}
		if outcome.Provider != "" {
			span.SetAttributes(attribute.String("polaris.provider", outcome.Provider))
		}
		if outcome.Modality != "" {
			span.SetAttributes(attribute.String("polaris.modality", string(outcome.Modality)))
		}
		if source := usageTokenSource(outcome); source != "" {
			span.SetAttributes(attribute.String("polaris.token_source", source))
		}
		if cacheStatus != "" {
			span.SetAttributes(attribute.String("polaris.cache_status", cacheStatus))
		}
		if outcome.FallbackModel != "" {
			span.SetAttributes(attribute.String("polaris.fallback_to", outcome.FallbackModel))
		}
		if outcome.Toolset != "" {
			span.SetAttributes(attribute.String("polaris.toolset_id", outcome.Toolset))
		}
		if outcome.MCPBinding != "" {
			span.SetAttributes(attribute.String("polaris.mcp_binding_id", outcome.MCPBinding))
		}
		if outcome.Model != "" && genaiTracingEnabled(c) {
			// OpenTelemetry GenAI semantic conventions (opt-in). No prompt or
			// completion content is captured — metadata only.
			span.SetAttributes(
				attribute.String("gen_ai.operation.name", genaiOperation(outcome.Modality)),
				attribute.String("gen_ai.request.model", outcome.Model),
				attribute.String("gen_ai.response.model", outcome.Model),
			)
			if outcome.Provider != "" {
				span.SetAttributes(attribute.String("gen_ai.system", outcome.Provider))
			}
			if outcome.PromptTokens > 0 {
				span.SetAttributes(attribute.Int("gen_ai.usage.input_tokens", outcome.PromptTokens))
			}
			if outcome.CompletionTokens > 0 {
				span.SetAttributes(attribute.Int("gen_ai.usage.output_tokens", outcome.CompletionTokens))
			}
			if len(outcome.FinishReasons) > 0 {
				span.SetAttributes(attribute.StringSlice("gen_ai.response.finish_reasons", outcome.FinishReasons))
			}
		}
		if status >= http.StatusBadRequest {
			span.SetStatus(codes.Error, http.StatusText(status))
		}
	}
}

func genaiTracingEnabled(c *gin.Context) bool {
	snapshot, ok := GetRuntimeSnapshot(c)
	return ok && snapshot != nil && snapshot.Config != nil && snapshot.Config.Observability.Traces.GenAI
}

func genaiOperation(m modality.Modality) string {
	if m == "" || m == modality.ModalityChat {
		return "chat"
	}
	return string(m)
}
