// Package core holds small, dependency-light helpers shared across provider
// adapters, the provider registry, and the verification tooling. It depends only
// on internal/modality (and stdlib), so it can be imported anywhere in the
// provider layer without creating an import cycle with the registry package.
package core

import (
	"encoding/json"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/modality"
)

// FirstNonEmpty returns the first value that is non-empty after trimming.
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// ProviderModelName derives the provider-native model name from a canonical
// "provider/model" identifier, falling back to fallbackModel's suffix. In
// practice a canonical model always contains a "/", so the no-slash branches are
// defensive.
func ProviderModelName(requestModel string, fallbackModel string) string {
	if requestModel == "" {
		return strings.TrimPrefix(fallbackModel[strings.Index(fallbackModel, "/")+1:], "/")
	}
	if idx := strings.IndexByte(requestModel, '/'); idx >= 0 {
		return requestModel[idx+1:]
	}
	if fallbackModel != "" {
		if idx := strings.IndexByte(fallbackModel, '/'); idx >= 0 {
			return fallbackModel[idx+1:]
		}
	}
	return requestModel
}

// IntFromAny coerces a JSON-decoded numeric value into an int.
func IntFromAny(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	default:
		return 0
	}
}

// MergeCapabilities returns the union of two capability lists, preserving order
// and dropping duplicates.
func MergeCapabilities(a []modality.Capability, b []modality.Capability) []modality.Capability {
	out := make([]modality.Capability, 0, len(a)+len(b))
	for _, items := range [][]modality.Capability{a, b} {
		for _, capability := range items {
			if !ContainsCapability(out, capability) {
				out = append(out, capability)
			}
		}
	}
	return out
}

// ContainsCapability reports whether capabilities contains candidate.
func ContainsCapability(capabilities []modality.Capability, candidate modality.Capability) bool {
	for _, capability := range capabilities {
		if capability == candidate {
			return true
		}
	}
	return false
}

// ToSet builds a trimmed set from values.
func ToSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[strings.TrimSpace(value)] = struct{}{}
	}
	return out
}

// ToPriorityMap maps each trimmed value to its index (its priority rank).
func ToPriorityMap(values []string) map[string]int {
	out := make(map[string]int, len(values))
	for index, value := range values {
		out[strings.TrimSpace(value)] = index
	}
	return out
}

// SelectorRank returns the priority rank of providerName, or len(priority) when
// it is not present (sorting unranked providers to the tail).
func SelectorRank(providerName string, priority map[string]int) int {
	if rank, ok := priority[providerName]; ok {
		return rank
	}
	return len(priority)
}
