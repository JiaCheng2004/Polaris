package provider

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/anthropic"
	"github.com/JiaCheng2004/Polaris/internal/provider/openai"
)

var (
	ErrUnknownAlias      = errors.New("unknown alias")
	ErrUnknownModel      = errors.New("unknown model")
	ErrModalityMismatch  = errors.New("modality mismatch")
	ErrCapabilityMissing = errors.New("capability missing")
	ErrAdapterMissing    = errors.New("adapter missing")
)

type Registry struct {
	models       map[string]Model
	chatAdapters map[string]modality.ChatAdapter
	aliases      map[string]string
	fallbacks    map[string][]string
}

type Model struct {
	ID              string                `json:"id"`
	Object          string                `json:"object"`
	Provider        string                `json:"provider,omitempty"`
	Name            string                `json:"-"`
	Modality        modality.Modality     `json:"modality"`
	Capabilities    []modality.Capability `json:"capabilities,omitempty"`
	ContextWindow   int                   `json:"context_window,omitempty"`
	MaxOutputTokens int                   `json:"max_output_tokens,omitempty"`
	MaxDuration     int                   `json:"max_duration,omitempty"`
	Resolutions     []string              `json:"resolutions,omitempty"`
	Voices          []string              `json:"voices,omitempty"`
	Formats         []string              `json:"formats,omitempty"`
	OutputFormats   []string              `json:"output_formats,omitempty"`
	Dimensions      int                   `json:"dimensions,omitempty"`
	ResolvesTo      string                `json:"resolves_to,omitempty"`
}

func New(cfg *config.Config) (*Registry, []string, error) {
	registry := &Registry{
		models:       map[string]Model{},
		chatAdapters: map[string]modality.ChatAdapter{},
		aliases:      map[string]string{},
		fallbacks:    map[string][]string{},
	}

	var warnings []string

	for providerName, providerCfg := range cfg.Providers {
		if !providerEnabled(providerName, providerCfg) {
			warnings = append(warnings, fmt.Sprintf("provider %s is disabled because required credentials are missing", providerName))
			continue
		}

		supported := false
		switch providerName {
		case "openai":
			supported = true
			client := openai.NewClient(providerCfg)
			for modelName, modelCfg := range providerCfg.Models {
				if modelCfg.Modality != modality.ModalityChat {
					continue
				}
				id := fmt.Sprintf("%s/%s", providerName, modelName)
				registry.models[id] = modelFromConfig(id, providerName, modelName, modelCfg)
				registry.chatAdapters[id] = openai.NewChatAdapter(client, id)
			}
		case "anthropic":
			supported = true
			client := anthropic.NewClient(providerCfg)
			for modelName, modelCfg := range providerCfg.Models {
				if modelCfg.Modality != modality.ModalityChat {
					continue
				}
				id := fmt.Sprintf("%s/%s", providerName, modelName)
				registry.models[id] = modelFromConfig(id, providerName, modelName, modelCfg)
				registry.chatAdapters[id] = anthropic.NewChatAdapter(client, id, modelCfg.MaxOutputTokens)
			}
		}
		if supported {
			continue
		}

		warnings = append(warnings, fmt.Sprintf("provider %s is configured but not supported by this runtime build", providerName))
	}

	for alias, target := range cfg.Routing.Aliases {
		if _, ok := registry.models[target]; !ok {
			warnings = append(warnings, fmt.Sprintf("alias %s points to unavailable model %s", alias, target))
			continue
		}
		registry.aliases[alias] = target
	}

	for _, rule := range cfg.Routing.Fallbacks {
		if _, ok := registry.models[rule.From]; !ok {
			continue
		}
		for _, target := range rule.To {
			if _, ok := registry.models[target]; ok {
				registry.fallbacks[rule.From] = append(registry.fallbacks[rule.From], target)
			}
		}
	}

	return registry, warnings, nil
}

func modelFromConfig(id, providerName, modelName string, modelCfg config.ModelConfig) Model {
	return Model{
		ID:              id,
		Object:          "model",
		Provider:        providerName,
		Name:            modelName,
		Modality:        modelCfg.Modality,
		Capabilities:    append([]modality.Capability(nil), modelCfg.Capabilities...),
		ContextWindow:   modelCfg.ContextWindow,
		MaxOutputTokens: modelCfg.MaxOutputTokens,
		MaxDuration:     modelCfg.MaxDuration,
		Resolutions:     append([]string(nil), modelCfg.Resolutions...),
		Voices:          append([]string(nil), modelCfg.Voices...),
		Formats:         append([]string(nil), modelCfg.Formats...),
		OutputFormats:   append([]string(nil), modelCfg.OutputFormats...),
		Dimensions:      modelCfg.Dimensions,
	}
}

func (r *Registry) Count() int {
	return len(r.models)
}

func (r *Registry) ProviderCount() int {
	providers := map[string]struct{}{}
	for _, model := range r.models {
		if model.Provider == "" {
			continue
		}
		providers[model.Provider] = struct{}{}
	}
	return len(providers)
}

func (r *Registry) ResolveAlias(alias string) (string, error) {
	target, ok := r.aliases[alias]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUnknownAlias, alias)
	}
	return target, nil
}

func (r *Registry) ResolveModel(name string) (Model, error) {
	if target, ok := r.aliases[name]; ok {
		return r.models[target], nil
	}
	model, ok := r.models[name]
	if !ok {
		if !strings.Contains(name, "/") {
			return Model{}, fmt.Errorf("%w: %s", ErrUnknownAlias, name)
		}
		return Model{}, fmt.Errorf("%w: %s", ErrUnknownModel, name)
	}
	return model, nil
}

func (r *Registry) GetChatAdapter(name string) (modality.ChatAdapter, Model, error) {
	model, err := r.RequireModel(name, modality.ModalityChat)
	if err != nil {
		return nil, Model{}, err
	}
	adapter, ok := r.chatAdapters[model.ID]
	if !ok {
		return nil, Model{}, fmt.Errorf("%w: %s", ErrAdapterMissing, model.ID)
	}
	return adapter, model, nil
}

func (r *Registry) RequireModel(name string, requiredModality modality.Modality, requiredCapabilities ...modality.Capability) (Model, error) {
	model, err := r.ResolveModel(name)
	if err != nil {
		return Model{}, err
	}
	if requiredModality != "" && model.Modality != requiredModality {
		return Model{}, fmt.Errorf("%w: requested %s got %s", ErrModalityMismatch, requiredModality, model.Modality)
	}
	for _, capability := range requiredCapabilities {
		if !containsCapability(model.Capabilities, capability) {
			return Model{}, fmt.Errorf("%w: %s", ErrCapabilityMissing, capability)
		}
	}
	return model, nil
}

func (r *Registry) ListModels(includeAliases bool) []Model {
	var items []Model
	for _, model := range r.models {
		items = append(items, model)
	}
	slices.SortFunc(items, func(a, b Model) int {
		return strings.Compare(a.ID, b.ID)
	})

	if !includeAliases {
		return items
	}

	for alias, target := range r.aliases {
		resolved := r.models[target]
		items = append(items, Model{
			ID:              alias,
			Object:          "model",
			Provider:        resolved.Provider,
			Modality:        resolved.Modality,
			Capabilities:    append([]modality.Capability(nil), resolved.Capabilities...),
			ContextWindow:   resolved.ContextWindow,
			MaxOutputTokens: resolved.MaxOutputTokens,
			MaxDuration:     resolved.MaxDuration,
			Resolutions:     append([]string(nil), resolved.Resolutions...),
			Voices:          append([]string(nil), resolved.Voices...),
			Formats:         append([]string(nil), resolved.Formats...),
			OutputFormats:   append([]string(nil), resolved.OutputFormats...),
			Dimensions:      resolved.Dimensions,
			ResolvesTo:      target,
		})
	}

	slices.SortFunc(items, func(a, b Model) int {
		return strings.Compare(a.ID, b.ID)
	})
	return items
}

func (r *Registry) GetFallbacks(modelID string) []string {
	return append([]string(nil), r.fallbacks[modelID]...)
}

func providerEnabled(name string, providerCfg config.ProviderConfig) bool {
	switch name {
	case "ollama":
		return true
	case "bytedance":
		return providerCfg.AccessKey != "" && providerCfg.SecretKey != ""
	default:
		return providerCfg.APIKey != ""
	}
}

func containsCapability(capabilities []modality.Capability, candidate modality.Capability) bool {
	for _, capability := range capabilities {
		if capability == candidate {
			return true
		}
	}
	return false
}
