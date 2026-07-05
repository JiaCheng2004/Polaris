// Package runtime holds the hot-reloadable configuration Snapshot (Holder) and
// the Reloader that atomically swaps it, plus the mapping from application
// config onto the process-lifetime reliability manager's tunables.
package runtime

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/reliability"
)

// ReliabilityConfig maps the application config onto reliability-manager
// tunables. The manager holds process-lifetime state; this supplies its
// thresholds and is re-applied on every hot reload.
func ReliabilityConfig(cfg *config.Config) reliability.Config {
	if cfg == nil {
		return reliability.Config{RetryBudgetRatio: 0.2}
	}
	r := cfg.Reliability
	return reliability.Config{
		Shed: reliability.ShedConfig{
			Enabled:                r.Shed.Enabled,
			GlobalMaxInflight:      r.Shed.GlobalMaxInflight,
			PerProviderMaxInflight: r.Shed.PerProviderMaxInflight,
		},
		Breaker: reliability.BreakerConfig{
			ErrorRate:      r.Breaker.ErrorRate,
			MinSamples:     r.Breaker.MinSamples,
			OpenFor:        r.Breaker.OpenFor,
			HalfOpenProbes: r.Breaker.HalfOpenProbes,
		},
		RetryBudgetRatio: r.RetryBudgetRatio,
	}
}
