package provider

import (
	"errors"
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

func TestRegistryRegistersPhase2BProviders(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"deepseek": {
				APIKey:  "sk-deepseek",
				BaseURL: "https://api.deepseek.com/v1",
				Timeout: time.Minute,
				Models: map[string]config.ModelConfig{
					"deepseek-chat": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming},
					},
				},
			},
			"xai": {
				APIKey:  "sk-xai",
				BaseURL: "https://api.x.ai/v1",
				Timeout: time.Minute,
				Models: map[string]config.ModelConfig{
					"grok-3": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming},
					},
				},
			},
			"ollama": {
				BaseURL: "http://localhost:11434",
				Timeout: 5 * time.Minute,
				Models: map[string]config.ModelConfig{
					"llama3": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming},
					},
				},
			},
		},
		Routing: config.RoutingConfig{
			Aliases: map[string]string{
				"budget-chat": "deepseek/deepseek-chat",
				"local-chat":  "ollama/llama3",
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if registry.Count() != 3 {
		t.Fatalf("expected 3 models, got %d", registry.Count())
	}
	if _, _, err := registry.GetChatAdapter("deepseek/deepseek-chat"); err != nil {
		t.Fatalf("GetChatAdapter(deepseek) error = %v", err)
	}
	if _, _, err := registry.GetChatAdapter("xai/grok-3"); err != nil {
		t.Fatalf("GetChatAdapter(xai) error = %v", err)
	}
	if _, _, err := registry.GetChatAdapter("local-chat"); err != nil {
		t.Fatalf("GetChatAdapter(ollama alias) error = %v", err)
	}
}

func TestRegistryRegistersPhaseAProviderFamilies(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openrouter": {
				APIKey: "sk-openrouter",
				Models: map[string]config.ModelConfig{
					"openai/gpt-5.4-mini": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityFunctionCalling},
					},
				},
			},
			"together": {
				APIKey: "sk-together",
				Models: map[string]config.ModelConfig{
					"meta-llama/Llama-3.3-70B-Instruct-Turbo": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming},
					},
				},
			},
			"groq": {
				APIKey: "sk-groq",
				Models: map[string]config.ModelConfig{
					"llama-3.3-70b-versatile": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming},
					},
				},
			},
			"fireworks": {
				APIKey: "sk-fireworks",
				Models: map[string]config.ModelConfig{
					"accounts/fireworks/models/llama-v3p1-8b-instruct": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming},
					},
				},
			},
			"featherless": {
				APIKey: "sk-featherless",
				Models: map[string]config.ModelConfig{
					"meta-llama/Meta-Llama-3.1-8B-Instruct": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming},
					},
				},
			},
			"moonshot": {
				APIKey: "sk-moonshot",
				Models: map[string]config.ModelConfig{
					"kimi-k2-turbo-preview": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityFunctionCalling},
					},
				},
			},
			"glm": {
				APIKey: "sk-glm",
				Models: map[string]config.ModelConfig{
					"glm-5.1": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityFunctionCalling},
					},
				},
			},
			"mistral": {
				APIKey: "sk-mistral",
				Models: map[string]config.ModelConfig{
					"mistral-medium-latest": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityFunctionCalling},
					},
				},
			},
			"bedrock": {
				AccessKeyID:     "AKIAEXAMPLE",
				AccessKeySecret: "secret",
				Location:        "us-east-1",
				Models: map[string]config.ModelConfig{
					"amazon.nova-2-lite-v1:0": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityVision},
					},
				},
			},
			"nvidia": {
				APIKey: "sk-nvidia",
				Models: map[string]config.ModelConfig{
					"nvidia/Llama-3_3-Nemotron-Super-49B-v1_5": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityFunctionCalling},
					},
				},
			},
			"replicate": {
				APIKey: "r8_test",
				Models: map[string]config.ModelConfig{
					"minimax/video-01": {
						Modality:     modality.ModalityVideo,
						Capabilities: []modality.Capability{modality.CapabilityTextToVideo, modality.CapabilityImageToVideo},
					},
				},
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	for _, modelID := range []string{
		"openrouter/openai/gpt-5.4-mini",
		"together/meta-llama/Llama-3.3-70B-Instruct-Turbo",
		"groq/llama-3.3-70b-versatile",
		"fireworks/accounts/fireworks/models/llama-v3p1-8b-instruct",
		"featherless/meta-llama/Meta-Llama-3.1-8B-Instruct",
		"moonshot/kimi-k2-turbo-preview",
		"glm/glm-5.1",
		"mistral/mistral-medium-latest",
		"bedrock/amazon.nova-2-lite-v1:0",
		"nvidia/nvidia/Llama-3_3-Nemotron-Super-49B-v1_5",
	} {
		if _, _, err := registry.GetChatAdapter(modelID); err != nil {
			t.Fatalf("GetChatAdapter(%s) error = %v", modelID, err)
		}
	}
	if _, _, err := registry.GetVideoAdapter("replicate/minimax/video-01"); err != nil {
		t.Fatalf("GetVideoAdapter(replicate/minimax/video-01) error = %v", err)
	}
}

func TestRegistryAddsCatalogAliasesAndMetadata(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"moonshot": {
				APIKey: "sk-moonshot",
				Models: map[string]config.ModelConfig{
					"kimi-k2-turbo-preview": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming},
					},
				},
			},
			"glm": {
				APIKey: "sk-glm",
				Models: map[string]config.ModelConfig{
					"glm-5.1": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming},
					},
				},
			},
			"bedrock": {
				AccessKeyID:     "AKIAEXAMPLE",
				AccessKeySecret: "secret",
				Location:        "us-east-1",
				Models: map[string]config.ModelConfig{
					"amazon.nova-2-pro-preview-20251202-v1:0": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming},
					},
				},
			},
			"nvidia": {
				APIKey: "sk-nvidia",
				Models: map[string]config.ModelConfig{
					"NVIDIA-Nemotron-Nano-9B-v2": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming},
					},
				},
			},
			"replicate": {
				APIKey: "r8_test",
				Models: map[string]config.ModelConfig{
					"minimax/video-01": {
						Modality:     modality.ModalityVideo,
						Capabilities: []modality.Capability{modality.CapabilityTextToVideo, modality.CapabilityImageToVideo},
					},
				},
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	model, err := registry.ResolveModel("Kimi K2 Turbo Preview")
	if err != nil {
		t.Fatalf("ResolveModel(Kimi K2 Turbo Preview) error = %v", err)
	}
	if model.ID != "moonshot/kimi-k2-turbo-preview" {
		t.Fatalf("resolved model = %s", model.ID)
	}

	glmModel, err := registry.ResolveModel("GLM-5.1")
	if err != nil {
		t.Fatalf("ResolveModel(GLM-5.1) error = %v", err)
	}
	if glmModel.DisplayName != "GLM-5.1" {
		t.Fatalf("display name = %q", glmModel.DisplayName)
	}

	bedrockModel, err := registry.ResolveModel("Nova 2.0 Pro Preview")
	if err != nil {
		t.Fatalf("ResolveModel(Nova 2.0 Pro Preview) error = %v", err)
	}
	if bedrockModel.ID != "bedrock/amazon.nova-2-pro-preview-20251202-v1:0" {
		t.Fatalf("bedrock resolved model = %s", bedrockModel.ID)
	}

	nvidiaModel, err := registry.ResolveModel("NVIDIA Nemotron Nano 9B V2")
	if err != nil {
		t.Fatalf("ResolveModel(NVIDIA Nemotron Nano 9B V2) error = %v", err)
	}
	if nvidiaModel.ID != "nvidia/nvidia/NVIDIA-Nemotron-Nano-9B-v2" {
		t.Fatalf("nvidia resolved model = %s", nvidiaModel.ID)
	}

	replicateModel, err := registry.ResolveModel("Replicate MiniMax Video 01")
	if err != nil {
		t.Fatalf("ResolveModel(Replicate MiniMax Video 01) error = %v", err)
	}
	if replicateModel.ID != "replicate/minimax/video-01" {
		t.Fatalf("replicate resolved model = %s", replicateModel.ID)
	}

	models := registry.ListModels(true)
	foundGLM := false
	for _, item := range models {
		if item.ID == "GLM-5.1" {
			if item.ResolvesTo != "glm/glm-5.1" {
				t.Fatalf("alias resolves_to = %q", item.ResolvesTo)
			}
			if item.DisplayName != "GLM-5.1" {
				t.Fatalf("alias display name = %q", item.DisplayName)
			}
			foundGLM = true
		}
	}
	if !foundGLM {
		t.Fatal("expected GLM-5.1 alias in ListModels(true)")
	}
	for _, item := range models {
		if item.ID == "Replicate MiniMax Video 01" {
			if item.ResolvesTo != "replicate/minimax/video-01" {
				t.Fatalf("replicate alias resolves_to = %q", item.ResolvesTo)
			}
			return
		}
	}
	t.Fatal("expected catalog aliases in ListModels(true)")
}

func TestRegistrySelectorsResolveByCapabilitiesAndProviderPriority(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai": {
				APIKey: "sk-openai",
				Models: map[string]config.ModelConfig{
					"gpt-4o-mini": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityFunctionCalling},
					},
				},
			},
			"deepseek": {
				APIKey: "sk-deepseek",
				Models: map[string]config.ModelConfig{
					"deepseek-chat": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming},
					},
				},
			},
		},
		Routing: config.RoutingConfig{
			Selectors: map[string]config.RoutingSelector{
				"tooling-chat": {
					Modality:     modality.ModalityChat,
					Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityFunctionCalling},
					Providers:    []string{"deepseek", "openai"},
				},
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	model, err := registry.RequireModel("tooling-chat", modality.ModalityChat, modality.CapabilityStreaming, modality.CapabilityFunctionCalling)
	if err != nil {
		t.Fatalf("RequireModel(selector) error = %v", err)
	}
	if model.ID != "openai/gpt-4o-mini" {
		t.Fatalf("selector resolved to %q", model.ID)
	}
}

func TestRegistryListModelsIncludesSelectors(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai": {
				APIKey: "sk-openai",
				Models: map[string]config.ModelConfig{
					"gpt-4o-mini": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityFunctionCalling},
					},
				},
			},
		},
		Routing: config.RoutingConfig{
			Selectors: map[string]config.RoutingSelector{
				"reasoning-chat": {
					Modality:     modality.ModalityChat,
					Capabilities: []modality.Capability{modality.CapabilityStreaming},
					Providers:    []string{"openai"},
				},
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	models := registry.ListModels(true)
	for _, item := range models {
		if item.ID == "reasoning-chat" {
			if item.ResolvesTo != "openai/gpt-4o-mini" {
				t.Fatalf("selector resolves_to = %q", item.ResolvesTo)
			}
			if item.Provider != "openai" {
				t.Fatalf("selector provider = %q", item.Provider)
			}
			return
		}
	}
	t.Fatal("expected selector alias in ListModels(true)")
}

func TestRegistryResolvesModelFamiliesDeterministically(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai": {
				APIKey: "sk-openai",
				Models: map[string]config.ModelConfig{
					"gpt-5.4-mini": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityFunctionCalling},
					},
				},
			},
			"openrouter": {
				APIKey: "sk-openrouter",
				Models: map[string]config.ModelConfig{
					"openai/gpt-5.4-mini": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityFunctionCalling},
					},
				},
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	familyModel, err := registry.ResolveModel("gpt-5.4-mini")
	if err != nil {
		t.Fatalf("ResolveModel(family) error = %v", err)
	}
	if familyModel.ID != "openai/gpt-5.4-mini" {
		t.Fatalf("family resolved to %q", familyModel.ID)
	}

	exactModel, err := registry.ResolveModel("openrouter/openai/gpt-5.4-mini")
	if err != nil {
		t.Fatalf("ResolveModel(exact provider variant) error = %v", err)
	}
	if exactModel.ID != "openrouter/openai/gpt-5.4-mini" {
		t.Fatalf("exact provider model resolved to %q", exactModel.ID)
	}

	required, err := registry.RequireModel("gpt-5.4-mini", modality.ModalityChat, modality.CapabilityFunctionCalling)
	if err != nil {
		t.Fatalf("RequireModel(family) error = %v", err)
	}
	if required.ID != "openai/gpt-5.4-mini" {
		t.Fatalf("required family resolved to %q", required.ID)
	}
}

func TestRegistryListModelsIncludesFamilies(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai": {
				APIKey: "sk-openai",
				Models: map[string]config.ModelConfig{
					"gpt-5.4-mini": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityFunctionCalling},
					},
				},
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	models := registry.ListModels(true)
	foundFamily := false
	for _, item := range models {
		if item.ID != "gpt-5.4-mini" {
			continue
		}
		if item.Kind != "family" {
			t.Fatalf("family kind = %q", item.Kind)
		}
		if item.ProviderVariant != "openai/gpt-5.4-mini" {
			t.Fatalf("family provider_variant = %q", item.ProviderVariant)
		}
		if item.ResolvesTo != "openai/gpt-5.4-mini" {
			t.Fatalf("family resolves_to = %q", item.ResolvesTo)
		}
		if item.FamilyDisplayName != "GPT-5.4 mini" {
			t.Fatalf("family display name = %q", item.FamilyDisplayName)
		}
		foundFamily = true
	}
	if !foundFamily {
		t.Fatal("expected family entry in ListModels(true)")
	}
}

func TestRegistryRequireResolvedModelAppliesRequestRouting(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai": {
				APIKey: "sk-openai",
				Models: map[string]config.ModelConfig{
					"gpt-5.4-mini": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityFunctionCalling},
					},
				},
			},
			"openrouter": {
				APIKey: "sk-openrouter",
				Models: map[string]config.ModelConfig{
					"openai/gpt-5.4-mini": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityFunctionCalling},
					},
				},
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	resolution, err := registry.RequireResolvedModel("gpt-5.4-mini", modality.ModalityChat, &modality.RoutingOptions{
		Providers: []string{"openrouter"},
	})
	if err != nil {
		t.Fatalf("RequireResolvedModel(family+routing) error = %v", err)
	}
	if resolution.Model.ID != "openrouter/openai/gpt-5.4-mini" {
		t.Fatalf("resolved model = %q", resolution.Model.ID)
	}
	if resolution.Mode != "request_hint" {
		t.Fatalf("resolution mode = %q", resolution.Mode)
	}
}

func TestRegistryRequireResolvedModelExactBypassesRequestRouting(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai": {
				APIKey: "sk-openai",
				Models: map[string]config.ModelConfig{
					"gpt-5.4-mini": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityFunctionCalling},
					},
				},
			},
			"openrouter": {
				APIKey: "sk-openrouter",
				Models: map[string]config.ModelConfig{
					"openai/gpt-5.4-mini": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityFunctionCalling},
					},
				},
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	resolution, err := registry.RequireResolvedModel("openai/gpt-5.4-mini", modality.ModalityChat, &modality.RoutingOptions{
		ExcludeProviders: []string{"openai"},
	})
	if err != nil {
		t.Fatalf("RequireResolvedModel(exact+routing) error = %v", err)
	}
	if resolution.Model.ID != "openai/gpt-5.4-mini" {
		t.Fatalf("resolved model = %q", resolution.Model.ID)
	}
	if resolution.Mode != "exact" {
		t.Fatalf("resolution mode = %q", resolution.Mode)
	}
}

func TestRegistryRegistersPhase2CProviders(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"google": {
				APIKey:  "google-key",
				BaseURL: "https://generativelanguage.googleapis.com",
				Timeout: time.Minute,
				Models: map[string]config.ModelConfig{
					"gemini-2.5-flash": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityVision, modality.CapabilityFunctionCalling, modality.CapabilityStreaming},
					},
				},
			},
			"qwen": {
				APIKey:  "qwen-key",
				BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
				Timeout: time.Minute,
				Models: map[string]config.ModelConfig{
					"qwen-max": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityVision, modality.CapabilityFunctionCalling, modality.CapabilityStreaming},
					},
				},
			},
		},
		Routing: config.RoutingConfig{
			Aliases: map[string]string{
				"google-chat": "google/gemini-2.5-flash",
				"qwen-chat":   "qwen/qwen-max",
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if registry.Count() != 2 {
		t.Fatalf("expected 2 models, got %d", registry.Count())
	}
	if _, _, err := registry.GetChatAdapter("google-chat"); err != nil {
		t.Fatalf("GetChatAdapter(google alias) error = %v", err)
	}
	if _, _, err := registry.GetChatAdapter("qwen-chat"); err != nil {
		t.Fatalf("GetChatAdapter(qwen alias) error = %v", err)
	}
}

func TestRegistryRegistersPhase2DProvider(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"bytedance": {
				APIKey:  "ark-key",
				BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
				Timeout: time.Minute,
				Models: map[string]config.ModelConfig{
					"doubao-pro-256k": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityFunctionCalling, modality.CapabilityStreaming},
						Endpoint:     "https://ark.cn-beijing.volces.com/api/v3",
					},
				},
			},
		},
		Routing: config.RoutingConfig{
			Aliases: map[string]string{
				"doubao-chat": "bytedance/doubao-pro-256k",
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if registry.Count() != 1 {
		t.Fatalf("expected 1 model, got %d", registry.Count())
	}
	if _, _, err := registry.GetChatAdapter("doubao-chat"); err != nil {
		t.Fatalf("GetChatAdapter(bytedance alias) error = %v", err)
	}
}

func TestRegistryRegistersCurrentByteDanceModelsAndAliases(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"bytedance": {
				APIKey:  "ark-key",
				BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
				Timeout: time.Minute,
				Models: map[string]config.ModelConfig{
					"doubao-seed-2.0-pro": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityVision, modality.CapabilityFunctionCalling, modality.CapabilityStreaming},
						Endpoint:     "https://ark.cn-beijing.volces.com/api/v3",
					},
					"doubao-seed-1.6-vision": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityVision, modality.CapabilityStreaming},
						Endpoint:     "https://ark.cn-beijing.volces.com/api/v3",
					},
					"doubao-seedream-5.0-lite": {
						Modality:      modality.ModalityImage,
						Capabilities:  []modality.Capability{modality.CapabilityGeneration},
						OutputFormats: []string{"png", "jpeg"},
						Endpoint:      "https://ark.cn-beijing.volces.com/api/v3",
					},
					"doubao-seedance-2.0": {
						Modality:         modality.ModalityVideo,
						Capabilities:     []modality.Capability{modality.CapabilityTextToVideo, modality.CapabilityImageToVideo, modality.CapabilityLastFrame, modality.CapabilityReferenceImages, modality.CapabilityVideoInput, modality.CapabilityAudioInput, modality.CapabilityNativeAudio},
						AllowedDurations: []int{4, 8, 12, 15},
						Resolutions:      []string{"720p"},
						Cancelable:       true,
						Endpoint:         "https://ark.cn-beijing.volces.com/api/v3",
					},
				},
			},
		},
		Routing: config.RoutingConfig{
			Aliases: map[string]string{
				"bytedance-chat":   "bytedance/doubao-seed-2.0-pro",
				"bytedance-vision": "bytedance/doubao-seed-1.6-vision",
				"bytedance-image":  "bytedance/doubao-seedream-5.0-lite",
				"bytedance-video":  "bytedance/doubao-seedance-2.0",
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	if _, _, err := registry.GetChatAdapter("bytedance-chat"); err != nil {
		t.Fatalf("GetChatAdapter(bytedance-chat) error = %v", err)
	}
	if _, _, err := registry.GetChatAdapter("bytedance-vision"); err != nil {
		t.Fatalf("GetChatAdapter(bytedance-vision) error = %v", err)
	}
	if _, _, err := registry.GetImageAdapter("bytedance-image"); err != nil {
		t.Fatalf("GetImageAdapter(bytedance-image) error = %v", err)
	}
	if _, _, err := registry.GetVideoAdapter("bytedance-video"); err != nil {
		t.Fatalf("GetVideoAdapter(bytedance-video) error = %v", err)
	}
}

func TestRegistryRegistersPhase3MetadataWithoutAdapters(t *testing.T) {
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
					"text-embedding-3-small": {
						Modality:   modality.ModalityEmbed,
						Dimensions: 1536,
					},
					"gpt-image-1": {
						Modality:      modality.ModalityImage,
						Capabilities:  []modality.Capability{modality.CapabilityGeneration, modality.CapabilityEditing},
						OutputFormats: []string{"png"},
					},
					"tts-1": {
						Modality:     modality.ModalityVoice,
						Capabilities: []modality.Capability{modality.CapabilityTTS},
						Voices:       []string{"nova"},
					},
				},
			},
			"google": {
				APIKey:  "google-key",
				BaseURL: "https://generativelanguage.googleapis.com",
				Timeout: time.Minute,
				Models: map[string]config.ModelConfig{
					"gemini-embedding-001": {
						Modality:   modality.ModalityEmbed,
						Dimensions: 768,
					},
				},
			},
			"bytedance": {
				APIKey:  "ark-key",
				BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
				Timeout: time.Minute,
				Models: map[string]config.ModelConfig{
					"seedance-2.0": {
						Modality:         modality.ModalityVideo,
						Capabilities:     []modality.Capability{modality.CapabilityTextToVideo, modality.CapabilityImageToVideo, modality.CapabilityLastFrame, modality.CapabilityReferenceImages, modality.CapabilityVideoInput, modality.CapabilityAudioInput, modality.CapabilityNativeAudio},
						MaxDuration:      15,
						AllowedDurations: []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
						Resolutions:      []string{"720p"},
						Cancelable:       true,
						Endpoint:         "https://ark.cn-beijing.volces.com/api/v3",
					},
				},
			},
		},
		Routing: config.RoutingConfig{
			Aliases: map[string]string{
				"default-embed": "openai/text-embedding-3-small",
				"google-embed":  "google/gemini-embedding-001",
				"default-image": "openai/gpt-image-1",
				"default-tts":   "openai/tts-1",
				"default-video": "bytedance/seedance-2.0",
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if registry.Count() != 6 {
		t.Fatalf("expected 6 registered runtime models, got %d", registry.Count())
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	imageModel, err := registry.RequireModel("default-image", modality.ModalityImage, modality.CapabilityGeneration)
	if err != nil {
		t.Fatalf("RequireModel(default-image) error = %v", err)
	}
	if imageModel.ID != "openai/gpt-image-1" {
		t.Fatalf("unexpected resolved image model %q", imageModel.ID)
	}

	if _, _, err := registry.GetEmbedAdapter("default-embed"); err != nil {
		t.Fatalf("expected OpenAI embed adapter, got %v", err)
	}
	if _, _, err := registry.GetEmbedAdapter("google-embed"); err != nil {
		t.Fatalf("expected Google embed adapter, got %v", err)
	}
	if _, _, err := registry.GetImageAdapter("default-image"); err != nil {
		t.Fatalf("expected OpenAI image adapter, got %v", err)
	}
	if _, _, err := registry.GetVoiceAdapter("default-tts"); err != nil {
		t.Fatalf("expected OpenAI voice adapter, got %v", err)
	}
	if _, _, err := registry.GetVideoAdapter("default-video"); err != nil {
		t.Fatalf("expected ByteDance video adapter, got %v", err)
	}
	if _, err := registry.RequireModel("default-image", modality.ModalityImage, modality.CapabilityMultiReference); !errors.Is(err, ErrCapabilityMissing) {
		t.Fatalf("expected ErrCapabilityMissing, got %v", err)
	}
}

func TestRegistryRegistersPhase3EImageProviders(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"bytedance": {
				APIKey:  "ark-key",
				BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
				Timeout: time.Minute,
				Models: map[string]config.ModelConfig{
					"seedream-4.5": {
						Modality:      modality.ModalityImage,
						Capabilities:  []modality.Capability{modality.CapabilityGeneration, modality.CapabilityEditing, modality.CapabilityMultiReference},
						OutputFormats: []string{"png", "jpeg"},
						Endpoint:      "https://ark.cn-beijing.volces.com/api/v3",
					},
				},
			},
			"qwen": {
				APIKey:  "qwen-key",
				BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
				Timeout: time.Minute,
				Models: map[string]config.ModelConfig{
					"qwen-image-2.0": {
						Modality:      modality.ModalityImage,
						Capabilities:  []modality.Capability{modality.CapabilityGeneration, modality.CapabilityEditing},
						OutputFormats: []string{"png"},
					},
				},
			},
		},
		Routing: config.RoutingConfig{
			Aliases: map[string]string{
				"seedream-image": "bytedance/seedream-4.5",
				"qwen-image":     "qwen/qwen-image-2.0",
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if _, _, err := registry.GetImageAdapter("seedream-image"); err != nil {
		t.Fatalf("GetImageAdapter(seedream alias) error = %v", err)
	}
	if _, _, err := registry.GetImageAdapter("qwen-image"); err != nil {
		t.Fatalf("GetImageAdapter(qwen alias) error = %v", err)
	}
}

func TestRegistryRegistersByteDanceVoicePerCapabilityCredentials(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"bytedance": {
				APIKey:       "ark-key",
				SpeechAPIKey: "speech-key",
				BaseURL:      "https://ark.cn-beijing.volces.com/api/v3",
				Timeout:      time.Minute,
				Models: map[string]config.ModelConfig{
					"doubao-tts-2.0": {
						Modality:     modality.ModalityVoice,
						Capabilities: []modality.Capability{modality.CapabilityTTS},
						Voices:       []string{"zh_female_vv_uranus_bigtts"},
						Endpoint:     "https://openspeech.bytedance.com/api/v3/tts/unidirectional/sse",
					},
					"doubao-asr-2.0": {
						Modality:     modality.ModalityVoice,
						Capabilities: []modality.Capability{modality.CapabilitySTT},
						Formats:      []string{"wav"},
						Endpoint:     "https://openspeech.bytedance.com/api/v3/auc/bigmodel/recognize/flash",
					},
				},
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if registry.Count() != 2 {
		t.Fatalf("expected both ByteDance TTS/STT models to register, got %d", registry.Count())
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if _, _, err := registry.GetVoiceAdapter("bytedance/doubao-asr-2.0"); err != nil {
		t.Fatalf("expected ByteDance STT adapter, got %v", err)
	}
	if _, _, err := registry.GetVoiceAdapter("bytedance/doubao-tts-2.0"); err != nil {
		t.Fatalf("expected ByteDance TTS adapter, got %v", err)
	}
}

func TestRegistryRegistersByteDanceStreamingTranscriptionModel(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"bytedance": {
				AppID:             "app-123",
				SpeechAccessToken: "speech-token",
				SpeechAPIKey:      "speech-key",
				BaseURL:           "https://ark.cn-beijing.volces.com/api/v3",
				Timeout:           time.Minute,
				Models: map[string]config.ModelConfig{
					"doubao-streaming-asr-2.0": {
						Modality:     modality.ModalityVoice,
						Capabilities: []modality.Capability{modality.CapabilityStreaming},
						Endpoint:     "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async",
					},
				},
			},
		},
		Routing: config.RoutingConfig{
			Aliases: map[string]string{
				"bytedance-streaming-asr": "bytedance/doubao-streaming-asr-2.0",
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if _, _, err := registry.GetStreamingTranscriptionAdapter("bytedance-streaming-asr"); err != nil {
		t.Fatalf("expected ByteDance streaming transcription adapter, got %v", err)
	}
}

func TestRegistryRegistersByteDanceVideoModelsWhenAPIKeyPresent(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"bytedance": {
				APIKey:  "ark-key",
				BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
				Timeout: time.Minute,
				Models: map[string]config.ModelConfig{
					"seedance-2.0": {
						Modality:         modality.ModalityVideo,
						Capabilities:     []modality.Capability{modality.CapabilityTextToVideo, modality.CapabilityVideoInput, modality.CapabilityAudioInput},
						MaxDuration:      15,
						AllowedDurations: []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
						Resolutions:      []string{"720p"},
						Cancelable:       true,
						Endpoint:         "https://ark.cn-beijing.volces.com/api/v3",
					},
				},
			},
		},
		Routing: config.RoutingConfig{
			Aliases: map[string]string{
				"default-video": "bytedance/seedance-2.0",
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	videoModel, err := registry.RequireModel("default-video", modality.ModalityVideo, modality.CapabilityTextToVideo)
	if err != nil {
		t.Fatalf("RequireModel(default-video) error = %v", err)
	}
	if videoModel.MaxDuration != 15 || len(videoModel.Resolutions) != 1 {
		t.Fatalf("expected video metadata to survive registry build, got %#v", videoModel)
	}
	if _, _, err := registry.GetVideoAdapter("default-video"); err != nil {
		t.Fatalf("expected video adapter, got %v", err)
	}
}

func TestRegistryRegistersPhase4CVideoProviders(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai": {
				APIKey:  "sk-openai",
				BaseURL: "https://api.openai.com/v1",
				Timeout: time.Minute,
				Models: map[string]config.ModelConfig{
					"sora-2": {
						Modality:         modality.ModalityVideo,
						Capabilities:     []modality.Capability{modality.CapabilityTextToVideo, modality.CapabilityImageToVideo, modality.CapabilityNativeAudio},
						AllowedDurations: []int{4, 8, 12},
						AspectRatios:     []string{"16:9", "9:16"},
						Resolutions:      []string{"720p"},
						Cancelable:       false,
					},
				},
			},
			"google-vertex": {
				ProjectID: "test-project",
				Location:  "us-central1",
				SecretKey: "vertex-job-secret",
				Timeout:   time.Minute,
				Models: map[string]config.ModelConfig{
					"veo-3.1-generate-001": {
						Modality:         modality.ModalityVideo,
						Capabilities:     []modality.Capability{modality.CapabilityTextToVideo, modality.CapabilityImageToVideo, modality.CapabilityLastFrame, modality.CapabilityNativeAudio},
						AllowedDurations: []int{4, 6, 8},
						AspectRatios:     []string{"16:9", "9:16"},
						Resolutions:      []string{"720p", "1080p"},
						Cancelable:       true,
					},
				},
			},
		},
		Routing: config.RoutingConfig{
			Aliases: map[string]string{
				"sora-video": "openai/sora-2",
				"veo-video":  "google-vertex/veo-3.1-generate-001",
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	sora, err := registry.RequireModel("sora-video", modality.ModalityVideo, modality.CapabilityTextToVideo)
	if err != nil {
		t.Fatalf("RequireModel(sora-video) error = %v", err)
	}
	if sora.Cancelable || len(sora.AllowedDurations) != 3 || len(sora.AspectRatios) != 2 {
		t.Fatalf("unexpected Sora metadata %#v", sora)
	}

	veo, err := registry.RequireModel("veo-video", modality.ModalityVideo, modality.CapabilityLastFrame)
	if err != nil {
		t.Fatalf("RequireModel(veo-video) error = %v", err)
	}
	if !veo.Cancelable || len(veo.Resolutions) != 2 {
		t.Fatalf("unexpected Veo metadata %#v", veo)
	}

	if _, _, err := registry.GetVideoAdapter("sora-video"); err != nil {
		t.Fatalf("expected Sora adapter, got %v", err)
	}
	if _, _, err := registry.GetVideoAdapter("veo-video"); err != nil {
		t.Fatalf("expected Veo adapter, got %v", err)
	}
}

func TestRegistryRegistersAudioModelsWithPipeline(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai": {
				APIKey:  "sk-openai",
				BaseURL: "https://api.openai.com/v1",
				Timeout: time.Minute,
				Models: map[string]config.ModelConfig{
					"realtime-voice": {
						Modality:     modality.ModalityAudio,
						Capabilities: []modality.Capability{modality.CapabilityAudioInput, modality.CapabilityAudioOutput},
						AudioPipeline: config.AudioPipelineConfig{
							ChatModel: "gpt-4o-mini",
							STTModel:  "whisper-1",
							TTSModel:  "tts-1",
						},
						SessionTTL: 10 * time.Minute,
					},
					"gpt-4o-mini": {Modality: modality.ModalityChat},
					"whisper-1":   {Modality: modality.ModalityVoice, Capabilities: []modality.Capability{modality.CapabilitySTT}},
					"tts-1":       {Modality: modality.ModalityVoice, Capabilities: []modality.Capability{modality.CapabilityTTS}},
				},
			},
		},
		Routing: config.RoutingConfig{
			Aliases: map[string]string{
				"default-audio": "openai/realtime-voice",
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if registry.Count() != 4 {
		t.Fatalf("expected runtime models, got %d", registry.Count())
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	audioModel, err := registry.RequireModel("default-audio", modality.ModalityAudio, modality.CapabilityAudioInput, modality.CapabilityAudioOutput)
	if err != nil {
		t.Fatalf("expected audio alias to resolve, got %v", err)
	}
	if audioModel.SessionTTL != int64((10 * time.Minute).Seconds()) {
		t.Fatalf("unexpected session ttl %#v", audioModel)
	}
	if _, _, err := registry.GetAudioAdapter("default-audio"); err != nil {
		t.Fatalf("expected audio adapter, got %v", err)
	}
}

func TestRegistryRegistersByteDanceRealtimeAudioModel(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"bytedance": {
				AppID:             "app-123",
				SpeechAccessToken: "speech-access-token",
				SpeechAPIKey:      "speech-access-key",
				BaseURL:           "https://ark.cn-beijing.volces.com/api/v3",
				Timeout:           time.Minute,
				Models: map[string]config.ModelConfig{
					"doubao-audio": {
						Modality:     modality.ModalityAudio,
						Capabilities: []modality.Capability{modality.CapabilityAudioInput, modality.CapabilityAudioOutput},
						Voices:       []string{"zh_female_vv_jupiter_bigtts"},
						SessionTTL:   10 * time.Minute,
						RealtimeSession: config.AudioRealtimeConfig{
							Transport: "bytedance_dialog",
							Auth:      "access_token",
							Model:     "1.2.1.1",
						},
					},
				},
			},
		},
		Routing: config.RoutingConfig{
			Aliases: map[string]string{
				"bytedance-audio": "bytedance/doubao-audio",
			},
		},
	}

	registry, warnings, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	audioModel, err := registry.RequireModel("bytedance-audio", modality.ModalityAudio, modality.CapabilityAudioInput, modality.CapabilityAudioOutput)
	if err != nil {
		t.Fatalf("expected audio alias to resolve, got %v", err)
	}
	if audioModel.SessionTTL != int64((10 * time.Minute).Seconds()) {
		t.Fatalf("unexpected session ttl %#v", audioModel)
	}
	if _, _, err := registry.GetAudioAdapter("bytedance-audio"); err != nil {
		t.Fatalf("expected ByteDance audio adapter, got %v", err)
	}
}
