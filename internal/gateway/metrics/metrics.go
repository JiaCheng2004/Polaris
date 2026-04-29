package metrics

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Recorder struct {
	registry        *prometheus.Registry
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	providerLatency *prometheus.HistogramVec
	tokensTotal     *prometheus.CounterVec
	estimatedCost   *prometheus.CounterVec
	pricingLookups  *prometheus.CounterVec
	rateLimitHits   *prometheus.CounterVec
	budgetDenials   *prometheus.CounterVec
	providerErrors  *prometheus.CounterVec
	failovers       *prometheus.CounterVec
	cacheEvents     *prometheus.CounterVec
	toolInvocations *prometheus.CounterVec
	mcpSessions     *prometheus.CounterVec
	activeStreams   *prometheus.GaugeVec
}

func NewRecorder() *Recorder {
	registry := prometheus.NewRegistry()

	recorder := &Recorder{
		registry: registry,
		requestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "polaris_requests_total",
			Help: "Total HTTP requests served by Polaris.",
		}, []string{"interface_family", "model", "modality", "status", "provider"}),
		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "polaris_request_duration_seconds",
			Help:    "End-to-end request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"interface_family", "model", "modality", "provider"}),
		providerLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "polaris_provider_latency_seconds",
			Help:    "Upstream provider latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"model", "provider"}),
		tokensTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "polaris_tokens_total",
			Help: "Total tokens processed by Polaris.",
		}, []string{"model", "provider", "direction", "token_source"}),
		estimatedCost: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "polaris_estimated_cost_usd",
			Help: "Estimated cost in USD.",
		}, []string{"model", "provider", "cost_source"}),
		pricingLookups: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "polaris_pricing_lookups_total",
			Help: "Pricing catalog lookups by model and lookup status.",
		}, []string{"model", "status"}),
		rateLimitHits: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "polaris_rate_limit_hits_total",
			Help: "Total rate-limit rejections by key.",
		}, []string{"key_id"}),
		budgetDenials: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "polaris_budget_denials_total",
			Help: "Total hard budget denials by project.",
		}, []string{"project_id"}),
		providerErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "polaris_provider_errors_total",
			Help: "Total upstream provider errors by provider and error type.",
		}, []string{"provider", "error_type"}),
		failovers: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "polaris_failovers_total",
			Help: "Total successful failovers from one model to another.",
		}, []string{"from_model", "to_model"}),
		cacheEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "polaris_cache_events_total",
			Help: "Total response cache events by status and model.",
		}, []string{"status", "model"}),
		toolInvocations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "polaris_tool_invocations_total",
			Help: "Total local tool invocations by tool name and status.",
		}, []string{"tool", "status"}),
		mcpSessions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "polaris_mcp_requests_total",
			Help: "Total MCP broker requests by binding and status.",
		}, []string{"binding", "status"}),
		activeStreams: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "polaris_active_streams",
			Help: "Currently active streaming responses.",
		}, []string{"model", "provider"}),
	}

	registry.MustRegister(
		recorder.requestsTotal,
		recorder.requestDuration,
		recorder.providerLatency,
		recorder.tokensTotal,
		recorder.estimatedCost,
		recorder.pricingLookups,
		recorder.rateLimitHits,
		recorder.budgetDenials,
		recorder.providerErrors,
		recorder.failovers,
		recorder.cacheEvents,
		recorder.toolInvocations,
		recorder.mcpSessions,
		recorder.activeStreams,
	)

	return recorder
}

func (r *Recorder) Handler() http.Handler {
	if r == nil {
		return promhttp.Handler()
	}
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
}

func (r *Recorder) ObserveRequest(interfaceFamily string, model string, modality string, provider string, statusCode int, totalLatency time.Duration, providerLatencyMs int, promptTokens int, completionTokens int, tokenSource string, estimatedCost float64, costSource string, errorType string) {
	if r == nil {
		return
	}

	status := strconv.Itoa(statusCode)
	r.requestsTotal.WithLabelValues(interfaceFamily, model, modality, status, provider).Inc()
	r.requestDuration.WithLabelValues(interfaceFamily, model, modality, provider).Observe(totalLatency.Seconds())

	if providerLatencyMs > 0 {
		r.providerLatency.WithLabelValues(model, provider).Observe(float64(providerLatencyMs) / 1000)
	}
	if tokenSource == "" {
		tokenSource = "unavailable"
	}
	if promptTokens > 0 {
		r.tokensTotal.WithLabelValues(model, provider, "input", tokenSource).Add(float64(promptTokens))
	}
	if completionTokens > 0 {
		r.tokensTotal.WithLabelValues(model, provider, "output", tokenSource).Add(float64(completionTokens))
	}
	if estimatedCost > 0 {
		if costSource == "" {
			costSource = "unknown"
		}
		r.estimatedCost.WithLabelValues(model, provider, costSource).Add(estimatedCost)
	}
}

func (r *Recorder) IncPricingLookup(model string, status string) {
	if r == nil || model == "" {
		return
	}
	if status == "" {
		status = "unknown"
	}
	r.pricingLookups.WithLabelValues(model, status).Inc()
}

func (r *Recorder) IncRateLimit(keyID string) {
	if r == nil {
		return
	}
	r.rateLimitHits.WithLabelValues(keyID).Inc()
}

func (r *Recorder) IncBudgetDenial(projectID string) {
	if r == nil {
		return
	}
	r.budgetDenials.WithLabelValues(projectID).Inc()
}

func (r *Recorder) IncFailover(fromModel string, toModel string) {
	if r == nil {
		return
	}
	r.failovers.WithLabelValues(fromModel, toModel).Inc()
}

func (r *Recorder) IncProviderError(provider string, errorType string) {
	if r == nil || provider == "" || errorType == "" {
		return
	}
	r.providerErrors.WithLabelValues(provider, errorType).Inc()
}

func (r *Recorder) IncCacheEvent(status string, model string) {
	if r == nil || status == "" {
		return
	}
	r.cacheEvents.WithLabelValues(status, model).Inc()
}

func (r *Recorder) IncToolInvocation(tool string, status string) {
	if r == nil || tool == "" {
		return
	}
	if status == "" {
		status = "ok"
	}
	r.toolInvocations.WithLabelValues(tool, status).Inc()
}

func (r *Recorder) IncMCPRequest(binding string, status string) {
	if r == nil || binding == "" {
		return
	}
	if status == "" {
		status = "ok"
	}
	r.mcpSessions.WithLabelValues(binding, status).Inc()
}

func (r *Recorder) StartStream(model string, provider string) func() {
	if r == nil {
		return func() {}
	}

	r.activeStreams.WithLabelValues(model, provider).Inc()
	var once sync.Once

	return func() {
		once.Do(func() {
			r.activeStreams.WithLabelValues(model, provider).Dec()
		})
	}
}
