package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/provider/catalog"
	"gopkg.in/yaml.v3"
)

const currentConfigVersion = 2

func loadV2(path string) ([]byte, []string, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve config path: %w", err)
	}

	raw, warnings, err := loadMergedConfigMap(absolutePath, map[string]struct{}{})
	if err != nil {
		return nil, warnings, err
	}

	normalized, err := normalizeV2Config(raw)
	if err != nil {
		return nil, warnings, err
	}

	data, err := yaml.Marshal(normalized)
	if err != nil {
		return nil, warnings, fmt.Errorf("encode normalized config: %w", err)
	}
	return data, warnings, nil
}

func ConfigFiles(path string) ([]string, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	seen := map[string]struct{}{}
	files, err := collectConfigFiles(absolutePath, map[string]struct{}{}, seen)
	if err != nil {
		return nil, err
	}
	return files, nil
}

func loadMergedConfigMap(path string, stack map[string]struct{}) (map[string]any, []string, error) {
	cleanPath := filepath.Clean(path)
	if _, exists := stack[cleanPath]; exists {
		return nil, nil, fmt.Errorf("config import cycle includes %s", cleanPath)
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read config %s: %w", cleanPath, err)
	}
	expanded, warnings := expandEnv(string(data))

	raw := map[string]any{}
	if err := yaml.Unmarshal([]byte(expanded), &raw); err != nil {
		return nil, warnings, fmt.Errorf("decode config yaml %s: %w", cleanPath, err)
	}
	if err := requireV2(raw, cleanPath); err != nil {
		return nil, warnings, err
	}

	imports, err := parseImports(raw["imports"], cleanPath)
	if err != nil {
		return nil, warnings, err
	}

	stack[cleanPath] = struct{}{}
	merged := map[string]any{}
	for _, importPath := range imports {
		imported, importWarnings, err := loadMergedConfigMap(importPath, stack)
		warnings = append(warnings, importWarnings...)
		if err != nil {
			delete(stack, cleanPath)
			return nil, uniqueSortedStrings(warnings), err
		}
		merged = mergeMaps(merged, imported)
	}
	delete(stack, cleanPath)

	delete(raw, "imports")
	merged = mergeMaps(merged, raw)
	return merged, uniqueSortedStrings(warnings), nil
}

func collectConfigFiles(path string, stack map[string]struct{}, seen map[string]struct{}) ([]string, error) {
	cleanPath := filepath.Clean(path)
	if _, exists := stack[cleanPath]; exists {
		return nil, fmt.Errorf("config import cycle includes %s", cleanPath)
	}
	if _, exists := seen[cleanPath]; exists {
		return nil, nil
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", cleanPath, err)
	}
	expanded, _ := expandEnv(string(data))

	raw := map[string]any{}
	if err := yaml.Unmarshal([]byte(expanded), &raw); err != nil {
		return nil, fmt.Errorf("decode config yaml %s: %w", cleanPath, err)
	}
	if err := requireV2(raw, cleanPath); err != nil {
		return nil, err
	}

	imports, err := parseImports(raw["imports"], cleanPath)
	if err != nil {
		return nil, err
	}

	stack[cleanPath] = struct{}{}
	files := []string{cleanPath}
	seen[cleanPath] = struct{}{}
	for _, importPath := range imports {
		importFiles, err := collectConfigFiles(importPath, stack, seen)
		if err != nil {
			delete(stack, cleanPath)
			return nil, err
		}
		files = append(files, importFiles...)
	}
	delete(stack, cleanPath)
	return files, nil
}

func requireV2(raw map[string]any, path string) error {
	version, ok := intValue(raw["version"])
	if !ok {
		return fmt.Errorf("config.version is required in %s", path)
	}
	if version != currentConfigVersion {
		return fmt.Errorf("config.version in %s must be %d, got %d", path, currentConfigVersion, version)
	}
	return nil
}

func normalizeV2Config(raw map[string]any) (map[string]any, error) {
	version, ok := intValue(raw["version"])
	if !ok {
		return nil, fmt.Errorf("config.version is required")
	}
	if version != currentConfigVersion {
		return nil, fmt.Errorf("config.version must be %d, got %d", currentConfigVersion, version)
	}

	normalized := map[string]any{}
	if runtime, ok := stringMap(raw["runtime"]); ok {
		for _, section := range []string{"server", "auth", "store", "cache", "control_plane", "tools", "mcp", "pricing", "observability"} {
			if value, exists := runtime[section]; exists {
				normalized[section] = value
			}
		}
	}
	if routing, exists := raw["routing"]; exists {
		normalized["routing"] = routing
	}

	providers, err := normalizeV2Providers(raw["providers"])
	if err != nil {
		return nil, err
	}
	normalized["providers"] = providers
	return normalized, nil
}

func normalizeV2Providers(value any) (map[string]any, error) {
	rawProviders, ok := stringMap(value)
	if !ok {
		if value == nil {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("providers must be a map")
	}

	modelCatalog, err := catalog.Default()
	if err != nil {
		return nil, fmt.Errorf("load provider model catalog: %w", err)
	}

	normalized := make(map[string]any, len(rawProviders))
	for providerName, rawProvider := range rawProviders {
		provider, ok := stringMap(rawProvider)
		if !ok {
			return nil, fmt.Errorf("providers.%s must be a map", providerName)
		}
		if enabled, exists, err := optionalBool(provider["enabled"], "providers."+providerName+".enabled"); err != nil {
			return nil, err
		} else if exists && !enabled {
			continue
		}

		normalizedProvider := map[string]any{}
		if credentials, ok := stringMap(provider["credentials"]); ok {
			for _, field := range []string{"api_key", "access_key_id", "access_key_secret", "session_token", "app_id", "speech_api_key", "speech_access_token", "secret_key", "project_name", "project_id", "location"} {
				if value, exists := credentials[field]; exists {
					normalizedProvider[field] = value
				}
			}
		}
		if transport, ok := stringMap(provider["transport"]); ok {
			for _, field := range []string{"base_url", "control_base_url", "timeout", "retry"} {
				if value, exists := transport[field]; exists {
					normalizedProvider[field] = value
				}
			}
		}

		models, err := normalizeV2ProviderModels(providerName, provider["models"], modelCatalog)
		if err != nil {
			return nil, err
		}
		normalizedProvider["models"] = models
		normalized[providerName] = normalizedProvider
	}

	return normalized, nil
}

func normalizeV2ProviderModels(providerName string, value any, modelCatalog *catalog.Catalog) (map[string]any, error) {
	rawModels, ok := stringMap(value)
	if !ok {
		return nil, fmt.Errorf("providers.%s.models must be a map with use and optional overrides", providerName)
	}

	usedModels, err := stringSlice(rawModels["use"], "providers."+providerName+".models.use")
	if err != nil {
		return nil, err
	}
	overrides, err := modelOverrideMap(rawModels["overrides"], providerName)
	if err != nil {
		return nil, err
	}
	if len(usedModels) == 0 && len(overrides) > 0 {
		for modelName := range overrides {
			usedModels = append(usedModels, modelName)
		}
		slices.Sort(usedModels)
	}

	normalized := make(map[string]any, len(usedModels))
	for _, modelName := range usedModels {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			return nil, fmt.Errorf("providers.%s.models.use must not contain empty values", providerName)
		}

		model := map[string]any{}
		if entry, ok := modelCatalog.Lookup(providerName + "/" + modelName); ok {
			model = catalogModelConfig(entry)
		}
		if override, ok := overrides[modelName]; ok {
			model = mergeMaps(model, override)
		}
		if _, ok := model["modality"]; !ok {
			return nil, fmt.Errorf("providers.%s.models.%s is not in the provider catalog and must define overrides.%s.modality", providerName, modelName, modelName)
		}
		normalized[modelName] = model
	}

	return normalized, nil
}

func catalogModelConfig(entry catalog.Entry) map[string]any {
	model := map[string]any{
		"modality":     entry.Modality,
		"capabilities": entry.Capabilities,
	}
	if entry.ContextWindow > 0 {
		model["context_window"] = entry.ContextWindow
	}
	if entry.MaxOutputTokens > 0 {
		model["max_output_tokens"] = entry.MaxOutputTokens
	}
	if len(entry.OutputFormats) > 0 {
		model["output_formats"] = entry.OutputFormats
	}
	if entry.MinDurationMs > 0 {
		model["min_duration_ms"] = entry.MinDurationMs
	}
	if entry.MaxDurationMs > 0 {
		model["max_duration_ms"] = entry.MaxDurationMs
	}
	if len(entry.SampleRatesHz) > 0 {
		model["sample_rates_hz"] = entry.SampleRatesHz
	}
	if entry.Dimensions > 0 {
		model["dimensions"] = entry.Dimensions
	}
	if strings.TrimSpace(entry.Endpoint) != "" {
		model["endpoint"] = entry.Endpoint
	}
	if entry.MaxDuration > 0 {
		model["max_duration"] = entry.MaxDuration
	}
	if len(entry.AllowedDurations) > 0 {
		model["allowed_durations"] = entry.AllowedDurations
	}
	if len(entry.AspectRatios) > 0 {
		model["aspect_ratios"] = entry.AspectRatios
	}
	if len(entry.Resolutions) > 0 {
		model["resolutions"] = entry.Resolutions
	}
	if entry.Cancelable {
		model["cancelable"] = entry.Cancelable
	}
	if len(entry.Voices) > 0 {
		model["voices"] = entry.Voices
	}
	if len(entry.Formats) > 0 {
		model["formats"] = entry.Formats
	}
	if entry.SessionTTL > 0 {
		model["session_ttl"] = entry.SessionTTL
	}
	return model
}

func parseImports(value any, path string) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	imports, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("imports in %s must be a list", path)
	}

	out := make([]string, 0, len(imports))
	for index, rawImport := range imports {
		importPath, ok := rawImport.(string)
		if !ok || strings.TrimSpace(importPath) == "" {
			return nil, fmt.Errorf("imports[%d] in %s must be a non-empty string", index, path)
		}
		importPath = strings.TrimSpace(importPath)
		if !filepath.IsAbs(importPath) {
			importPath = filepath.Join(filepath.Dir(path), importPath)
		}
		out = append(out, filepath.Clean(importPath))
	}
	return out, nil
}

func modelOverrideMap(value any, providerName string) (map[string]map[string]any, error) {
	if value == nil {
		return map[string]map[string]any{}, nil
	}
	rawOverrides, ok := stringMap(value)
	if !ok {
		return nil, fmt.Errorf("providers.%s.models.overrides must be a map", providerName)
	}

	overrides := make(map[string]map[string]any, len(rawOverrides))
	for modelName, rawOverride := range rawOverrides {
		override, ok := stringMap(rawOverride)
		if !ok {
			return nil, fmt.Errorf("providers.%s.models.overrides.%s must be a map", providerName, modelName)
		}
		overrides[modelName] = override
	}
	return overrides, nil
}

func mergeMaps(base map[string]any, overlay map[string]any) map[string]any {
	merged := cloneMap(base)
	for key, overlayValue := range overlay {
		if overlayMap, ok := stringMap(overlayValue); ok {
			if baseMap, ok := stringMap(merged[key]); ok {
				merged[key] = mergeMaps(baseMap, overlayMap)
				continue
			}
			merged[key] = cloneMap(overlayMap)
			continue
		}
		merged[key] = overlayValue
	}
	return merged
}

func cloneMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		if nested, ok := stringMap(value); ok {
			out[key] = cloneMap(nested)
			continue
		}
		out[key] = value
	}
	return out
}

func stringMap(value any) (map[string]any, bool) {
	out, ok := value.(map[string]any)
	return out, ok
}

func stringSlice(value any, field string) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	values, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a list", field)
	}
	out := make([]string, 0, len(values))
	for index, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be a string", field, index)
		}
		out = append(out, text)
	}
	return out, nil
}

func intValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return 0, false
	}
}

func optionalBool(value any, field string) (bool, bool, error) {
	if value == nil {
		return false, false, nil
	}
	switch typed := value.(type) {
	case bool:
		return typed, true, nil
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		if err != nil {
			return false, true, fmt.Errorf("%s must be a boolean", field)
		}
		return parsed, true, nil
	default:
		return false, true, fmt.Errorf("%s must be a boolean", field)
	}
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}
