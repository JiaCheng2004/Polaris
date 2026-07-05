// Package reliability provides Polaris's process-lifetime resilience state:
// per-provider health scoring (EWMA latency + sliding error-rate window),
// circuit breaking, load shedding (concurrency caps), and retry budgets. A
// single Manager is constructed in main.go, registered as a transport observer
// so it sees every upstream attempt, and consulted by the failover path for
// admission. State lives here (not in the reloadable config Snapshot) so it
// survives hot-reload swaps; Reconfigure applies new thresholds in place.
package reliability

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/transport"
)

// MetricsSink receives reliability metric updates. The gateway metrics.Recorder
// satisfies it structurally, keeping this package below the gateway layer.
type MetricsSink interface {
	SetProviderHealth(slug string, score float64)
	SetBreakerState(slug string, state int)
	IncBreakerTransition(slug, from, to string)
	IncShed(slug, reason string)
	IncRetryBudgetExhausted(slug string)
}

type noopMetrics struct{}

func (noopMetrics) SetProviderHealth(string, float64)           {}
func (noopMetrics) SetBreakerState(string, int)                 {}
func (noopMetrics) IncBreakerTransition(string, string, string) {}
func (noopMetrics) IncShed(string, string)                      {}
func (noopMetrics) IncRetryBudgetExhausted(string)              {}

// Config holds reliability tunables, sourced from runtime.reliability config.
type Config struct {
	Shed             ShedConfig
	Breaker          BreakerConfig
	RetryBudgetRatio float64
}

// ShedConfig configures load shedding. All zero = disabled (unlimited).
type ShedConfig struct {
	Enabled                bool
	GlobalMaxInflight      int
	PerProviderMaxInflight int
}

// AdmitReason explains an Admit decision.
type AdmitReason int

const (
	// AdmitOK means the attempt may proceed.
	AdmitOK AdmitReason = iota
	// AdmitBreakerOpen means the provider's breaker is open (demote/skip).
	AdmitBreakerOpen
	// AdmitGlobalFull means the global in-flight cap is reached (shed).
	AdmitGlobalFull
	// AdmitProviderFull means the per-provider cap is reached (shed).
	AdmitProviderFull
)

func (r AdmitReason) String() string {
	switch r {
	case AdmitBreakerOpen:
		return "breaker_open"
	case AdmitGlobalFull:
		return "global_full"
	case AdmitProviderFull:
		return "provider_full"
	default:
		return "ok"
	}
}

// HealthView is a read-only snapshot of one provider's reliability state.
type HealthView struct {
	Slug          string       `json:"provider"`
	Samples       int          `json:"samples"`
	ErrorRate     float64      `json:"error_rate"`
	EWMALatencyMs float64      `json:"ewma_latency_ms"`
	Score         float64      `json:"health_score"`
	Breaker       BreakerState `json:"-"`
	BreakerState  string       `json:"breaker"`
	Inflight      int64        `json:"inflight"`
}

type entry struct {
	slug     string
	health   *healthWindow
	breaker  *breaker
	inflight atomic.Int64
	lastSeen atomic.Int64 // unix nanos
}

// Manager is the process-lifetime reliability state for all providers.
type Manager struct {
	entries        sync.Map // slug -> *entry
	cfg            atomic.Pointer[Config]
	metrics        MetricsSink
	budget         *retryBudget
	globalInflight atomic.Int64
	now            func() time.Time
	closed         atomic.Bool
}

// NewManager builds a Manager. A nil sink is replaced with a no-op.
func NewManager(cfg Config, sink MetricsSink) *Manager {
	return newManagerWithClock(cfg, sink, time.Now)
}

func newManagerWithClock(cfg Config, sink MetricsSink, now func() time.Time) *Manager {
	if sink == nil {
		sink = noopMetrics{}
	}
	m := &Manager{metrics: sink, now: now, budget: newRetryBudget(cfg.RetryBudgetRatio)}
	m.cfg.Store(&cfg)
	return m
}

func (m *Manager) config() Config {
	if c := m.cfg.Load(); c != nil {
		return *c
	}
	return Config{}
}

func (m *Manager) entryFor(slug string) *entry {
	if existing, ok := m.entries.Load(slug); ok {
		return existing.(*entry)
	}
	cfg := m.config()
	e := &entry{slug: slug, health: newHealthWindow(), breaker: newBreaker(cfg.Breaker)}
	actual, _ := m.entries.LoadOrStore(slug, e)
	return actual.(*entry)
}

// isProviderFailure reports whether an attempt outcome is provider-attributable
// and should poison health: transport errors, 429, and 5xx. Client 4xx never do.
func isProviderFailure(info transport.AttemptInfo) bool {
	if info.Err != nil {
		return true
	}
	if info.Status == 429 || info.Status >= 500 {
		return true
	}
	return false
}

// Observe is the transport.AttemptHook. It records every upstream attempt.
func (m *Manager) Observe(info transport.AttemptInfo) {
	if info.Slug == "" {
		return
	}
	m.report(info.Slug, !isProviderFailure(info), info.Latency)
}

// Report records an outcome for a provider key (exposed for the routing Executor
// and tests).
func (m *Manager) Report(slug string, success bool, latency time.Duration) {
	if slug == "" {
		return
	}
	m.report(slug, success, latency)
}

func (m *Manager) report(slug string, success bool, latency time.Duration) {
	now := m.now()
	e := m.entryFor(slug)
	e.lastSeen.Store(now.UnixNano())
	samples, errRate, ewma := e.health.record(success, latency, now)
	transitioned, from, to := e.breaker.record(success, errRate, samples, now)
	if success {
		m.budget.recordSuccess(now)
	}
	m.metrics.SetProviderHealth(slug, healthScore(errRate, ewma))
	m.metrics.SetBreakerState(slug, int(to))
	if transitioned {
		m.metrics.IncBreakerTransition(slug, from.String(), to.String())
	}
}

// Admit decides whether an attempt against slug may proceed. On AdmitOK it
// returns a release func that MUST be called exactly once when the attempt
// finishes. On any other reason the release is a no-op and the caller should
// demote/skip this target.
func (m *Manager) Admit(slug string) (release func(), reason AdmitReason) {
	e := m.entryFor(slug)
	now := m.now()

	allowed, transitioned, from, to := e.breaker.allow(now)
	if transitioned {
		m.metrics.IncBreakerTransition(slug, from.String(), to.String())
		m.metrics.SetBreakerState(slug, int(to))
	}
	if !allowed {
		m.metrics.IncShed(slug, AdmitBreakerOpen.String())
		return noopRelease, AdmitBreakerOpen
	}

	cfg := m.config()
	if cfg.Shed.Enabled && cfg.Shed.PerProviderMaxInflight > 0 {
		if e.inflight.Add(1) > int64(cfg.Shed.PerProviderMaxInflight) {
			e.inflight.Add(-1)
			m.metrics.IncShed(slug, AdmitProviderFull.String())
			return noopRelease, AdmitProviderFull
		}
	} else {
		e.inflight.Add(1)
	}

	var once sync.Once
	return func() {
		once.Do(func() { e.inflight.Add(-1) })
	}, AdmitOK
}

// AdmitGlobal enforces the front-door global in-flight cap (all requests, every
// modality). The shed middleware calls it once per request. It returns a release
// that MUST be called when the request finishes, and whether the request was
// admitted (false = over the global cap → the caller should return 503).
func (m *Manager) AdmitGlobal() (release func(), admitted bool) {
	cfg := m.config()
	if !cfg.Shed.Enabled || cfg.Shed.GlobalMaxInflight <= 0 {
		m.globalInflight.Add(1)
		var once sync.Once
		return func() { once.Do(func() { m.globalInflight.Add(-1) }) }, true
	}
	if m.globalInflight.Add(1) > int64(cfg.Shed.GlobalMaxInflight) {
		m.globalInflight.Add(-1)
		m.metrics.IncShed("global", AdmitGlobalFull.String())
		return noopRelease, false
	}
	var once sync.Once
	return func() { once.Do(func() { m.globalInflight.Add(-1) }) }, true
}

// GlobalInflight reports the current global in-flight request count (for /ready).
func (m *Manager) GlobalInflight() int64 { return m.globalInflight.Load() }

func noopRelease() {}

// AllowRetry reports whether one more failover/retry attempt is within the
// global retry budget, consuming a token if so. slug is used only for metric
// attribution.
func (m *Manager) AllowRetry(slug string) bool {
	if m.budget.allowRetry(m.now()) {
		return true
	}
	m.metrics.IncRetryBudgetExhausted(slug)
	return false
}

// Health returns a read-only snapshot for one provider.
func (m *Manager) Health(slug string) HealthView {
	e := m.entryFor(slug)
	samples, errRate, ewma := e.health.stats(m.now())
	state := e.breaker.currentState()
	return HealthView{
		Slug:          slug,
		Samples:       samples,
		ErrorRate:     errRate,
		EWMALatencyMs: ewma,
		Score:         healthScore(errRate, ewma),
		Breaker:       state,
		BreakerState:  state.String(),
		Inflight:      e.inflight.Load(),
	}
}

// Snapshot returns health views for every tracked provider (for /ready).
func (m *Manager) Snapshot() []HealthView {
	now := m.now()
	var views []HealthView
	m.entries.Range(func(_, v any) bool {
		e := v.(*entry)
		samples, errRate, ewma := e.health.stats(now)
		state := e.breaker.currentState()
		views = append(views, HealthView{
			Slug:          e.slug,
			Samples:       samples,
			ErrorRate:     errRate,
			EWMALatencyMs: ewma,
			Score:         healthScore(errRate, ewma),
			Breaker:       state,
			BreakerState:  state.String(),
			Inflight:      e.inflight.Load(),
		})
		return true
	})
	return views
}

// Reconfigure applies new thresholds in place (hot-reload). Entry state (health
// windows, in-flight, breaker phase) is preserved; only tunables change.
func (m *Manager) Reconfigure(cfg Config) {
	m.cfg.Store(&cfg)
	m.budget.reconfigure(cfg.RetryBudgetRatio)
	m.entries.Range(func(_, v any) bool {
		v.(*entry).breaker.reconfigure(cfg.Breaker)
		return true
	})
}

// Close marks the manager closed. It currently holds no background goroutines
// (the entry map is bounded by provider count), but the hook exists for the
// shutdown sequence and future probe/hedge loops.
func (m *Manager) Close() { m.closed.Store(true) }
