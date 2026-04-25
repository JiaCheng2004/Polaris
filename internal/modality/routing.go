package modality

import "strings"

const (
	CostTierLow      = "low"
	CostTierBalanced = "balanced"
	CostTierPremium  = "premium"

	LatencyTierFast        = "fast"
	LatencyTierBalanced    = "balanced"
	LatencyTierBestQuality = "best_quality"
)

type RoutingOptions struct {
	Providers           []string     `json:"providers,omitempty"`
	ExcludeProviders    []string     `json:"exclude_providers,omitempty"`
	Capabilities        []Capability `json:"capabilities,omitempty"`
	Statuses            []string     `json:"statuses,omitempty"`
	VerificationClasses []string     `json:"verification_classes,omitempty"`
	Prefer              []string     `json:"prefer,omitempty"`
	CostTier            string       `json:"cost_tier,omitempty"`
	LatencyTier         string       `json:"latency_tier,omitempty"`
}

func (r *RoutingOptions) Empty() bool {
	if r == nil {
		return true
	}
	return len(r.Providers) == 0 &&
		len(r.ExcludeProviders) == 0 &&
		len(r.Capabilities) == 0 &&
		len(r.Statuses) == 0 &&
		len(r.VerificationClasses) == 0 &&
		len(r.Prefer) == 0 &&
		strings.TrimSpace(r.CostTier) == "" &&
		strings.TrimSpace(r.LatencyTier) == ""
}
