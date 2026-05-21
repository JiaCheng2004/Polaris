package provider

import (
	"slices"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/modality"
)

type ModelRoutingMetadata struct {
	Aliases   []ModelRoutingAlias             `json:"aliases,omitempty"`
	Selectors []ModelRoutingSelector          `json:"selectors,omitempty"`
	Defaults  map[string]ModelExecutorDefault `json:"defaults,omitempty"`
}

type ModelRoutingAlias struct {
	Alias      string `json:"alias"`
	Target     string `json:"target"`
	ResolvesTo string `json:"resolves_to"`
	Kind       string `json:"kind"`
}

type ModelRoutingSelector struct {
	Alias               string                `json:"alias"`
	ResolvesTo          string                `json:"resolves_to,omitempty"`
	Modality            modality.Modality     `json:"modality,omitempty"`
	Capabilities        []modality.Capability `json:"capabilities,omitempty"`
	Providers           []string              `json:"providers,omitempty"`
	ExcludeProviders    []string              `json:"exclude_providers,omitempty"`
	Statuses            []string              `json:"statuses,omitempty"`
	VerificationClasses []string              `json:"verification_classes,omitempty"`
	Prefer              []string              `json:"prefer,omitempty"`
	CostTier            string                `json:"cost_tier,omitempty"`
	LatencyTier         string                `json:"latency_tier,omitempty"`
}

type ModelExecutorDefault struct {
	Alias        string                `json:"alias"`
	Model        string                `json:"model"`
	Provider     string                `json:"provider,omitempty"`
	Modality     modality.Modality     `json:"modality,omitempty"`
	Capabilities []modality.Capability `json:"capabilities,omitempty"`
}

func (r *Registry) RoutingMetadata() ModelRoutingMetadata {
	if r == nil {
		return ModelRoutingMetadata{}
	}

	metadata := ModelRoutingMetadata{
		Aliases:   r.routingAliases(),
		Selectors: r.routingSelectors(),
		Defaults:  r.routingDefaults(),
	}
	if len(metadata.Defaults) == 0 {
		metadata.Defaults = nil
	}
	return metadata
}

func (r *Registry) routingAliases() []ModelRoutingAlias {
	items := make([]ModelRoutingAlias, 0, len(r.configuredAliases))
	for alias, target := range r.configuredAliases {
		items = append(items, ModelRoutingAlias{
			Alias:      alias,
			Target:     target,
			ResolvesTo: target,
			Kind:       "alias",
		})
	}
	slices.SortFunc(items, func(a, b ModelRoutingAlias) int {
		return strings.Compare(a.Alias, b.Alias)
	})
	return items
}

func (r *Registry) routingSelectors() []ModelRoutingSelector {
	items := make([]ModelRoutingSelector, 0, len(r.selectors))
	for alias, selector := range r.selectors {
		resolved, err := selectModelForSelector(r, alias, selector, selector.Modality, selector.Capabilities, nil)
		resolvesTo := ""
		if err == nil {
			resolvesTo = resolved.ID
		}
		items = append(items, ModelRoutingSelector{
			Alias:               alias,
			ResolvesTo:          resolvesTo,
			Modality:            selector.Modality,
			Capabilities:        append([]modality.Capability(nil), selector.Capabilities...),
			Providers:           append([]string(nil), selector.Providers...),
			ExcludeProviders:    append([]string(nil), selector.ExcludeProviders...),
			Statuses:            append([]string(nil), selector.Statuses...),
			VerificationClasses: append([]string(nil), selector.VerificationClasses...),
			Prefer:              append([]string(nil), selector.Prefer...),
			CostTier:            selector.CostTier,
			LatencyTier:         selector.LatencyTier,
		})
	}
	slices.SortFunc(items, func(a, b ModelRoutingSelector) int {
		return strings.Compare(a.Alias, b.Alias)
	})
	return items
}

func (r *Registry) routingDefaults() map[string]ModelExecutorDefault {
	defaults := make(map[string]ModelExecutorDefault)
	for _, candidate := range []struct {
		capability string
		alias      string
	}{
		{capability: "chat", alias: "default-chat"},
		{capability: "image_generation", alias: "default-image"},
		{capability: "video_generation", alias: "default-video"},
		{capability: "music_generation", alias: "default-music"},
		{capability: "tts", alias: "default-voice-tts"},
		{capability: "stt", alias: "default-voice-stt"},
		{capability: "streaming_transcription", alias: "default-voice-stream"},
		{capability: "audio", alias: "default-audio"},
		{capability: "embedding", alias: "default-embed"},
	} {
		if item, ok := r.executorDefault(candidate.alias); ok {
			defaults[candidate.capability] = item
			addSecondaryDefaults(defaults, item)
		}
	}
	return defaults
}

func (r *Registry) executorDefault(alias string) (ModelExecutorDefault, bool) {
	if _, ok := r.configuredAliases[alias]; !ok {
		if _, ok := r.selectors[alias]; !ok {
			return ModelExecutorDefault{}, false
		}
	}
	resolution, err := r.RequireResolvedModel(alias, "", nil)
	if err != nil {
		return ModelExecutorDefault{}, false
	}
	model := withModelMetadata(resolution.Model)
	return ModelExecutorDefault{
		Alias:        alias,
		Model:        resolution.ResolvedModel,
		Provider:     resolution.ResolvedProvider,
		Modality:     model.Modality,
		Capabilities: append([]modality.Capability(nil), model.Capabilities...),
	}, true
}

func addSecondaryDefaults(defaults map[string]ModelExecutorDefault, item ModelExecutorDefault) {
	model := withModelMetadata(Model{
		Modality:     item.Modality,
		Capabilities: item.Capabilities,
	})
	switch {
	case item.Alias == "default-image" && model.CapabilityFlags.ImageEdit:
		defaults["image_edit"] = item
	case item.Alias == "default-music" && model.CapabilityFlags.LyricsGeneration:
		defaults["lyrics_generation"] = item
	}
}
