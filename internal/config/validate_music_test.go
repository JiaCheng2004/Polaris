package config

import (
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestValidateRejectsMiniMaxMusicWithoutExplicitBaseURL(t *testing.T) {
	cfg := Default()
	cfg.Providers["minimax"] = ProviderConfig{
		APIKey:  "minimax-key",
		Timeout: time.Minute,
		Models: map[string]ModelConfig{
			"music-2.6": {
				Modality:     modality.ModalityMusic,
				Capabilities: []modality.Capability{modality.CapabilityMusicGeneration},
			},
		},
	}

	err := Validate(&cfg)
	if err == nil || !strings.Contains(err.Error(), "providers.minimax.base_url is required") {
		t.Fatalf("expected explicit MiniMax base_url validation error, got %v", err)
	}
}

func TestValidateRejectsUnknownMiniMaxMusicBaseURL(t *testing.T) {
	cfg := Default()
	cfg.Providers["minimax"] = ProviderConfig{
		APIKey:  "minimax-key",
		BaseURL: "https://example.com",
		Timeout: time.Minute,
		Models: map[string]ModelConfig{
			"music-2.6": {
				Modality:     modality.ModalityMusic,
				Capabilities: []modality.Capability{modality.CapabilityMusicGeneration},
			},
		},
	}

	err := Validate(&cfg)
	if err == nil || !strings.Contains(err.Error(), "providers.minimax.base_url") {
		t.Fatalf("expected MiniMax base_url validation error, got %v", err)
	}
}

func TestValidateAcceptsKnownMiniMaxMusicBaseURLs(t *testing.T) {
	for _, baseURL := range []string{"https://api.minimax.io", "https://api.minimaxi.com"} {
		t.Run(baseURL, func(t *testing.T) {
			cfg := Default()
			cfg.Providers["minimax"] = ProviderConfig{
				APIKey:  "minimax-key",
				BaseURL: baseURL,
				Timeout: time.Minute,
				Models: map[string]ModelConfig{
					"music-2.6": {
						Modality:     modality.ModalityMusic,
						Capabilities: []modality.Capability{modality.CapabilityMusicGeneration},
					},
				},
			}

			if err := Validate(&cfg); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}
