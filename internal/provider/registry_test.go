package provider

import (
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestRegistryBuildsAvailableModelsAndAliases(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai": {
				APIKey:  "sk-openai",
				BaseURL: "https://api.openai.com/v1",
				Timeout: time.Minute,
				Models: map[string]config.ModelConfig{
					"gpt-4o": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming},
					},
				},
			},
			"anthropic": {
				BaseURL: "https://api.anthropic.com",
				Timeout: time.Minute,
				Models: map[string]config.ModelConfig{
					"claude-sonnet-4-6": {
						Modality: modality.ModalityChat,
					},
				},
			},
		},
		Routing: config.RoutingConfig{
			Aliases: map[string]string{
				"default-chat": "openai/gpt-4o",
				"premium-chat": "anthropic/claude-sonnet-4-6",
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if registry.Count() != 1 {
		t.Fatalf("expected 1 available model, got %d", registry.Count())
	}

	model, err := registry.ResolveModel("default-chat")
	if err != nil {
		t.Fatalf("ResolveModel(alias) error = %v", err)
	}
	if model.ID != "openai/gpt-4o" {
		t.Fatalf("expected alias to resolve to openai/gpt-4o, got %s", model.ID)
	}

	if len(warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d (%v)", len(warnings), warnings)
	}
	if !strings.Contains(strings.Join(warnings, " "), "anthropic") {
		t.Fatalf("expected anthropic warning, got %v", warnings)
	}
}
