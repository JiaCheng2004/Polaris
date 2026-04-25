package e2e

import (
	"context"
	"os"
	"testing"
)

func TestLiveSmokeMatrix(t *testing.T) {
	if os.Getenv("POLARIS_LIVE_SMOKE") != "1" {
		t.Skip("POLARIS_LIVE_SMOKE is not set")
	}

	harness := newLiveSmokeHarness(t)
	ctx := context.Background()

	for _, tc := range registeredLiveSmokeCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t, ctx, harness)
		})
	}
}
