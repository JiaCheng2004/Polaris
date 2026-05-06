package modality

import "strings"

type ReasoningOptions struct {
	Effort         string `json:"effort,omitempty"`
	BudgetTokens   int    `json:"budget_tokens,omitempty"`
	IncludeSummary string `json:"include_summary,omitempty"`
	DisplayMode    string `json:"display_mode,omitempty"`
}

func ReasoningBudgetTokens(options *ReasoningOptions, model string) int {
	if options == nil {
		return 0
	}
	if options.BudgetTokens > 0 {
		return options.BudgetTokens
	}
	switch strings.ToLower(strings.TrimSpace(options.Effort)) {
	case "minimal":
		return 1024
	case "low":
		return 2048
	case "medium":
		return 4096
	case "high":
		return 8192
	default:
		return 0
	}
}

func ReasoningEffort(options *ReasoningOptions) string {
	if options == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(options.Effort)) {
	case "minimal", "low", "medium", "high":
		return strings.ToLower(strings.TrimSpace(options.Effort))
	default:
		return ""
	}
}
