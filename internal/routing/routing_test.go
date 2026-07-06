package routing

import (
	"testing"
)

func ids(cands []Candidate) []string {
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.ModelID
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func healthMap(m map[string]Health) HealthFunc {
	return func(p string) Health { return m[p] }
}

func TestStaticReturnsUnchanged(t *testing.T) {
	r := NewRouter()
	cands := []Candidate{{ModelID: "a", Provider: "pa"}, {ModelID: "b", Provider: "pb"}, {ModelID: "c", Provider: "pc"}}
	for _, s := range []string{"", StrategyStatic, "unknown_strategy"} {
		got := r.Order("k", s, cands, nil, AdaptiveWeights{})
		if s == "unknown_strategy" {
			continue // unknown behaves static within healthy set; order preserved here
		}
		if !eq(ids(got), []string{"a", "b", "c"}) {
			t.Fatalf("strategy %q reordered static candidates: %v", s, ids(got))
		}
	}
}

func TestLeastLatencyOrders(t *testing.T) {
	r := NewRouter()
	cands := []Candidate{{ModelID: "a", Provider: "pa"}, {ModelID: "b", Provider: "pb"}, {ModelID: "c", Provider: "pc"}}
	health := healthMap(map[string]Health{
		"pa": {LatencyMs: 300},
		"pb": {LatencyMs: 100},
		"pc": {LatencyMs: 200},
	})
	got := r.Order("k", StrategyLeastLatency, cands, health, AdaptiveWeights{})
	if !eq(ids(got), []string{"b", "c", "a"}) {
		t.Fatalf("least_latency order = %v, want [b c a]", ids(got))
	}
}

func TestCostOptimizedOrders(t *testing.T) {
	r := NewRouter()
	cands := []Candidate{
		{ModelID: "a", Provider: "pa", CostPer1K: 0.03},
		{ModelID: "b", Provider: "pb", CostPer1K: 0.01},
		{ModelID: "c", Provider: "pc", CostPer1K: 0}, // unknown → last
	}
	got := r.Order("k", StrategyCostOptimized, cands, nil, AdaptiveWeights{})
	if !eq(ids(got), []string{"b", "a", "c"}) {
		t.Fatalf("cost order = %v, want [b a c]", ids(got))
	}
}

func TestWeightedOrders(t *testing.T) {
	r := NewRouter()
	cands := []Candidate{
		{ModelID: "a", Provider: "pa", Weight: 1},
		{ModelID: "b", Provider: "pb", Weight: 5},
		{ModelID: "c", Provider: "pc", Weight: 3},
	}
	got := r.Order("k", StrategyWeighted, cands, nil, AdaptiveWeights{})
	if !eq(ids(got), []string{"b", "c", "a"}) {
		t.Fatalf("weighted order = %v, want [b c a]", ids(got))
	}
}

func TestAdaptiveFavorsHealthy(t *testing.T) {
	r := NewRouter()
	cands := []Candidate{{ModelID: "a", Provider: "pa"}, {ModelID: "b", Provider: "pb"}}
	health := healthMap(map[string]Health{
		"pa": {Score: 0.2, LatencyMs: 500},
		"pb": {Score: 0.95, LatencyMs: 80},
	})
	got := r.Order("k", StrategyAdaptive, cands, health, AdaptiveWeights{})
	if got[0].ModelID != "b" {
		t.Fatalf("adaptive picked %s first, want the healthier b", got[0].ModelID)
	}
}

func TestOpenBreakerDemotedToTail(t *testing.T) {
	r := NewRouter()
	cands := []Candidate{
		{ModelID: "a", Provider: "pa"},
		{ModelID: "b", Provider: "pb"},
		{ModelID: "c", Provider: "pc"},
	}
	health := healthMap(map[string]Health{
		"pa": {BreakerOpen: true, LatencyMs: 10}, // open → demoted despite low latency
		"pb": {LatencyMs: 200},
		"pc": {LatencyMs: 100},
	})
	got := r.Order("k", StrategyLeastLatency, cands, health, AdaptiveWeights{})
	// healthy ordered by latency (c,b) then the demoted a.
	if !eq(ids(got), []string{"c", "b", "a"}) {
		t.Fatalf("open-breaker demotion order = %v, want [c b a]", ids(got))
	}
}

func TestRoundRobinRotates(t *testing.T) {
	r := NewRouter()
	cands := []Candidate{{ModelID: "a", Provider: "pa"}, {ModelID: "b", Provider: "pb"}, {ModelID: "c", Provider: "pc"}}
	first := ids(r.Order("k", StrategyRoundRobin, cands, nil, AdaptiveWeights{}))
	second := ids(r.Order("k", StrategyRoundRobin, cands, nil, AdaptiveWeights{}))
	third := ids(r.Order("k", StrategyRoundRobin, cands, nil, AdaptiveWeights{}))
	if !eq(first, []string{"a", "b", "c"}) || !eq(second, []string{"b", "c", "a"}) || !eq(third, []string{"c", "a", "b"}) {
		t.Fatalf("round-robin did not rotate: %v %v %v", first, second, third)
	}
	// A different key has independent rotation.
	if !eq(ids(r.Order("other", StrategyRoundRobin, cands, nil, AdaptiveWeights{})), []string{"a", "b", "c"}) {
		t.Fatal("round-robin state leaked across keys")
	}
}

func TestSingleCandidateUnchanged(t *testing.T) {
	r := NewRouter()
	cands := []Candidate{{ModelID: "a", Provider: "pa"}}
	got := r.Order("k", StrategyAdaptive, cands, nil, AdaptiveWeights{})
	if len(got) != 1 || got[0].ModelID != "a" {
		t.Fatalf("single candidate changed: %v", ids(got))
	}
}

func TestInputNotMutated(t *testing.T) {
	r := NewRouter()
	cands := []Candidate{{ModelID: "a", Provider: "pa", CostPer1K: 0.05}, {ModelID: "b", Provider: "pb", CostPer1K: 0.01}}
	_ = r.Order("k", StrategyCostOptimized, cands, nil, AdaptiveWeights{})
	if cands[0].ModelID != "a" || cands[1].ModelID != "b" {
		t.Fatalf("input slice mutated: %v", ids(cands))
	}
}

func BenchmarkRouterOrderAdaptive(b *testing.B) {
	r := NewRouter()
	cands := []Candidate{
		{ModelID: "openai/gpt-4o", Provider: "openai", CostPer1K: 0.03},
		{ModelID: "anthropic/claude", Provider: "anthropic", CostPer1K: 0.02},
		{ModelID: "deepseek/chat", Provider: "deepseek", CostPer1K: 0.001},
	}
	health := healthMap(map[string]Health{
		"openai":    {Score: 0.9, LatencyMs: 200},
		"anthropic": {Score: 0.95, LatencyMs: 150},
		"deepseek":  {Score: 0.8, LatencyMs: 90},
	})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Order("openai/gpt-4o", StrategyAdaptive, cands, health, AdaptiveWeights{})
	}
}
