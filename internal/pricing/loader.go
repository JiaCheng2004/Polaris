package pricing

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"gopkg.in/yaml.v3"
)

func Load(path string) (*Catalog, []string, error) {
	entries := map[string]Entry{}
	var sources []string
	var warnings []string

	bundled, err := fs.Glob(embeddedData, "data/*.yaml")
	if err != nil {
		return nil, nil, fmt.Errorf("list embedded pricing data: %w", err)
	}
	sort.Strings(bundled)
	for _, name := range bundled {
		data, err := embeddedData.ReadFile(name)
		if err != nil {
			return nil, warnings, fmt.Errorf("read embedded pricing data %s: %w", name, err)
		}
		parsed, err := parseFile(data, name)
		if err != nil {
			return nil, warnings, err
		}
		mergeEntries(entries, parsed.Models)
		sources = append(sources, name)
	}

	if strings.TrimSpace(path) != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, warnings, fmt.Errorf("read pricing file %s: %w", path, err)
		}
		expanded, envWarnings := config.ExpandEnv(string(data))
		warnings = append(warnings, envWarnings...)
		parsed, err := parseFile([]byte(expanded), path)
		if err != nil {
			return nil, warnings, err
		}
		mergeEntries(entries, parsed.Models)
		sources = append(sources, path)
	}

	return newCatalog(entries, time.Now().UTC(), sources), uniqueSorted(warnings), nil
}

func LoadBundled() (*Catalog, error) {
	catalog, _, err := Load("")
	return catalog, err
}

func parseFile(data []byte, source string) (*File, error) {
	var parsed File
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode pricing yaml %s: %w", source, err)
	}
	if err := validateFile(parsed); err != nil {
		return nil, fmt.Errorf("validate pricing yaml %s: %w", source, err)
	}
	normalizeFile(&parsed)
	return &parsed, nil
}

func mergeEntries(dst map[string]Entry, src map[string]Entry) {
	for key, entry := range src {
		dst[strings.TrimSpace(key)] = entry
	}
}

func validateFile(file File) error {
	if file.Version != 1 {
		return fmt.Errorf("version must be 1")
	}
	if len(file.Models) == 0 {
		return fmt.Errorf("models must not be empty")
	}
	for key, entry := range file.Models {
		if err := validateEntry(key, entry); err != nil {
			return fmt.Errorf("models.%s: %w", key, err)
		}
	}
	return nil
}

func validateEntry(key string, entry Entry) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("model key must not be empty")
	}
	if !strings.Contains(key, "/") {
		return fmt.Errorf("model key must use provider/model form")
	}
	if strings.Contains(key, "*") && !strings.HasSuffix(key, "*") {
		return fmt.Errorf("wildcard is only supported as a suffix")
	}
	if !validMode(entry.Mode) {
		return fmt.Errorf("mode %q is invalid", entry.Mode)
	}
	if entry.Pricing != nil && len(entry.TieredPricing) > 0 {
		return fmt.Errorf("pricing and tiered_pricing are mutually exclusive")
	}
	if entry.Pricing == nil && len(entry.TieredPricing) == 0 {
		return fmt.Errorf("pricing or tiered_pricing is required")
	}
	if entry.Pricing != nil {
		if err := validateRates(*entry.Pricing); err != nil {
			return fmt.Errorf("pricing: %w", err)
		}
	}
	for name, tier := range entry.Tiers {
		if !validTier(name) {
			return fmt.Errorf("tiers.%s is not a known tier", name)
		}
		if err := validateRates(tier); err != nil {
			return fmt.Errorf("tiers.%s: %w", name, err)
		}
	}
	for name, deployment := range entry.Deployments {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("deployment name must not be empty")
		}
		if err := validateRates(deployment); err != nil {
			return fmt.Errorf("deployments.%s: %w", name, err)
		}
	}
	for index, tier := range entry.TieredPricing {
		if tier.Range[0] < 0 || tier.Range[1] < 0 {
			return fmt.Errorf("tiered_pricing[%d].range must not be negative", index)
		}
		if tier.Range[1] > 0 && tier.Range[0] > tier.Range[1] {
			return fmt.Errorf("tiered_pricing[%d].range lower bound must be <= upper bound", index)
		}
		if err := validateRates(tier.Rates); err != nil {
			return fmt.Errorf("tiered_pricing[%d]: %w", index, err)
		}
	}
	for name, rate := range entry.AdditionalUnits {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("additional_units keys must not be empty")
		}
		if rate < 0 {
			return fmt.Errorf("additional_units.%s must not be negative", name)
		}
	}
	return nil
}

func validateRates(rates Rates) error {
	if rates.Multiplier < 0 {
		return fmt.Errorf("multiplier must not be negative")
	}
	for name, value := range map[string]float64{
		"input_per_mtok":              rates.InputPerMTok,
		"output_per_mtok":             rates.OutputPerMTok,
		"output_reasoning_per_mtok":   rates.OutputReasoningPerMTok,
		"cache_read_per_mtok":         rates.CacheReadPerMTok,
		"cache_write_5m_per_mtok":     rates.CacheWrite5mPerMTok,
		"cache_write_1h_per_mtok":     rates.CacheWrite1hPerMTok,
		"input_cache_hit_per_mtok":    rates.InputCacheHitPerMTok,
		"input_image_token_per_mtok":  rates.InputImageTokenPerMTok,
		"output_image_token_per_mtok": rates.OutputImageTokenPerMTok,
		"input_per_audio_second":      rates.InputPerAudioSecond,
		"output_per_audio_second":     rates.OutputPerAudioSecond,
		"input_per_video_second":      rates.InputPerVideoSecond,
		"output_per_video_second":     rates.OutputPerVideoSecond,
		"input_per_character":         rates.InputPerCharacter,
		"input_per_image":             rates.InputPerImage,
		"output_per_image":            rates.OutputPerImage,
		"output_per_pixel":            rates.OutputPerPixel,
		"per_call":                    rates.PerCall,
	} {
		if value < 0 {
			return fmt.Errorf("%s must not be negative", name)
		}
	}
	return nil
}

func normalizeFile(file *File) {
	for key, entry := range file.Models {
		if strings.TrimSpace(entry.Currency) == "" {
			entry.Currency = "USD"
		}
		file.Models[key] = entry
	}
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
