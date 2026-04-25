package verification

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/provider/catalog"
)

type CountSummary struct {
	Strict  int `json:"strict"`
	OptIn   int `json:"opt_in"`
	Skipped int `json:"skipped"`
}

type ProviderSummary struct {
	Name           string       `json:"name"`
	RuntimeEnabled bool         `json:"runtime_enabled"`
	Counts         CountSummary `json:"counts"`
}

type ModelSummary struct {
	ID                string `json:"id"`
	Provider          string `json:"provider"`
	DisplayName       string `json:"display_name,omitempty"`
	Modality          string `json:"modality"`
	Status            string `json:"status,omitempty"`
	VerificationClass string `json:"verification_class,omitempty"`
	RuntimeEnabled    bool   `json:"runtime_enabled"`
}

type RouteSummary struct {
	Name              string `json:"name"`
	Kind              string `json:"kind"`
	ResolvesTo        string `json:"resolves_to,omitempty"`
	Provider          string `json:"provider,omitempty"`
	VerificationClass string `json:"verification_class,omitempty"`
	Status            string `json:"status,omitempty"`
}

type Report struct {
	Counts    CountSummary      `json:"counts"`
	Providers []ProviderSummary `json:"providers"`
	Models    []ModelSummary    `json:"models"`
	Aliases   []RouteSummary    `json:"aliases"`
	Selectors []RouteSummary    `json:"selectors"`
	Warnings  []string          `json:"warnings,omitempty"`
	Problems  []string          `json:"problems,omitempty"`
}

func (r Report) Valid() bool {
	return len(r.Problems) == 0
}

func (r Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

func (r Report) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Polaris configured model verification\n\n")
	fmt.Fprintf(&b, "Counts\n")
	fmt.Fprintf(&b, "  strict:  %d\n", r.Counts.Strict)
	fmt.Fprintf(&b, "  opt_in:  %d\n", r.Counts.OptIn)
	fmt.Fprintf(&b, "  skipped: %d\n", r.Counts.Skipped)

	if len(r.Providers) > 0 {
		fmt.Fprintf(&b, "\nProviders\n")
		for _, item := range r.Providers {
			state := "disabled"
			if item.RuntimeEnabled {
				state = "enabled"
			}
			fmt.Fprintf(&b, "  - %s [%s] strict=%d opt_in=%d skipped=%d\n", item.Name, state, item.Counts.Strict, item.Counts.OptIn, item.Counts.Skipped)
		}
	}

	if len(r.Models) > 0 {
		fmt.Fprintf(&b, "\nModels\n")
		for _, item := range r.Models {
			state := "disabled"
			if item.RuntimeEnabled {
				state = "enabled"
			}
			fmt.Fprintf(&b, "  - %s (%s, %s, %s, %s)\n", item.ID, item.Modality, item.Status, item.VerificationClass, state)
		}
	}

	if len(r.Aliases) > 0 {
		fmt.Fprintf(&b, "\nAliases\n")
		for _, item := range r.Aliases {
			fmt.Fprintf(&b, "  - %s -> %s (%s, %s)\n", item.Name, item.ResolvesTo, item.Status, item.VerificationClass)
		}
	}

	if len(r.Selectors) > 0 {
		fmt.Fprintf(&b, "\nSelectors\n")
		for _, item := range r.Selectors {
			fmt.Fprintf(&b, "  - %s -> %s (%s, %s)\n", item.Name, item.ResolvesTo, item.Status, item.VerificationClass)
		}
	}

	if len(r.Warnings) > 0 {
		fmt.Fprintf(&b, "\nWarnings\n")
		for _, warning := range r.Warnings {
			fmt.Fprintf(&b, "  - %s\n", warning)
		}
	}

	if len(r.Problems) > 0 {
		fmt.Fprintf(&b, "\nProblems\n")
		for _, problem := range r.Problems {
			fmt.Fprintf(&b, "  - %s\n", problem)
		}
	}

	return b.String()
}

func BuildReport(cfg *config.Config, registry *provider.Registry, warnings []string) (Report, error) {
	if cfg == nil {
		return Report{}, fmt.Errorf("config is required")
	}

	cat, err := catalog.Default()
	if err != nil {
		return Report{}, fmt.Errorf("load provider catalog: %w", err)
	}

	configured := configuredModels(cfg, cat)

	report := Report{
		Warnings: append([]string(nil), warnings...),
	}

	providerCounts := make(map[string]CountSummary)

	modelIDs := make([]string, 0, len(configured))
	for id := range configured {
		modelIDs = append(modelIDs, id)
	}
	sort.Strings(modelIDs)

	for _, id := range modelIDs {
		model := configured[id]
		runtimeEnabled := provider.ProviderRuntimeEnabled(model.Provider, cfg.Providers[model.Provider])
		class, err := classForModel(model)
		if err != nil {
			report.Problems = append(report.Problems, fmt.Sprintf("%s: %v", model.ID, err))
		}
		incrementCounts(&report.Counts, class)
		counts := providerCounts[model.Provider]
		incrementCounts(&counts, class)
		providerCounts[model.Provider] = counts

		report.Models = append(report.Models, ModelSummary{
			ID:                model.ID,
			Provider:          model.Provider,
			DisplayName:       model.DisplayName,
			Modality:          string(model.Modality),
			Status:            model.Status,
			VerificationClass: model.VerificationClass,
			RuntimeEnabled:    runtimeEnabled,
		})
	}

	providerNames := make([]string, 0, len(cfg.Providers))
	for name := range cfg.Providers {
		providerNames = append(providerNames, name)
	}
	sort.Strings(providerNames)
	for _, name := range providerNames {
		report.Providers = append(report.Providers, ProviderSummary{
			Name:           name,
			RuntimeEnabled: provider.ProviderRuntimeEnabled(name, cfg.Providers[name]),
			Counts:         providerCounts[name],
		})
	}

	aliasNames := make([]string, 0, len(cfg.Routing.Aliases))
	for name := range cfg.Routing.Aliases {
		aliasNames = append(aliasNames, name)
	}
	sort.Strings(aliasNames)
	for _, name := range aliasNames {
		route, err := routeSummaryForName(cfg, configured, name, "alias")
		if err != nil {
			report.Problems = append(report.Problems, err.Error())
			continue
		}
		report.Aliases = append(report.Aliases, route)
	}

	selectorNames := make([]string, 0, len(cfg.Routing.Selectors))
	for name := range cfg.Routing.Selectors {
		selectorNames = append(selectorNames, name)
	}
	sort.Strings(selectorNames)
	for _, name := range selectorNames {
		route, err := routeSummaryForName(cfg, configured, name, "selector")
		if err != nil {
			report.Problems = append(report.Problems, err.Error())
			continue
		}
		report.Selectors = append(report.Selectors, route)
	}

	return report, nil
}

func configuredModels(cfg *config.Config, cat *catalog.Catalog) map[string]provider.Model {
	out := make(map[string]provider.Model)
	for providerName, providerCfg := range cfg.Providers {
		for modelName, modelCfg := range providerCfg.Models {
			id := providerName + "/" + modelName
			model := provider.Model{
				ID:           id,
				Provider:     providerName,
				Name:         modelName,
				Modality:     modelCfg.Modality,
				Capabilities: append([]modality.Capability(nil), modelCfg.Capabilities...),
			}
			if entry, ok := cat.Lookup(id); ok {
				model.DisplayName = entry.DisplayName
				model.Status = entry.Status
				model.VerificationClass = entry.VerificationClass
				model.DocURL = entry.DocURL
				model.LastVerified = entry.LastVerified
			}
			out[id] = model
		}
	}
	return out
}

func routeSummaryForName(cfg *config.Config, configured map[string]provider.Model, name string, kind string) (RouteSummary, error) {
	model, err := resolveConfiguredModel(cfg, configured, name)
	if err != nil {
		return RouteSummary{}, fmt.Errorf("%s %q: %w", kind, name, err)
	}
	return RouteSummary{
		Name:              name,
		Kind:              kind,
		ResolvesTo:        model.ID,
		Provider:          model.Provider,
		VerificationClass: model.VerificationClass,
		Status:            model.Status,
	}, nil
}

func resolveConfiguredModelWithRuntime(cfg *config.Config, registry *provider.Registry, configured map[string]provider.Model, name string) (provider.Model, error) {
	if registry != nil {
		if model, err := registry.ResolveModel(name); err == nil {
			return model, nil
		}
		if target, ok := cfg.Routing.Aliases[name]; ok {
			if model, err := registry.ResolveModel(target); err == nil {
				return model, nil
			}
		}
	}
	return resolveConfiguredModel(cfg, configured, name)
}

func incrementCounts(counts *CountSummary, class Class) {
	switch class {
	case ClassStrict:
		counts.Strict++
	case ClassOptIn:
		counts.OptIn++
	case ClassSkipped:
		counts.Skipped++
	}
}

func resolveConfiguredModel(cfg *config.Config, configured map[string]provider.Model, name string) (provider.Model, error) {
	if model, ok := configured[name]; ok {
		return model, nil
	}
	if target, ok := cfg.Routing.Aliases[name]; ok {
		model, ok := configured[target]
		if !ok {
			return provider.Model{}, fmt.Errorf("unknown alias: %s", name)
		}
		return model, nil
	}
	if selector, ok := cfg.Routing.Selectors[name]; ok {
		return selectConfiguredModelForSelector(configured, name, selector)
	}
	return provider.Model{}, fmt.Errorf("unknown model: %s", name)
}

func selectConfiguredModelForSelector(configured map[string]provider.Model, alias string, selector config.RoutingSelector) (provider.Model, error) {
	required := mergeCapabilities(selector.Capabilities, nil)
	excludeProviders := toSet(selector.ExcludeProviders)
	allowedStatuses := toSet(selector.Statuses)
	allowedVerificationClasses := toSet(selector.VerificationClasses)
	providerPriority := toPriorityMap(selector.Providers)

	pickByName := func(name string) (provider.Model, bool) {
		model, ok := configured[name]
		if ok && modelMatchesSelector(model, selector.Modality, required, excludeProviders, allowedStatuses, allowedVerificationClasses, selector.Providers) {
			return model, true
		}
		return provider.Model{}, false
	}

	for _, preferred := range selector.Prefer {
		if model, ok := pickByName(preferred); ok {
			return model, nil
		}
	}

	candidates := make([]provider.Model, 0, len(configured))
	for _, model := range configured {
		if modelMatchesSelector(model, selector.Modality, required, excludeProviders, allowedStatuses, allowedVerificationClasses, selector.Providers) {
			candidates = append(candidates, model)
		}
	}
	if len(candidates) == 0 {
		return provider.Model{}, fmt.Errorf("%w: %s", provider.ErrRouteNotResolved, alias)
	}

	slices.SortFunc(candidates, func(a, b provider.Model) int {
		aRank := selectorRank(a, providerPriority)
		bRank := selectorRank(b, providerPriority)
		if aRank != bRank {
			if aRank < bRank {
				return -1
			}
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})

	return candidates[0], nil
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

func containsCapability(capabilities []modality.Capability, candidate modality.Capability) bool {
	for _, capability := range capabilities {
		if capability == candidate {
			return true
		}
	}
	return false
}

func modelMatchesSelector(
	model provider.Model,
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

func selectorRank(model provider.Model, priority map[string]int) int {
	if rank, ok := priority[model.Provider]; ok {
		return rank
	}
	return len(priority)
}
