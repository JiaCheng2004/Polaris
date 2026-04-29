package middleware

import "testing"

func TestEstimateCostUSDChatModel(t *testing.T) {
	got := EstimateCostUSD("openai/gpt-4o", 1000, 500)
	want := 0.0125
	if got != want {
		t.Fatalf("expected %.7f, got %.7f", want, got)
	}
}

func TestEstimateCostUSDEmbeddingModel(t *testing.T) {
	got := EstimateCostUSD("openai/text-embedding-3-small", 8, 0)
	want := 0.00000016
	if got != want {
		t.Fatalf("expected %.8f, got %.8f", want, got)
	}
}

func TestEstimateCostUSDOriginalHardcodedModelsRemainCompatible(t *testing.T) {
	cases := []struct {
		model      string
		prompt     int
		completion int
		want       float64
	}{
		{"openai/gpt-4o-mini", 1000, 500, 0.00045},
		{"openai/text-embedding-3-large", 1000, 0, 0.00013},
		{"anthropic/claude-sonnet-4-6", 1000, 500, 0.0105},
		{"anthropic/claude-opus-4-6", 1000, 500, 0.0525},
	}

	for _, tc := range cases {
		if got := EstimateCostUSD(tc.model, tc.prompt, tc.completion); got != tc.want {
			t.Fatalf("expected %.8f for %s, got %.8f", tc.want, tc.model, got)
		}
	}
}

func TestEstimateCostUSDUnknownModelDefaultsToZero(t *testing.T) {
	if got := EstimateCostUSD("unknown/model", 100, 50); got != 0 {
		t.Fatalf("expected zero cost for unknown model, got %.8f", got)
	}
}
