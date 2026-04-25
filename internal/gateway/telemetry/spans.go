package telemetry

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

func StartInternalSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return otel.Tracer("polaris/runtime").Start(ctx, name, trace.WithSpanKind(trace.SpanKindInternal), trace.WithAttributes(attrs...))
}

func StartClientSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return otel.Tracer("polaris/runtime").Start(ctx, name, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attrs...))
}

func AnnotateCurrentSpan(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return
	}
	span.SetAttributes(attrs...)
}

func RecordSpanError(span trace.Span, err error) {
	if err == nil || !span.SpanContext().IsValid() {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

func InjectHTTPHeaders(ctx context.Context, header http.Header) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(header))
}

type ProviderTransport struct {
	provider string
	base     http.RoundTripper
}

func NewProviderTransport(provider string, base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &ProviderTransport{provider: provider, base: base}
}

func (t *ProviderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, span := StartClientSpan(req.Context(), "provider.http",
		attribute.String("polaris.provider", t.provider),
		semconv.HTTPRequestMethodKey.String(req.Method),
		attribute.String("http.target", req.URL.Path),
		attribute.String("server.address", req.URL.Host),
	)
	defer span.End()

	cloned := req.Clone(ctx)
	InjectHTTPHeaders(ctx, cloned.Header)

	resp, err := t.base.RoundTrip(cloned)
	if err != nil {
		RecordSpanError(span, err)
		return nil, err
	}
	span.SetAttributes(semconv.HTTPResponseStatusCode(resp.StatusCode))
	if resp.StatusCode >= http.StatusBadRequest {
		span.SetStatus(codes.Error, http.StatusText(resp.StatusCode))
	}
	return resp, nil
}
