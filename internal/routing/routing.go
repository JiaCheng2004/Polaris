// Package routing orders provider candidates for a request according to a
// per-route strategy (static, round-robin, weighted, least-latency,
// cost-optimized, adaptive), consuming live provider health from the reliability
// manager. The default "static" strategy returns candidates unchanged, so
// behavior is byte-identical to the configured fallback order until an operator
// opts into a strategy. A Router holds the small amount of cross-request state
// that round-robin needs; every other strategy is a pure function of the inputs.
package routing

import (
	"sort"
	"sync"
)

// Strategy names.
const (
	StrategyStatic        = "static"
	StrategyRoundRobin    = "round_robin"
	StrategyWeighted      = "weighted"
	StrategyLeastLatency  = "least_latency"
	StrategyCostOptimized = "cost_optimized"
	StrategyAdaptive      = "adaptive"
)

// Candidate is one routable target (a resolved model on a provider).
type Candidate struct {
	ModelID   string
	Provider  string  // slug used for health lookup
	Weight    int     // relative weight for weighted/round-robin (default 1)
	CostPer1K float64 // estimated $/1K tokens for cost/adaptive (0 = unknown)
}

// Health is a provider's current health for routing decisions.
type Health struct {
	Score       float64 // 0..1, 1 = healthy
	LatencyMs   float64 // EWMA latency estimate
	BreakerOpen bool
}

// HealthFunc returns health for a provider slug. A nil HealthFunc yields
// zero-value (neutral) health for every provider.
type HealthFunc func(provider string) Health

// AdaptiveWeights tunes the adaptive score. Zero values fall back to defaults
// (0.5 success, 0.3 latency, 0.2 cost).
type AdaptiveWeights struct {
	Success float64
	Latency float64
	Cost    float64
}

func (w AdaptiveWeights) withDefaults() AdaptiveWeights {
	if w.Success == 0 && w.Latency == 0 && w.Cost == 0 {
		return AdaptiveWeights{Success: 0.5, Latency: 0.3, Cost: 0.2}
	}
	return w
}

// Router applies a strategy to order candidates. It is safe for concurrent use.
type Router struct {
	mu sync.Mutex
	rr map[string]int
}

// NewRouter builds an empty Router.
func NewRouter() *Router { return &Router{rr: make(map[string]int)} }

// Order returns candidates ordered by strategy. Open-breaker targets are demoted
// to the tail (never removed, so a single-provider deployment still attempts the
// best available). key identifies the route for round-robin state. The input
// slice is never mutated. For the static strategy the input order is returned
// unchanged.
func (r *Router) Order(key, strategy string, candidates []Candidate, health HealthFunc, weights AdaptiveWeights) []Candidate {
	if len(candidates) <= 1 || strategy == "" || strategy == StrategyStatic {
		return candidates
	}
	if health == nil {
		health = func(string) Health { return Health{} }
	}

	// Partition into healthy (breaker closed) and demoted (breaker open),
	// preserving relative order within each partition.
	healthy := make([]Candidate, 0, len(candidates))
	demoted := make([]Candidate, 0)
	for _, c := range candidates {
		if health(c.Provider).BreakerOpen {
			demoted = append(demoted, c)
		} else {
			healthy = append(healthy, c)
		}
	}

	switch strategy {
	case StrategyLeastLatency:
		healthy = orderLeastLatency(healthy, health)
	case StrategyCostOptimized:
		healthy = orderCost(healthy)
	case StrategyAdaptive:
		healthy = orderAdaptive(healthy, health, weights.withDefaults())
	case StrategyWeighted:
		healthy = orderWeighted(healthy)
	case StrategyRoundRobin:
		healthy = r.orderRoundRobin(key, healthy)
	default:
		// Unknown strategy behaves as static within the healthy set.
	}
	return append(healthy, demoted...)
}

// orderLeastLatency sorts by EWMA latency ascending (stable; unknown latency
// sorts as best so a cold provider gets a chance).
func orderLeastLatency(cands []Candidate, health HealthFunc) []Candidate {
	out := append([]Candidate(nil), cands...)
	sort.SliceStable(out, func(i, j int) bool {
		return health(out[i].Provider).LatencyMs < health(out[j].Provider).LatencyMs
	})
	return out
}

// orderCost sorts by estimated cost ascending; unknown cost (0) sorts last so a
// priced provider is preferred.
func orderCost(cands []Candidate) []Candidate {
	out := append([]Candidate(nil), cands...)
	sort.SliceStable(out, func(i, j int) bool {
		ci, cj := out[i].CostPer1K, out[j].CostPer1K
		if ci == 0 {
			return false
		}
		if cj == 0 {
			return true
		}
		return ci < cj
	})
	return out
}

// orderAdaptive sorts by a blended score descending.
func orderAdaptive(cands []Candidate, health HealthFunc, w AdaptiveWeights) []Candidate {
	maxLat, maxCost := 1.0, 1.0
	for _, c := range cands {
		if l := health(c.Provider).LatencyMs; l > maxLat {
			maxLat = l
		}
		if c.CostPer1K > maxCost {
			maxCost = c.CostPer1K
		}
	}
	score := func(c Candidate) float64 {
		h := health(c.Provider)
		latNorm := h.LatencyMs / maxLat
		costNorm := c.CostPer1K / maxCost
		return w.Success*h.Score + w.Latency*(1-latNorm) + w.Cost*(1-costNorm)
	}
	out := append([]Candidate(nil), cands...)
	sort.SliceStable(out, func(i, j int) bool { return score(out[i]) > score(out[j]) })
	return out
}

// orderWeighted sorts by weight descending (stable), so higher-weight targets
// are preferred while the order remains deterministic.
func orderWeighted(cands []Candidate) []Candidate {
	out := append([]Candidate(nil), cands...)
	sort.SliceStable(out, func(i, j int) bool { return weightOf(out[i]) > weightOf(out[j]) })
	return out
}

// orderRoundRobin rotates the healthy set by a per-key counter so successive
// requests start at different candidates.
func (r *Router) orderRoundRobin(key string, cands []Candidate) []Candidate {
	if len(cands) <= 1 {
		return cands
	}
	r.mu.Lock()
	start := r.rr[key] % len(cands)
	r.rr[key] = (r.rr[key] + 1) % len(cands)
	r.mu.Unlock()
	out := make([]Candidate, 0, len(cands))
	out = append(out, cands[start:]...)
	out = append(out, cands[:start]...)
	return out
}

func weightOf(c Candidate) int {
	if c.Weight <= 0 {
		return 1
	}
	return c.Weight
}
