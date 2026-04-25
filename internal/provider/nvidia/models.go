package nvidia

import "strings"

func NormalizeModelName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "nvidia/") {
		return trimmed
	}
	return "nvidia/" + trimmed
}
