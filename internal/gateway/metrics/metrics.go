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

	providerHealth       *prometheus.GaugeVec
	breakerState         *prometheus.GaugeVec
	breakerTransitions   *prometheus.CounterVec
	shedTotal            *prometheus.CounterVec
	retryBudgetExhausted *prometheus.CounterVec
	usageDropped         *prometheus.CounterVec
	idempotentReplays    *prometheus.CounterVec
	rateLimitDegraded    *prometheus.CounterVec
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
		providerHealth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "polaris_provider_health_score",
			Help: "Provider health score in [0,1] (1 = healthy).",
		}, []string{"provider"}),
		breakerState: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "polaris_breaker_state",
			Help: "Circuit-breaker state per provider (0=closed, 1=open, 2=half_open).",
		}, []string{"provider"}),
		breakerTransitions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "polaris_breaker_transitions_total",
			Help: "Total circuit-breaker state transitions.",
		}, []string{"provider", "from", "to"}),
		shedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "polaris_shed_total",
			Help: "Total attempts shed by the reliability manager, by reason.",
		}, []string{"provider", "reason"}),
		retryBudgetExhausted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "polaris_retry_budget_exhausted_total",
			Help: "Total failover attempts denied because the retry budget was exhausted.",
		}, []string{"provider"}),
		usageDropped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "polaris_usage_dropped_total",
			Help: "Total usage/audit log rows dropped, by kind and reason.",
		}, []string{"kind", "reason"}),
		idempotentReplays: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "polaris_idempotent_replays_total",
			Help: "Total idempotency-key replays served from cache, by endpoint.",
		}, []string{"endpoint"}),
		rateLimitDegraded: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "polaris_ratelimit_degraded_total",
			Help: "Total requests where the primary rate limiter was unavailable, by fail mode and action.",
		}, []string{"mode", "action"}),
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
		recorder.providerHealth,
		recorder.breakerState,
		recorder.breakerTransitions,
		recorder.shedTotal,
		recorder.retryBudgetExhausted,
		recorder.usageDropped,
		recorder.idempotentReplays,
		recorder.rateLimitDegraded,
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

// SetProviderHealth records a provider's health score in [0,1].
func (r *Recorder) SetProviderHealth(provider string, score float64) {
	if r == nil || provider == "" {
		return
	}
	r.providerHealth.WithLabelValues(provider).Set(score)
}

// SetBreakerState records a provider's breaker state (0=closed, 1=open, 2=half_open).
func (r *Recorder) SetBreakerState(provider string, state int) {
	if r == nil || provider == "" {
		return
	}
	r.breakerState.WithLabelValues(provider).Set(float64(state))
}

// IncBreakerTransition records a breaker state transition.
func (r *Recorder) IncBreakerTransition(provider, from, to string) {
	if r == nil || provider == "" {
		return
	}
	r.breakerTransitions.WithLabelValues(provider, from, to).Inc()
}

// IncShed records an attempt shed by the reliability manager.
func (r *Recorder) IncShed(provider, reason string) {
	if r == nil || provider == "" {
		return
	}
	r.shedTotal.WithLabelValues(provider, reason).Inc()
}

// IncRetryBudgetExhausted records a failover denied by the retry budget.
func (r *Recorder) IncRetryBudgetExhausted(provider string) {
	if r == nil {
		return
	}
	if provider == "" {
		provider = "unknown"
	}
	r.retryBudgetExhausted.WithLabelValues(provider).Inc()
}

// IncUsageDropped records a dropped usage/audit log row.
func (r *Recorder) IncUsageDropped(kind, reason string) {
	if r == nil {
		return
	}
	if kind == "" {
		kind = "request"
	}
	if reason == "" {
		reason = "unknown"
	}
	r.usageDropped.WithLabelValues(kind, reason).Inc()
}

// IncIdempotentReplay records an idempotency-key replay served from cache.
func (r *Recorder) IncIdempotentReplay(endpoint string) {
	if r == nil {
		return
	}
	if endpoint == "" {
		endpoint = "unknown"
	}
	r.idempotentReplays.WithLabelValues(endpoint).Inc()
}

// IncRateLimitDegraded records a request where the primary rate limiter was
// unavailable. action is "fallback" (degraded to local counting), "allowed"
// (fail-open), or "rejected" (fail-closed 503).
func (r *Recorder) IncRateLimitDegraded(mode, action string) {
	if r == nil {
		return
	}
	if mode == "" {
		mode = "open"
	}
	if action == "" {
		action = "unknown"
	}
	r.rateLimitDegraded.WithLabelValues(mode, action).Inc()
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
