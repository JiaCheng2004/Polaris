package provider

import (
	"fmt"
	"slices"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func (r *Registry) ResolveAlias(alias string) (string, error) {
	target, ok := r.aliases[alias]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUnknownAlias, alias)
	}
	return target, nil
}

func (r *Registry) ResolveModel(name string) (Model, error) {
	model, ok := r.models[name]
	if ok {
		return model, nil
	}
	if target, ok := r.aliases[name]; ok {
		return r.models[target], nil
	}
	if familyID, exists := r.familyAliases[name]; exists {
		return r.resolveFamilyModel(familyID, "", nil, nil)
	}
	if _, exists := r.families[name]; exists {
		return r.resolveFamilyModel(name, "", nil, nil)
	}
	if !strings.Contains(name, "/") {
		return Model{}, fmt.Errorf("%w: %s", ErrUnknownAlias, name)
	}
	return Model{}, fmt.Errorf("%w: %s", ErrUnknownModel, name)
}

func (r *Registry) RequireModel(name string, requiredModality modality.Modality, requiredCapabilities ...modality.Capability) (Model, error) {
	resolution, err := r.RequireResolvedModel(name, requiredModality, nil, requiredCapabilities...)
	if err != nil {
		return Model{}, err
	}
	return resolution.Model, nil
}

func (r *Registry) RequireResolvedModel(name string, requiredModality modality.Modality, routing *modality.RoutingOptions, requiredCapabilities ...modality.Capability) (Resolution, error) {
	if model, ok := r.models[name]; ok {
		if err := validateResolvedModel(model, requiredModality, requiredCapabilities); err != nil {
			return Resolution{}, err
		}
		return Resolution{
			Model:            model,
			RequestedModel:   name,
			ResolvedModel:    model.ID,
			ResolvedProvider: model.Provider,
			Mode:             "exact",
		}, nil
	}
	if target, ok := r.aliases[name]; ok {
		model := r.models[target]
		if err := validateResolvedModel(model, requiredModality, requiredCapabilities); err != nil {
			return Resolution{}, err
		}
		return Resolution{
			Model:            model,
			RequestedModel:   name,
			ResolvedModel:    model.ID,
			ResolvedProvider: model.Provider,
			Mode:             "alias",
		}, nil
	}
	if familyID, ok := r.familyAliases[name]; ok {
		model, err := r.resolveFamilyModel(familyID, requiredModality, requiredCapabilities, routing)
		if err != nil {
			return Resolution{}, err
		}
		return Resolution{
			Model:            model,
			RequestedModel:   name,
			ResolvedModel:    model.ID,
			ResolvedProvider: model.Provider,
			Mode:             resolutionMode("family", routing),
		}, nil
	}
	if _, ok := r.families[name]; ok {
		model, err := r.resolveFamilyModel(name, requiredModality, requiredCapabilities, routing)
		if err != nil {
			return Resolution{}, err
		}
		return Resolution{
			Model:            model,
			RequestedModel:   name,
			ResolvedModel:    model.ID,
			ResolvedProvider: model.Provider,
			Mode:             resolutionMode("family", routing),
		}, nil
	}
	if selector, ok := r.selectors[name]; ok {
		model, err := selectModelForSelector(r, name, selector, requiredModality, requiredCapabilities, routing)
		if err != nil {
			return Resolution{}, err
		}
		return Resolution{
			Model:            model,
			RequestedModel:   name,
			ResolvedModel:    model.ID,
			ResolvedProvider: model.Provider,
			Mode:             resolutionMode("selector", routing),
		}, nil
	}
	if !strings.Contains(name, "/") {
		return Resolution{}, fmt.Errorf("%w: %s", ErrUnknownAlias, name)
	}
	return Resolution{}, fmt.Errorf("%w: %s", ErrUnknownModel, name)
}

func (r *Registry) resolveFamilyModel(familyID string, requiredModality modality.Modality, requiredCapabilities []modality.Capability, routing *modality.RoutingOptions) (Model, error) {
	family, ok := r.families[familyID]
	if !ok || len(family.Variants) == 0 {
		return Model{}, fmt.Errorf("%w: %s", ErrUnknownAlias, familyID)
	}

	candidates := make([]Model, 0, len(family.Variants))
	for _, modelID := range family.Variants {
		model, ok := r.models[modelID]
		if !ok {
			continue
		}
		if requiredModality != "" && model.Modality != requiredModality {
			continue
		}
		matches := true
		for _, capability := range requiredCapabilities {
			if !containsCapability(model.Capabilities, capability) {
				matches = false
				break
			}
		}
		if matches {
			candidates = append(candidates, model)
		}
	}
	return selectModelCandidates(r, familyID, candidates, mergedRoutingPolicy(config.RoutingSelector{Modality: requiredModality, Capabilities: requiredCapabilities}, routing))
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
		items = append(items, aliasModel(alias, "alias", resolved, target, resolved.Capabilities))
	}

	for familyID, family := range r.families {
		resolved, err := r.resolveFamilyModel(familyID, family.Modality, nil, nil)
		if err != nil {
			continue
		}
		items = append(items, aliasModel(familyID, "family", resolved, resolved.ID, resolved.Capabilities))
		for _, alias := range family.Aliases {
			items = append(items, aliasModel(alias, "family", resolved, resolved.ID, resolved.Capabilities))
		}
	}

	for alias, selector := range r.selectors {
		resolved, err := selectModelForSelector(r, alias, selector, selector.Modality, selector.Capabilities, nil)
		if err != nil {
			continue
		}
		items = append(items, aliasModel(alias, "selector", resolved, resolved.ID, mergeCapabilities(selector.Capabilities, resolved.Capabilities)))
	}

	slices.SortFunc(items, func(a, b Model) int {
		return strings.Compare(a.ID, b.ID)
	})
	return dedupeModels(items)
}

func (r *Registry) GetFallbacks(modelID string) []string {
	return append([]string(nil), r.fallbacks[modelID]...)
}

func aliasModel(id string, kind string, resolved Model, target string, capabilities []modality.Capability) Model {
	return Model{
		ID:                id,
		Object:            "model",
		Kind:              kind,
		Provider:          resolved.Provider,
		ProviderVariant:   resolved.ID,
		DisplayName:       resolved.DisplayName,
		FamilyID:          resolved.FamilyID,
		FamilyDisplayName: resolved.FamilyDisplayName,
		Status:            resolved.Status,
		VerificationClass: resolved.VerificationClass,
		CostTier:          resolved.CostTier,
		LatencyTier:       resolved.LatencyTier,
		DocURL:            resolved.DocURL,
		LastVerified:      resolved.LastVerified,
		Modality:          resolved.Modality,
		Capabilities:      append([]modality.Capability(nil), capabilities...),
		ContextWindow:     resolved.ContextWindow,
		MaxOutputTokens:   resolved.MaxOutputTokens,
		MaxDuration:       resolved.MaxDuration,
		AllowedDurations:  append([]int(nil), resolved.AllowedDurations...),
		AspectRatios:      append([]string(nil), resolved.AspectRatios...),
		Resolutions:       append([]string(nil), resolved.Resolutions...),
		Cancelable:        resolved.Cancelable,
		Voices:            append([]string(nil), resolved.Voices...),
		Formats:           append([]string(nil), resolved.Formats...),
		OutputFormats:     append([]string(nil), resolved.OutputFormats...),
		MinDurationMs:     resolved.MinDurationMs,
		MaxDurationMs:     resolved.MaxDurationMs,
		SampleRatesHz:     append([]int(nil), resolved.SampleRatesHz...),
		Dimensions:        resolved.Dimensions,
		SessionTTL:        resolved.SessionTTL,
		ResolvesTo:        target,
	}
}

func dedupeModels(items []Model) []Model {
	seen := make(map[string]struct{}, len(items))
	out := make([]Model, 0, len(items))
	for _, item := range items {
		if _, exists := seen[item.ID]; exists {
			continue
		}
		seen[item.ID] = struct{}{}
		out = append(out, item)
	}
	return out
}

func verificationRank(class string) int {
	switch class {
	case "strict":
		return 0
	case "opt_in":
		return 1
	case "skipped":
		return 2
	default:
		return 3
	}
}

func validateResolvedModel(model Model, requiredModality modality.Modality, requiredCapabilities []modality.Capability) error {
	if requiredModality != "" && model.Modality != requiredModality {
		return fmt.Errorf("%w: requested %s got %s", ErrModalityMismatch, requiredModality, model.Modality)
	}
	for _, capability := range requiredCapabilities {
		if !containsCapability(model.Capabilities, capability) {
			return fmt.Errorf("%w: %s", ErrCapabilityMissing, capability)
		}
	}
	return nil
}

func resolutionMode(base string, routing *modality.RoutingOptions) string {
	if routing != nil && !routing.Empty() {
		return "request_hint"
	}
	return base
}

func statusRank(status string) int {
	switch status {
	case "ga":
		return 0
	case "preview":
		return 1
	case "experimental":
		return 2
	default:
		return 3
	}
}

func (r *Registry) ListConfiguredVoices(providerFilter string, modelName string) ([]modality.VoiceCatalogItem, error) {
	trimmedProvider := strings.TrimSpace(providerFilter)
	itemsByKey := map[string]*modality.VoiceCatalogItem{}

	appendModelVoices := func(model Model) {
		if len(model.Voices) == 0 {
			return
		}
		if trimmedProvider != "" && model.Provider != trimmedProvider {
			return
		}
		for _, voiceID := range model.Voices {
			trimmedVoiceID := strings.TrimSpace(voiceID)
			if trimmedVoiceID == "" {
				continue
			}
			key := model.Provider + ":" + trimmedVoiceID
			item := itemsByKey[key]
			if item == nil {
				item = &modality.VoiceCatalogItem{
					ID:       trimmedVoiceID,
					Provider: model.Provider,
					Type:     "configured",
				}
				itemsByKey[key] = item
			}
			if !slices.Contains(item.Models, model.ID) {
				item.Models = append(item.Models, model.ID)
			}
		}
	}

	if strings.TrimSpace(modelName) != "" {
		model, err := r.ResolveModel(modelName)
		if err != nil {
			return nil, err
		}
		appendModelVoices(model)
	} else {
		for _, model := range r.models {
			appendModelVoices(model)
		}
	}

	items := make([]modality.VoiceCatalogItem, 0, len(itemsByKey))
	for _, item := range itemsByKey {
		slices.Sort(item.Models)
		items = append(items, *item)
	}
	slices.SortFunc(items, func(a, b modality.VoiceCatalogItem) int {
		if compare := strings.Compare(a.Provider, b.Provider); compare != 0 {
			return compare
		}
		return strings.Compare(a.ID, b.ID)
	})
	return items, nil
}
