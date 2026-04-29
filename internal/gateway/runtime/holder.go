package runtime

import (
	"sync/atomic"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/pricing"
	"github.com/JiaCheng2004/Polaris/internal/provider"
)

type Snapshot struct {
	Config     *config.Config
	Registry   *provider.Registry
	Pricing    pricing.Estimator
	StaticKeys map[string]config.StaticKeyConfig
}

type Holder struct {
	current atomic.Pointer[Snapshot]
	pricing pricing.Estimator
}

func NewHolder(cfg *config.Config, registry *provider.Registry) *Holder {
	return NewHolderWithPricing(cfg, registry, pricing.DefaultEstimator())
}

func NewHolderWithPricing(cfg *config.Config, registry *provider.Registry, estimator pricing.Estimator) *Holder {
	holder := &Holder{}
	if estimator == nil {
		estimator = pricing.DefaultEstimator()
	}
	holder.pricing = estimator
	holder.Swap(cfg, registry)
	return holder
}

func (h *Holder) Current() *Snapshot {
	if h == nil {
		return nil
	}
	return h.current.Load()
}

func (h *Holder) PricingHolder() *pricing.Holder {
	if h == nil {
		return nil
	}
	holder, _ := h.pricing.(*pricing.Holder)
	return holder
}

func (h *Holder) Swap(cfg *config.Config, registry *provider.Registry) *Snapshot {
	if h == nil {
		return nil
	}

	snapshot := &Snapshot{
		Config:     cfg,
		Registry:   registry,
		Pricing:    h.pricing,
		StaticKeys: compileStaticKeys(cfg),
	}
	h.current.Store(snapshot)
	return snapshot
}

func compileStaticKeys(cfg *config.Config) map[string]config.StaticKeyConfig {
	if cfg == nil {
		return nil
	}

	compiled := make(map[string]config.StaticKeyConfig, len(cfg.Auth.StaticKeys))
	for _, key := range cfg.Auth.StaticKeys {
		compiled[key.KeyHash] = key
	}
	return compiled
}
