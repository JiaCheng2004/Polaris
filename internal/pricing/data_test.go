package pricing

import (
	"io/fs"
	"testing"
)

func TestBundledDataRoundTrips(t *testing.T) {
	files, err := fs.Glob(embeddedData, "data/*.yaml")
	if err != nil {
		t.Fatalf("glob embedded data: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected bundled pricing data")
	}
	for _, file := range files {
		data, err := embeddedData.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		parsed, err := parseFile(data, file)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for key, entry := range parsed.Models {
			if !validMode(entry.Mode) {
				t.Fatalf("%s has invalid mode %q", key, entry.Mode)
			}
			if entry.Pricing == nil && len(entry.TieredPricing) == 0 {
				t.Fatalf("%s has no pricing", key)
			}
			for tier := range entry.Tiers {
				if !validTier(tier) {
					t.Fatalf("%s has invalid tier %q", key, tier)
				}
			}
		}
	}
}

func TestBundledDataCoversPrimaryConfiguredModels(t *testing.T) {
	catalog, err := LoadBundled()
	if err != nil {
		t.Fatalf("LoadBundled() error = %v", err)
	}
	for _, model := range []string{
		"openai/gpt-5.5",
		"openai/gpt-4o",
		"openai/gpt-image-2",
		"openai/gpt-4o-audio",
		"openai/sora-2",
		"anthropic/claude-sonnet-4-6",
		"deepseek/deepseek-chat",
		"google/gemini-2.5-flash-lite",
		"google/nano-banana-pro",
		"google-vertex/veo-3.1-fast-generate-001",
		"xai/grok-4-1-fast-reasoning",
		"qwen/qwen3.5-flash",
		"qwen/qwen-image-2.0",
		"bytedance/doubao-seed-2.0-pro",
		"bytedance/doubao-tts-2.0",
		"bytedance/doubao-seedance-2.0",
		"bytedance/seedream-4.5",
		"minimax/music-2.6",
		"elevenlabs/music_v1",
		"ollama/llama3",
	} {
		if _, ok := catalog.Lookup(model); !ok {
			t.Fatalf("expected bundled pricing for %s", model)
		}
	}
}
