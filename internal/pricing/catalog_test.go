package pricing

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadBundledAndLookup(t *testing.T) {
	catalog, err := LoadBundled()
	if err != nil {
		t.Fatalf("LoadBundled() error = %v", err)
	}
	if _, ok := catalog.Lookup("openai/gpt-4o"); !ok {
		t.Fatalf("expected bundled openai/gpt-4o pricing")
	}
	if _, ok := catalog.Lookup("ollama/llama3.3"); !ok {
		t.Fatalf("expected ollama wildcard pricing")
	}
}

func TestLoadOverridePrecedenceAndEnvExpansion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pricing.yaml")
	t.Setenv("PRICE_SOURCE", "https://example.test/pricing")
	if err := os.WriteFile(path, []byte(`
version: 1
models:
  openai/gpt-4o:
    mode: chat
    source: ${PRICE_SOURCE}
    pricing: { input_per_mtok: 99.00, output_per_mtok: 101.00 }
  custom/model:
    mode: chat
    pricing: { input_per_mtok: 1.00, output_per_mtok: 2.00 }
`), 0o600); err != nil {
		t.Fatalf("write override: %v", err)
	}

	catalog, warnings, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	entry, ok := catalog.Lookup("openai/gpt-4o")
	if !ok {
		t.Fatalf("expected override entry")
	}
	if entry.Pricing == nil || entry.Pricing.InputPerMTok != 99 || entry.Source != "https://example.test/pricing" {
		t.Fatalf("override did not win: %#v", entry)
	}
	if _, ok := catalog.Lookup("custom/model"); !ok {
		t.Fatalf("expected custom model from override")
	}
}

func TestHolderSwap(t *testing.T) {
	first := newCatalog(map[string]Entry{
		"test/model": {Mode: "chat", Currency: "USD", Pricing: &Rates{InputPerMTok: 1}},
	}, time.Now().UTC(), []string{"first"})
	second := newCatalog(map[string]Entry{
		"test/model": {Mode: "chat", Currency: "USD", Pricing: &Rates{InputPerMTok: 2}},
	}, time.Now().UTC(), []string{"second"})

	holder := NewHolder(first)
	if got := holder.Estimate(EstimateRequest{Model: "test/model", InputTokens: 1_000_000}).TotalUSD; got != 1 {
		t.Fatalf("expected first catalog cost 1, got %.2f", got)
	}
	holder.Swap(second)
	if got := holder.Estimate(EstimateRequest{Model: "test/model", InputTokens: 1_000_000}).TotalUSD; got != 2 {
		t.Fatalf("expected swapped catalog cost 2, got %.2f", got)
	}
}

func TestInvalidYAMLRejected(t *testing.T) {
	_, err := parseFile([]byte(`
version: 1
models:
  badmodel:
    mode: chat
    pricing: { input_per_mtok: -1 }
`), "test.yaml")
	if err == nil {
		t.Fatalf("expected invalid pricing yaml to be rejected")
	}
}
