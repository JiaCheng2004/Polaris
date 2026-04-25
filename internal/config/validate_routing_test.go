package config

import (
	"strings"
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestValidateRejectsInvalidRoutingSelectorStatus(t *testing.T) {
	cfg := Default()
	cfg.Providers["openai"] = ProviderConfig{
		APIKey: "sk-openai",
		Models: map[string]ModelConfig{
			"gpt-4o": {
				Modality:     modality.ModalityChat,
				Capabilities: []modality.Capability{modality.CapabilityStreaming},
			},
		},
	}
	cfg.Routing.Selectors["intent-chat"] = RoutingSelector{
		Modality: modality.ModalityChat,
		Statuses: []string{"broken"},
	}

	err := Validate(&cfg)
	if err == nil || !strings.Contains(err.Error(), `routing.selectors.intent-chat has invalid status "broken"`) {
		t.Fatalf("expected selector status validation error, got %v", err)
	}
}

func TestValidateAcceptsRoutingSelector(t *testing.T) {
	cfg := Default()
	cfg.Providers["openai"] = ProviderConfig{
		APIKey: "sk-openai",
		Models: map[string]ModelConfig{
			"gpt-4o": {
				Modality:     modality.ModalityChat,
				Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityFunctionCalling},
			},
		},
	}
	cfg.Routing.Selectors["intent-chat"] = RoutingSelector{
		Modality:            modality.ModalityChat,
		Capabilities:        []modality.Capability{modality.CapabilityStreaming},
		Providers:           []string{"openai"},
		Statuses:            []string{"ga"},
		VerificationClasses: []string{"strict"},
		Prefer:              []string{"openai/gpt-4o"},
	}

	if err := Validate(&cfg); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
