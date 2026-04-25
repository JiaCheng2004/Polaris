package e2e

import (
	"context"
	"testing"
)

type liveSmokeCase struct {
	name       string
	modelNames []string
	run        func(t *testing.T, ctx context.Context, harness *liveSmokeHarness)
}

func registeredLiveSmokeCases() []liveSmokeCase {
	cases := []liveSmokeCase{
		{
			name: "models",
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				models, err := harness.client.ListModels(ctx, true)
				if err != nil {
					t.Fatalf("ListModels() error = %v", err)
				}
				if len(models.Data) == 0 {
					t.Fatalf("expected non-empty model catalog")
				}
			},
		},
	}
	cases = append(cases, liveSmokeCoreCases()...)
	cases = append(cases, liveSmokeByteDanceCases()...)
	cases = append(cases, liveSmokePlatformCases()...)
	cases = append(cases, liveSmokeUsageCase())
	return cases
}
