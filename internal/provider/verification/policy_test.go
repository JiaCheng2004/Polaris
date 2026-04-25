package verification

import (
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/provider"
)

func TestForModelsMergesVerificationClasses(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai": {
				APIKey: "test",
				Models: map[string]config.ModelConfig{
					"gpt-5.4": {
						Modality: "chat",
					},
				},
			},
			"elevenlabs": {
				APIKey: "test",
				Models: map[string]config.ModelConfig{
					"music_v1": {
						Modality: "music",
					},
				},
			},
		},
		Routing: config.RoutingConfig{
			Aliases: map[string]string{
				"strict-chat":  "openai/gpt-5.4",
				"opt-in-music": "elevenlabs/music_v1",
			},
		},
	}

	registry, _, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}

	got, err := ForModels(registry, "strict-chat", "opt-in-music")
	if err != nil {
		t.Fatalf("ForModels() error = %v", err)
	}
	if got.Class != ClassStrict {
		t.Fatalf("verification class = %s, want %s", got.Class, ClassStrict)
	}
	if len(got.Providers) != 2 || got.Providers[0] != "elevenlabs" || got.Providers[1] != "openai" {
		t.Fatalf("providers = %#v", got.Providers)
	}
}

func TestForModelsRejectsMissingVerificationMetadata(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai": {
				APIKey: "test",
				Models: map[string]config.ModelConfig{
					"not-in-catalog": {
						Modality: "chat",
					},
				},
			},
		},
		Routing: config.RoutingConfig{
			Aliases: map[string]string{
				"missing-meta": "openai/not-in-catalog",
			},
		},
	}

	registry, _, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}

	if _, err := ForModels(registry, "missing-meta"); err == nil {
		t.Fatal("expected missing verification metadata error")
	}
}
