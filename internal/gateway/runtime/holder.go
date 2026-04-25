package runtime

import (
	"sync/atomic"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/provider"
)

type Snapshot struct {
	Config     *config.Config
	Registry   *provider.Registry
	StaticKeys map[string]config.StaticKeyConfig
}

type Holder struct {
	current atomic.Pointer[Snapshot]
}

func NewHolder(cfg *config.Config, registry *provider.Registry) *Holder {
	holder := &Holder{}
	holder.Swap(cfg, registry)
	return holder
}

func (h *Holder) Current() *Snapshot {
	if h == nil {
		return nil
	}
	return h.current.Load()
}

func (h *Holder) Swap(cfg *config.Config, registry *provider.Registry) *Snapshot {
	if h == nil {
		return nil
	}

	snapshot := &Snapshot{
		Config:     cfg,
		Registry:   registry,
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
