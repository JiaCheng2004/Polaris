package e2e

import (
	"path/filepath"
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/provider/verification"
)

func TestLiveSmokeCasesHaveVerificationMetadata(t *testing.T) {
	t.Setenv("MINIMAX_BASE_URL", "https://api.minimax.io")

	cfg, warnings, err := config.Load(filepath.Join("..", "..", "config", "polaris.live-smoke.yaml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	_ = warnings

	registry, _, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}

	for _, tc := range registeredLiveSmokeCases() {
		if len(tc.modelNames) == 0 {
			continue
		}
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := verification.ForConfig(cfg, registry, tc.modelNames...); err != nil {
				t.Fatalf("verification.ForModels(%v) error = %v", tc.modelNames, err)
			}
		})
	}
}

func TestLiveSmokeOptInEnabled(t *testing.T) {
	t.Run("unset", func(t *testing.T) {
		t.Setenv("POLARIS_LIVE_SMOKE_INCLUDE_OPT_IN", "")
		if liveSmokeOptInEnabled("POLARIS_LIVE_SMOKE_INCLUDE_OPT_IN") {
			t.Fatalf("expected preview opt-in to be disabled when unset")
		}
	})

	t.Run("disabled", func(t *testing.T) {
		t.Setenv("POLARIS_LIVE_SMOKE_INCLUDE_OPT_IN", "0")
		if liveSmokeOptInEnabled("POLARIS_LIVE_SMOKE_INCLUDE_OPT_IN") {
			t.Fatalf("expected preview opt-in to stay disabled for 0")
		}
	})

	t.Run("enabled", func(t *testing.T) {
		t.Setenv("POLARIS_LIVE_SMOKE_INCLUDE_OPT_IN", "1")
		if !liveSmokeOptInEnabled("POLARIS_LIVE_SMOKE_INCLUDE_OPT_IN") {
			t.Fatalf("expected preview opt-in to enable for 1")
		}
	})
}

func TestLiveSmokeProviderOptInName(t *testing.T) {
	t.Parallel()

	got := liveSmokeProviderOptInName("google-vertex")
	if got != "POLARIS_LIVE_SMOKE_PROVIDER_GOOGLE_VERTEX" {
		t.Fatalf("provider opt-in env = %q", got)
	}
}
