package reliability

import (
	"sync"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/transport"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type fakeSink struct {
	mu              sync.Mutex
	health          map[string]float64
	breaker         map[string]int
	transitions     int
	shed            map[string]int
	budgetExhausted int
}

func newFakeSink() *fakeSink {
	return &fakeSink{health: map[string]float64{}, breaker: map[string]int{}, shed: map[string]int{}}
}

func (s *fakeSink) SetProviderHealth(slug string, score float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.health[slug] = score
}
func (s *fakeSink) SetBreakerState(slug string, state int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.breaker[slug] = state
}
func (s *fakeSink) IncBreakerTransition(slug, from, to string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transitions++
}
func (s *fakeSink) IncShed(slug, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shed[reason]++
}
func (s *fakeSink) IncRetryBudgetExhausted(slug string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.budgetExhausted++
}

func newTestManager(cfg Config) (*Manager, *fakeClock, *fakeSink) {
	clk := &fakeClock{t: time.Unix(10000, 0)}
	sink := newFakeSink()
	return newManagerWithClock(cfg, sink, clk.now), clk, sink
}

func TestManagerObserveClassifiesFailures(t *testing.T) {
	m, _, _ := newTestManager(Config{})
	cases := []struct {
		info transport.AttemptInfo
		fail bool
	}{
		{transport.AttemptInfo{Slug: "p", Status: 200}, false},
		{transport.AttemptInfo{Slug: "p", Status: 400}, false}, // client error never poisons
		{transport.AttemptInfo{Slug: "p", Status: 404}, false},
		{transport.AttemptInfo{Slug: "p", Status: 429}, true},
		{transport.AttemptInfo{Slug: "p", Status: 500}, true},
		{transport.AttemptInfo{Slug: "p", Status: 503}, true},
	}
	for _, c := range cases {
		if got := isProviderFailure(c.info); got != c.fail {
			t.Fatalf("status %d: isProviderFailure=%v want %v", c.info.Status, got, c.fail)
		}
	}
	// Empty slug is ignored.
	m.Observe(transport.AttemptInfo{Slug: "", Status: 500})
	if len(m.Snapshot()) != 0 {
		t.Fatal("empty-slug attempt created an entry")
	}
}

func TestManagerObservePoisonsHealthAndOpensBreaker(t *testing.T) {
	m, _, sink := newTestManager(Config{Breaker: BreakerConfig{ErrorRate: 0.5, MinSamples: 10}})
	for i := 0; i < 12; i++ {
		m.Observe(transport.AttemptInfo{Slug: "openai", Status: 500, Latency: 10 * time.Millisecond})
	}
	h := m.Health("openai")
	if h.Breaker != Open {
		t.Fatalf("breaker = %v, want open after sustained 5xx", h.Breaker)
	}
	if h.ErrorRate < 0.99 {
		t.Fatalf("error rate = %f, want ~1", h.ErrorRate)
	}
	if sink.transitions == 0 {
		t.Fatal("no breaker transition metric emitted")
	}
}

func TestManagerAdmitBreakerOpen(t *testing.T) {
	m, _, sink := newTestManager(Config{Breaker: BreakerConfig{ErrorRate: 0.5, MinSamples: 5, OpenFor: time.Minute}})
	for i := 0; i < 6; i++ {
		m.Report("p", false, time.Millisecond)
	}
	release, reason := m.Admit("p")
	if reason != AdmitBreakerOpen {
		t.Fatalf("reason = %v, want breaker_open", reason)
	}
	release() // must be safe to call
	if sink.shed[AdmitBreakerOpen.String()] == 0 {
		t.Fatal("shed metric not incremented for breaker_open")
	}
}

func TestManagerBreakerRecoversAfterOpenFor(t *testing.T) {
	m, clk, _ := newTestManager(Config{Breaker: BreakerConfig{ErrorRate: 0.5, MinSamples: 5, OpenFor: 30 * time.Second, HalfOpenProbes: 1}})
	for i := 0; i < 6; i++ {
		m.Report("p", false, time.Millisecond)
	}
	if _, reason := m.Admit("p"); reason != AdmitBreakerOpen {
		t.Fatalf("admit while open = %v, want breaker_open", reason)
	}
	// Before OpenFor elapses, still open.
	clk.advance(10 * time.Second)
	if _, reason := m.Admit("p"); reason != AdmitBreakerOpen {
		t.Fatalf("admit after 10s = %v, want still breaker_open", reason)
	}
	// After OpenFor, a probe is admitted (half-open).
	clk.advance(25 * time.Second)
	release, reason := m.Admit("p")
	if reason != AdmitOK {
		t.Fatalf("admit after OpenFor = %v, want ok (half-open probe)", reason)
	}
	release()
	// A successful probe closes the breaker.
	m.Report("p", true, time.Millisecond)
	if m.Health("p").Breaker != Closed {
		t.Fatalf("breaker = %v after successful probe, want closed", m.Health("p").Breaker)
	}
}

func TestManagerAdmitConcurrencyCap(t *testing.T) {
	m, _, sink := newTestManager(Config{Shed: ShedConfig{Enabled: true, PerProviderMaxInflight: 2}})
	r1, reason := m.Admit("p")
	if reason != AdmitOK {
		t.Fatalf("first admit = %v", reason)
	}
	r2, reason := m.Admit("p")
	if reason != AdmitOK {
		t.Fatalf("second admit = %v", reason)
	}
	_, reason = m.Admit("p")
	if reason != AdmitProviderFull {
		t.Fatalf("third admit = %v, want provider_full", reason)
	}
	// Releasing frees a slot.
	r1()
	r3, reason := m.Admit("p")
	if reason != AdmitOK {
		t.Fatalf("admit after release = %v, want ok", reason)
	}
	r2()
	r3()
	if sink.shed[AdmitProviderFull.String()] == 0 {
		t.Fatal("provider_full shed metric not emitted")
	}
	if m.Health("p").Inflight != 0 {
		t.Fatalf("inflight = %d after all released, want 0", m.Health("p").Inflight)
	}
}

func TestManagerAdmitGlobalCap(t *testing.T) {
	m, _, _ := newTestManager(Config{Shed: ShedConfig{Enabled: true, GlobalMaxInflight: 1}})
	r1, ok := m.AdmitGlobal()
	if !ok {
		t.Fatal("first global admit rejected")
	}
	if _, ok := m.AdmitGlobal(); ok {
		t.Fatal("second global admit allowed beyond cap")
	}
	if m.GlobalInflight() != 1 {
		t.Fatalf("global inflight = %d, want 1", m.GlobalInflight())
	}
	r1()
	r2, ok := m.AdmitGlobal()
	if !ok {
		t.Fatal("global admit after release rejected")
	}
	r2()
	if m.GlobalInflight() != 0 {
		t.Fatalf("global inflight = %d after release, want 0", m.GlobalInflight())
	}
}

func TestManagerAdmitGlobalDisabled(t *testing.T) {
	m, _, _ := newTestManager(Config{}) // shed disabled
	for i := 0; i < 5; i++ {
		if _, ok := m.AdmitGlobal(); !ok {
			t.Fatal("global admit rejected while shedding disabled")
		}
	}
	if m.GlobalInflight() != 5 {
		t.Fatalf("global inflight = %d, want 5 (tracked even when uncapped)", m.GlobalInflight())
	}
}

func TestManagerAdmitReleaseIdempotent(t *testing.T) {
	m, _, _ := newTestManager(Config{Shed: ShedConfig{Enabled: true, PerProviderMaxInflight: 1}})
	r, _ := m.Admit("p")
	r()
	r() // double release must not underflow
	if got := m.Health("p").Inflight; got != 0 {
		t.Fatalf("provider inflight = %d after double release, want 0", got)
	}
}

func TestManagerAllowRetry(t *testing.T) {
	m, _, sink := newTestManager(Config{RetryBudgetRatio: 0.2})
	allowed := 0
	for i := 0; i < int(retryBudgetMinTokens)+20; i++ {
		if m.AllowRetry("p") {
			allowed++
		}
	}
	if allowed == 0 || allowed > int(retryBudgetMinTokens)+1 {
		t.Fatalf("allowed %d retries, want bounded ~%d", allowed, int(retryBudgetMinTokens))
	}
	if sink.budgetExhausted == 0 {
		t.Fatal("budget exhaustion metric not emitted")
	}
}

func TestManagerReconfigure(t *testing.T) {
	m, _, _ := newTestManager(Config{Breaker: BreakerConfig{ErrorRate: 0.5, MinSamples: 100}})
	// With MinSamples 100, 10 failures do not open.
	for i := 0; i < 10; i++ {
		m.Report("p", false, time.Millisecond)
	}
	if m.Health("p").Breaker != Closed {
		t.Fatal("breaker opened before reconfigure")
	}
	// Tighten to MinSamples 5; next failures open it.
	m.Reconfigure(Config{Breaker: BreakerConfig{ErrorRate: 0.5, MinSamples: 5}})
	for i := 0; i < 6; i++ {
		m.Report("p", false, time.Millisecond)
	}
	if m.Health("p").Breaker != Open {
		t.Fatal("breaker did not open after reconfigure tightened thresholds")
	}
}

func TestManagerSnapshot(t *testing.T) {
	m, _, _ := newTestManager(Config{})
	m.Report("a", true, time.Millisecond)
	m.Report("b", false, time.Millisecond)
	views := m.Snapshot()
	if len(views) != 2 {
		t.Fatalf("snapshot has %d entries, want 2", len(views))
	}
	seen := map[string]bool{}
	for _, v := range views {
		seen[v.Slug] = true
		if v.BreakerState == "" {
			t.Fatal("snapshot view missing breaker string")
		}
	}
	if !seen["a"] || !seen["b"] {
		t.Fatalf("snapshot missing providers: %v", seen)
	}
}

func TestManagerNilSink(t *testing.T) {
	m := NewManager(Config{}, nil)
	m.Report("p", true, time.Millisecond) // must not panic
	m.Close()
}
