package verification

import (
	"fmt"
	"sort"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/provider/catalog"
)

type Class string

const (
	ClassStrict  Class = "strict"
	ClassOptIn   Class = "opt_in"
	ClassSkipped Class = "skipped"
)

type Result struct {
	Class     Class
	Models    []provider.Model
	Providers []string
}

func ForModels(registry *provider.Registry, names ...string) (Result, error) {
	if registry == nil {
		return Result{}, fmt.Errorf("registry is required")
	}
	if len(names) == 0 {
		return Result{}, fmt.Errorf("at least one model name is required")
	}

	models := make([]provider.Model, 0, len(names))
	providers := make(map[string]struct{})
	class := ClassSkipped

	for _, name := range names {
		model, err := registry.ResolveModel(name)
		if err != nil {
			return Result{}, fmt.Errorf("resolve model %q: %w", name, err)
		}
		modelClass, err := classForModel(model)
		if err != nil {
			return Result{}, fmt.Errorf("model %q: %w", model.ID, err)
		}
		class = mergeClass(class, modelClass)
		models = append(models, model)
		if model.Provider != "" {
			providers[model.Provider] = struct{}{}
		}
	}

	providerNames := make([]string, 0, len(providers))
	for name := range providers {
		providerNames = append(providerNames, name)
	}
	sort.Strings(providerNames)

	return Result{
		Class:     class,
		Models:    models,
		Providers: providerNames,
	}, nil
}

func ForConfig(cfg *config.Config, registry *provider.Registry, names ...string) (Result, error) {
	if cfg == nil {
		return Result{}, fmt.Errorf("config is required")
	}
	if len(names) == 0 {
		return Result{}, fmt.Errorf("at least one model name is required")
	}

	cat, err := catalog.Default()
	if err != nil {
		return Result{}, fmt.Errorf("load provider catalog: %w", err)
	}
	configured := configuredModels(cfg, cat)

	models := make([]provider.Model, 0, len(names))
	providers := make(map[string]struct{})
	class := ClassSkipped

	for _, name := range names {
		model, err := resolveConfiguredModelWithRuntime(cfg, registry, configured, name)
		if err != nil {
			return Result{}, err
		}
		modelClass, err := classForModel(model)
		if err != nil {
			return Result{}, fmt.Errorf("model %q: %w", model.ID, err)
		}
		class = mergeClass(class, modelClass)
		models = append(models, model)
		if model.Provider != "" {
			providers[model.Provider] = struct{}{}
		}
	}

	providerNames := make([]string, 0, len(providers))
	for name := range providers {
		providerNames = append(providerNames, name)
	}
	sort.Strings(providerNames)

	return Result{
		Class:     class,
		Models:    models,
		Providers: providerNames,
	}, nil
}

func classForModel(model provider.Model) (Class, error) {
	switch model.VerificationClass {
	case string(ClassStrict):
		return ClassStrict, nil
	case string(ClassOptIn):
		return ClassOptIn, nil
	case string(ClassSkipped):
		return ClassSkipped, nil
	case "":
		return "", fmt.Errorf("missing verification metadata")
	default:
		return "", fmt.Errorf("unsupported verification_class %q", model.VerificationClass)
	}
}

func mergeClass(current, next Class) Class {
	if current == ClassStrict || next == ClassStrict {
		return ClassStrict
	}
	if current == ClassOptIn || next == ClassOptIn {
		return ClassOptIn
	}
	return ClassSkipped
}
