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

func TestEstimateCostUSDUnknownPhase3ModelsDefaultToZero(t *testing.T) {
	cases := []string{
		"google/gemini-embedding-001",
		"openai/gpt-image-1",
		"openai/tts-1",
		"bytedance/doubao-asr-2.0",
	}

	for _, model := range cases {
		if got := EstimateCostUSD(model, 100, 50); got != 0 {
			t.Fatalf("expected zero cost for %s, got %.8f", model, got)
		}
	}
}
