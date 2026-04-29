package integration

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/pricing"
)

func TestPricingHotReloadSwapsCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pricing.yaml")
	writePricingFile(t, path, 1)

	catalog, _, err := pricing.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	holder := pricing.NewHolder(catalog)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pricing.WatchFile(ctx, holder, path, 10*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if got := holder.Estimate(pricing.EstimateRequest{Model: "openai/gpt-4o", InputTokens: 1_000_000}).TotalUSD; got != 1 {
		t.Fatalf("expected initial override cost 1, got %.2f", got)
	}

	time.Sleep(20 * time.Millisecond)
	writePricingFile(t, path, 9)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got := holder.Estimate(pricing.EstimateRequest{Model: "openai/gpt-4o", InputTokens: 1_000_000}).TotalUSD
		if got == 9 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("pricing override did not reload before deadline")
}

func writePricingFile(t *testing.T, path string, inputRate int) {
	t.Helper()
	data := []byte(`version: 1
models:
  openai/gpt-4o:
    mode: chat
    pricing: { input_per_mtok: ` + strconv.Itoa(inputRate) + `, output_per_mtok: 1 }
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write pricing file: %v", err)
	}
}
