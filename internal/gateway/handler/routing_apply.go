package handler

import (
	"path"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/reliability"
	"github.com/JiaCheng2004/Polaris/internal/routing"
	"github.com/gin-gonic/gin"
)

// routeTargets reorders candidate targets by the matching routing policy's
// strategy. With no matching policy (the default) or the "static" strategy it
// returns the input unchanged, so behavior is byte-identical to the configured
// fallback order until an operator opts into a strategy.
func (h *ChatHandler) routeTargets(c *gin.Context, targets []chatTarget, mod modality.Modality) []chatTarget {
	if len(targets) <= 1 || h.router == nil {
		return targets
	}
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil || snapshot.Config == nil {
		return targets
	}
	policy := matchRoutePolicy(snapshot.Config.Routing.Policies, targets[0].model.ID, mod)
	if policy == nil || policy.Strategy == "" || policy.Strategy == routing.StrategyStatic {
		return targets
	}

	byModel := make(map[string]chatTarget, len(targets))
	cands := make([]routing.Candidate, len(targets))
	for i, t := range targets {
		byModel[t.model.ID] = t
		cands[i] = routing.Candidate{ModelID: t.model.ID, Provider: t.model.Provider}
	}
	ordered := h.router.Order(targets[0].model.ID, policy.Strategy, cands, h.routeHealth(), routing.AdaptiveWeights{
		Success: policy.Weights.Success,
		Latency: policy.Weights.Latency,
		Cost:    policy.Weights.Cost,
	})
	out := make([]chatTarget, 0, len(ordered))
	for _, cand := range ordered {
		if t, ok := byModel[cand.ModelID]; ok {
			out = append(out, t)
		}
	}
	return out
}

func (h *ChatHandler) routeHealth() routing.HealthFunc {
	if h.reliability == nil {
		return nil
	}
	return func(provider string) routing.Health {
		v := h.reliability.Health(provider)
		return routing.Health{Score: v.Score, LatencyMs: v.EWMALatencyMs, BreakerOpen: v.Breaker == reliability.Open}
	}
}

func matchRoutePolicy(policies []config.RoutePolicy, modelID string, mod modality.Modality) *config.RoutePolicy {
	for i := range policies {
		p := &policies[i]
		if p.Match.Modality != "" && p.Match.Modality != string(mod) {
			continue
		}
		if p.Match.Model == "" {
			return p
		}
		if ok, err := path.Match(p.Match.Model, modelID); err == nil && ok {
			return p
		}
	}
	return nil
}
