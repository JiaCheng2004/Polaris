package provider

import (
	"fmt"
	"slices"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func registerConfiguredAliases(registry *Registry, routing config.RoutingConfig, warnings *[]string) {
	for alias, target := range routing.Aliases {
		if _, ok := registry.models[target]; !ok {
			*warnings = append(*warnings, fmt.Sprintf("alias %s points to unavailable model %s", alias, target))
			continue
		}
		registry.aliases[alias] = target
	}
}

func registerCatalogAliases(registry *Registry) {
	for modelID := range registry.models {
		entry, ok := lookupModelCatalog(modelID)
		if !ok {
			continue
		}
		for _, alias := range entry.Aliases {
			if _, exists := registry.aliases[alias]; exists {
				continue
			}
			registry.aliases[alias] = modelID
		}
	}
}

func registerCatalogFamilies(registry *Registry) {
	for modelID, model := range registry.models {
		if strings.TrimSpace(model.FamilyID) == "" {
			continue
		}
		familyMeta, ok := lookupModelFamily(model.FamilyID)
		if !ok {
			continue
		}
		family := registry.families[model.FamilyID]
		if family.ID == "" {
			family = modelFamily{
				ID:          familyMeta.ID,
				DisplayName: familyMeta.DisplayName,
				Modality:    familyMeta.Modality,
				Aliases:     append([]string(nil), familyMeta.Aliases...),
			}
		}
		if !slices.Contains(family.Variants, modelID) {
			family.Variants = append(family.Variants, modelID)
		}
		registry.families[model.FamilyID] = family
	}

	for familyID, family := range registry.families {
		slices.SortFunc(family.Variants, func(a, b string) int {
			aModel := registry.models[a]
			bModel := registry.models[b]
			if aModel.FamilyPriority != bModel.FamilyPriority {
				if aModel.FamilyPriority < bModel.FamilyPriority {
					return -1
				}
				return 1
			}
			return strings.Compare(aModel.ID, bModel.ID)
		})
		registry.families[familyID] = family
		for _, alias := range family.Aliases {
			if _, exists := registry.familyAliases[alias]; exists {
				continue
			}
			registry.familyAliases[alias] = familyID
		}
	}
}

func registerSelectors(registry *Registry, routing config.RoutingConfig, warnings *[]string) {
	for alias, selector := range routing.Selectors {
		if _, exists := registry.aliases[alias]; exists {
			*warnings = append(*warnings, fmt.Sprintf("selector %s conflicts with an existing alias and was skipped", alias))
			continue
		}
		if _, exists := registry.models[alias]; exists {
			*warnings = append(*warnings, fmt.Sprintf("selector %s conflicts with an existing model id and was skipped", alias))
			continue
		}
		if _, exists := registry.selectors[alias]; exists {
			*warnings = append(*warnings, fmt.Sprintf("selector %s is defined multiple times and was skipped", alias))
			continue
		}
		if _, err := selectModelForSelector(registry, alias, selector, selector.Modality, selector.Capabilities, nil); err != nil {
			*warnings = append(*warnings, fmt.Sprintf("selector %s did not match any enabled model and was skipped", alias))
			continue
		}
		registry.selectors[alias] = selector
	}
}

func registerFallbacks(registry *Registry, routing config.RoutingConfig) {
	for _, rule := range routing.Fallbacks {
		if _, ok := registry.models[rule.From]; !ok {
			continue
		}
		for _, target := range rule.To {
			if _, ok := registry.models[target]; ok {
				registry.fallbacks[rule.From] = append(registry.fallbacks[rule.From], target)
			}
		}
	}
}

func selectModelForSelector(registry *Registry, alias string, selector config.RoutingSelector, requiredModality modality.Modality, requiredCapabilities []modality.Capability, routing *modality.RoutingOptions) (Model, error) {
	candidates := make([]Model, 0, len(registry.models))
	for _, model := range registry.models {
		candidates = append(candidates, model)
	}
	return selectModelCandidates(registry, alias, candidates, mergedRoutingPolicy(selectorOverride(selector, requiredModality, requiredCapabilities), routing))
}

func mergeCapabilities(a []modality.Capability, b []modality.Capability) []modality.Capability {
	out := make([]modality.Capability, 0, len(a)+len(b))
	for _, items := range [][]modality.Capability{a, b} {
		for _, capability := range items {
			if !containsCapability(out, capability) {
				out = append(out, capability)
			}
		}
	}
	return out
}

func modelMatchesSelector(
	model Model,
	requiredModality modality.Modality,
	requiredCapabilities []modality.Capability,
	excludeProviders map[string]struct{},
	allowedStatuses map[string]struct{},
	allowedVerificationClasses map[string]struct{},
	allowedProviders []string,
) bool {
	if requiredModality != "" && model.Modality != requiredModality {
		return false
	}
	if _, excluded := excludeProviders[model.Provider]; excluded {
		return false
	}
	if len(allowedProviders) > 0 && !slices.Contains(allowedProviders, model.Provider) {
		return false
	}
	if len(allowedStatuses) > 0 {
		if _, ok := allowedStatuses[model.Status]; !ok {
			return false
		}
	}
	if len(allowedVerificationClasses) > 0 {
		if _, ok := allowedVerificationClasses[model.VerificationClass]; !ok {
			return false
		}
	}
	for _, capability := range requiredCapabilities {
		if !containsCapability(model.Capabilities, capability) {
			return false
		}
	}
	return true
}

type routingPolicy struct {
	modality            modality.Modality
	capabilities        []modality.Capability
	providers           []string
	excludeProviders    map[string]struct{}
	statuses            map[string]struct{}
	verificationClasses map[string]struct{}
	prefer              []string
	costTier            string
	latencyTier         string
}

func selectorOverride(selector config.RoutingSelector, requiredModality modality.Modality, requiredCapabilities []modality.Capability) config.RoutingSelector {
	if requiredModality != "" {
		selector.Modality = requiredModality
	}
	selector.Capabilities = mergeCapabilities(selector.Capabilities, requiredCapabilities)
	return selector
}

func mergedRoutingPolicy(selector config.RoutingSelector, routing *modality.RoutingOptions) routingPolicy {
	providers := append([]string(nil), selector.Providers...)
	if routing != nil && len(routing.Providers) > 0 {
		if len(providers) > 0 {
			providers = intersectOrdered(routing.Providers, providers)
		} else {
			providers = append([]string(nil), routing.Providers...)
		}
	}

	statuses := append([]string(nil), selector.Statuses...)
	if routing != nil && len(routing.Statuses) > 0 {
		if len(statuses) > 0 {
			statuses = intersectUnordered(routing.Statuses, statuses)
		} else {
			statuses = append([]string(nil), routing.Statuses...)
		}
	}

	verificationClasses := append([]string(nil), selector.VerificationClasses...)
	if routing != nil && len(routing.VerificationClasses) > 0 {
		if len(verificationClasses) > 0 {
			verificationClasses = intersectUnordered(routing.VerificationClasses, verificationClasses)
		} else {
			verificationClasses = append([]string(nil), routing.VerificationClasses...)
		}
	}

	excludeProviders := append([]string(nil), selector.ExcludeProviders...)
	if routing != nil {
		excludeProviders = appendUniqueStrings(excludeProviders, routing.ExcludeProviders...)
	}

	prefer := append([]string(nil), selector.Prefer...)
	if routing != nil && len(routing.Prefer) > 0 {
		prefer = appendUniqueStrings(append([]string(nil), routing.Prefer...), selector.Prefer...)
	}

	capabilities := append([]modality.Capability(nil), selector.Capabilities...)
	if routing != nil && len(routing.Capabilities) > 0 {
		capabilities = mergeCapabilities(capabilities, routing.Capabilities)
	}

	costTier := strings.TrimSpace(selector.CostTier)
	latencyTier := strings.TrimSpace(selector.LatencyTier)
	if routing != nil {
		if strings.TrimSpace(routing.CostTier) != "" {
			costTier = strings.TrimSpace(routing.CostTier)
		}
		if strings.TrimSpace(routing.LatencyTier) != "" {
			latencyTier = strings.TrimSpace(routing.LatencyTier)
		}
	}

	exclude := make(map[string]struct{}, len(excludeProviders))
	for _, providerName := range excludeProviders {
		exclude[strings.TrimSpace(providerName)] = struct{}{}
	}

	return routingPolicy{
		modality:            selector.Modality,
		capabilities:        capabilities,
		providers:           providers,
		excludeProviders:    exclude,
		statuses:            toSet(statuses),
		verificationClasses: toSet(verificationClasses),
		prefer:              prefer,
		costTier:            costTier,
		latencyTier:         latencyTier,
	}
}

func selectModelCandidates(registry *Registry, alias string, candidates []Model, policy routingPolicy) (Model, error) {
	filtered := make([]Model, 0, len(candidates))
	for _, model := range candidates {
		if modelMatchesSelector(model, policy.modality, policy.capabilities, policy.excludeProviders, policy.statuses, policy.verificationClasses, policy.providers) {
			filtered = append(filtered, model)
		}
	}
	if len(filtered) == 0 {
		return Model{}, fmt.Errorf("%w: %s", ErrRouteNotResolved, alias)
	}

	filteredSet := make(map[string]struct{}, len(filtered))
	for _, model := range filtered {
		filteredSet[model.ID] = struct{}{}
	}
	for _, preferred := range policy.prefer {
		model, err := registry.ResolveModel(preferred)
		if err != nil {
			continue
		}
		if _, ok := filteredSet[model.ID]; ok {
			return model, nil
		}
	}

	providerPriority := toPriorityMap(policy.providers)
	slices.SortFunc(filtered, func(a, b Model) int {
		aRank := selectorRank(a, providerPriority)
		bRank := selectorRank(b, providerPriority)
		if aRank != bRank {
			if aRank < bRank {
				return -1
			}
			return 1
		}
		if rank := tierRank(a.CostTier, policy.costTier) - tierRank(b.CostTier, policy.costTier); rank != 0 {
			return rank
		}
		if rank := tierRank(a.LatencyTier, policy.latencyTier) - tierRank(b.LatencyTier, policy.latencyTier); rank != 0 {
			return rank
		}
		if a.FamilyPriority != b.FamilyPriority {
			if a.FamilyPriority < b.FamilyPriority {
				return -1
			}
			return 1
		}
		if rank := verificationRank(a.VerificationClass) - verificationRank(b.VerificationClass); rank != 0 {
			return rank
		}
		if rank := statusRank(a.Status) - statusRank(b.Status); rank != 0 {
			return rank
		}
		return strings.Compare(a.ID, b.ID)
	})

	return filtered[0], nil
}

func toSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[strings.TrimSpace(value)] = struct{}{}
	}
	return out
}

func toPriorityMap(values []string) map[string]int {
	out := make(map[string]int, len(values))
	for index, value := range values {
		out[strings.TrimSpace(value)] = index
	}
	return out
}

func selectorRank(model Model, priority map[string]int) int {
	if rank, ok := priority[model.Provider]; ok {
		return rank
	}
	return len(priority)
}

func tierRank(modelTier string, requestedTier string) int {
	if strings.TrimSpace(requestedTier) == "" {
		return 0
	}
	switch {
	case strings.EqualFold(strings.TrimSpace(modelTier), strings.TrimSpace(requestedTier)):
		return 0
	case strings.TrimSpace(modelTier) == "":
		return 2
	default:
		return 1
	}
}

func appendUniqueStrings(base []string, extra ...string) []string {
	out := append([]string(nil), base...)
	seen := make(map[string]struct{}, len(out))
	for _, item := range out {
		seen[strings.TrimSpace(item)] = struct{}{}
	}
	for _, item := range extra {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func intersectOrdered(primary []string, secondary []string) []string {
	if len(primary) == 0 || len(secondary) == 0 {
		return nil
	}
	allowed := toSet(secondary)
	out := make([]string, 0, len(primary))
	for _, item := range primary {
		trimmed := strings.TrimSpace(item)
		if _, ok := allowed[trimmed]; ok {
			out = append(out, trimmed)
		}
	}
	return out
}

func intersectUnordered(primary []string, secondary []string) []string {
	if len(primary) == 0 || len(secondary) == 0 {
		return nil
	}
	allowed := toSet(secondary)
	out := make([]string, 0, len(primary))
	for _, item := range primary {
		trimmed := strings.TrimSpace(item)
		if _, ok := allowed[trimmed]; ok {
			out = append(out, trimmed)
		}
	}
	return out
}
