package handler

import (
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/store"
)

func TestFoldByProvider(t *testing.T) {
	rows := []store.UsageSummaryRow{
		{Key: "openai/gpt-4o", Requests: 2, TotalTokens: 160, CostUSD: 0.11, Errors: 1, ProviderLatencySumMs: 300},
		{Key: "openai/gpt-4o-mini", Requests: 3, TotalTokens: 30, CostUSD: 0.03, ProviderLatencySumMs: 150},
		{Key: "anthropic/claude-opus-4-8", Requests: 1, TotalTokens: 300, CostUSD: 0.30, ProviderLatencySumMs: 400},
	}
	folded := foldByProvider(rows)
	if len(folded) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(folded))
	}
	byKey := map[string]store.UsageSummaryRow{}
	for _, r := range folded {
		byKey[r.Key] = r
	}
	oa := byKey["openai"]
	if oa.Requests != 5 || oa.TotalTokens != 190 || oa.Errors != 1 {
		t.Fatalf("openai fold wrong: %#v", oa)
	}
	if oa.ProviderLatencySumMs != 450 {
		t.Fatalf("openai latency sum should combine: got %d", oa.ProviderLatencySumMs)
	}
	if byKey["anthropic"].Requests != 1 {
		t.Fatalf("anthropic fold wrong: %#v", byKey["anthropic"])
	}
}

func TestUsageRowJSONAverages(t *testing.T) {
	row := usageRowJSON(store.UsageSummaryRow{Key: "openai", Requests: 4, ProviderLatencySumMs: 800, TotalLatencySumMs: 1000})
	if row["avg_provider_latency_ms"] != 200.0 {
		t.Fatalf("avg provider latency = %v, want 200", row["avg_provider_latency_ms"])
	}
	if row["avg_total_latency_ms"] != 250.0 {
		t.Fatalf("avg total latency = %v, want 250", row["avg_total_latency_ms"])
	}

	zero := usageRowJSON(store.UsageSummaryRow{Key: "total"})
	if zero["avg_provider_latency_ms"] != 0.0 {
		t.Fatalf("zero-request avg must be 0, got %v", zero["avg_provider_latency_ms"])
	}
}
