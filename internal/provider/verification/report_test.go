package verification

import (
	"strings"
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestBuildReportIncludesAliasesAndSelectors(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai": {
				APIKey: "test",
				Models: map[string]config.ModelConfig{
					"gpt-5.4": {Modality: "chat", Capabilities: []modality.Capability{"streaming", "function_calling"}},
				},
			},
			"anthropic": {
				APIKey: "test",
				Models: map[string]config.ModelConfig{
					"claude-sonnet-4-6": {Modality: "chat", Capabilities: []modality.Capability{"streaming", "function_calling"}},
				},
			},
		},
		Routing: config.RoutingConfig{
			Aliases: map[string]string{
				"default-chat": "openai/gpt-5.4",
			},
			Selectors: map[string]config.RoutingSelector{
				"tooling-chat": {
					Modality:     "chat",
					Capabilities: []modality.Capability{"streaming", "function_calling"},
					Providers:    []string{"anthropic", "openai"},
				},
			},
		},
	}

	report, err := BuildReport(cfg, nil, []string{"registry warning"})
	if err != nil {
		t.Fatalf("BuildReport() error = %v", err)
	}
	if !report.Valid() {
		t.Fatalf("expected valid report, got problems=%#v", report.Problems)
	}
	if report.Counts.Strict != 2 {
		t.Fatalf("strict count = %d, want 2", report.Counts.Strict)
	}
	if len(report.Aliases) != 1 || report.Aliases[0].ResolvesTo != "openai/gpt-5.4" {
		t.Fatalf("aliases = %#v", report.Aliases)
	}
	if len(report.Selectors) != 1 || report.Selectors[0].ResolvesTo != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("selectors = %#v", report.Selectors)
	}
	if !strings.Contains(report.Text(), "registry warning") {
		t.Fatalf("expected warning in text report, got %q", report.Text())
	}
}

func TestBuildReportFlagsMissingMetadata(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai": {
				APIKey: "test",
				Models: map[string]config.ModelConfig{
					"not-in-catalog": {Modality: "chat"},
				},
			},
		},
	}

	report, err := BuildReport(cfg, nil, nil)
	if err != nil {
		t.Fatalf("BuildReport() error = %v", err)
	}
	if report.Valid() {
		t.Fatalf("expected invalid report")
	}
	if len(report.Problems) == 0 {
		t.Fatalf("expected metadata problems")
	}
}
