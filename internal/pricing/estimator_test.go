package pricing

import "testing"

func TestEstimateChatCacheReasoningAndTiers(t *testing.T) {
	catalog, err := LoadBundled()
	if err != nil {
		t.Fatalf("LoadBundled() error = %v", err)
	}

	result := catalog.Estimate(EstimateRequest{
		Model:              "openai/o3",
		Tier:               "flex",
		InputTokens:        1000,
		CachedInputTokens:  100,
		CacheWrite5mTokens: 50,
		OutputTokens:       500,
		ReasoningTokens:    250,
	})

	want := 0.00085 + 0.000025 + 0.002 + 0.001
	if !closeEnough(result.TotalUSD, want) {
		t.Fatalf("expected %.9f, got %.9f (%#v)", want, result.TotalUSD, result.BreakdownUSD)
	}
	if result.Source != SourceTable || result.LookupStatus != LookupHit {
		t.Fatalf("unexpected source/status: %#v", result)
	}
}

func TestEstimateTieredDeploymentAdditionalAndUnits(t *testing.T) {
	catalog, err := LoadBundled()
	if err != nil {
		t.Fatalf("LoadBundled() error = %v", err)
	}

	qwen := catalog.Estimate(EstimateRequest{
		Model:        "qwen/qwen3-max",
		InputTokens:  64000,
		OutputTokens: 1000,
	})
	if qwen.EffectiveBand != "lte_128k" {
		t.Fatalf("expected lte_128k band, got %q", qwen.EffectiveBand)
	}
	if !closeEnough(qwen.TotalUSD, 0.1656) {
		t.Fatalf("unexpected qwen cost %.9f", qwen.TotalUSD)
	}

	anthropic := catalog.Estimate(EstimateRequest{
		Model:        "anthropic/claude-sonnet-4-6",
		Deployment:   "us",
		InputTokens:  1000,
		OutputTokens: 1000,
		UnitCounts:   map[string]int{"web_search": 1000},
	})
	if !closeEnough(anthropic.TotalUSD, 10.0198) {
		t.Fatalf("unexpected anthropic cost %.9f (%#v)", anthropic.TotalUSD, anthropic.BreakdownUSD)
	}

	dalle := catalog.Estimate(EstimateRequest{Model: "openai/dall-e-3", Images: 2})
	if !closeEnough(dalle.TotalUSD, 0.08) {
		t.Fatalf("unexpected image cost %.9f", dalle.TotalUSD)
	}

	whisper := catalog.Estimate(EstimateRequest{Model: "openai/whisper-1", AudioSeconds: 60})
	if !closeEnough(whisper.TotalUSD, 0.006) {
		t.Fatalf("unexpected whisper cost %.9f", whisper.TotalUSD)
	}

	tts := catalog.Estimate(EstimateRequest{Model: "openai/tts-1", Characters: 1000})
	if !closeEnough(tts.TotalUSD, 0.015) {
		t.Fatalf("unexpected tts cost %.9f", tts.TotalUSD)
	}

	video := catalog.Estimate(EstimateRequest{Model: "bytedance/seedance-2-0", VideoSeconds: 5})
	if !closeEnough(video.TotalUSD, 0.70) {
		t.Fatalf("unexpected video cost %.9f", video.TotalUSD)
	}
}

func TestEstimateMissingAndWildcard(t *testing.T) {
	catalog, err := LoadBundled()
	if err != nil {
		t.Fatalf("LoadBundled() error = %v", err)
	}

	missing := catalog.Estimate(EstimateRequest{Model: "unknown/model", InputTokens: 100, OutputTokens: 100})
	if missing.TotalUSD != 0 || missing.Source != SourceMissing || missing.LookupStatus != LookupMiss {
		t.Fatalf("unexpected missing result %#v", missing)
	}

	ollama := catalog.Estimate(EstimateRequest{Model: "ollama/llama3.3", InputTokens: 100, OutputTokens: 100})
	if ollama.TotalUSD != 0 || ollama.Source != SourceTable || ollama.LookupStatus != LookupGlobHit {
		t.Fatalf("unexpected ollama result %#v", ollama)
	}
}

func closeEnough(got float64, want float64) bool {
	const epsilon = 0.000000001
	if got > want {
		return got-want < epsilon
	}
	return want-got < epsilon
}
