package handler

import "github.com/JiaCheng2004/Polaris/internal/modality"

func providerUsageSource(usage modality.Usage) modality.TokenCountSource {
	if usage.Source != "" {
		return usage.Source
	}
	if usage.PromptTokens > 0 || usage.CompletionTokens > 0 || usage.TotalTokens > 0 {
		return modality.TokenCountSourceProviderReported
	}
	return modality.TokenCountSourceUnavailable
}

func countsTokenSource(promptTokens int, completionTokens int, totalTokens int) modality.TokenCountSource {
	if promptTokens > 0 || completionTokens > 0 || totalTokens > 0 {
		return modality.TokenCountSourceProviderReported
	}
	return modality.TokenCountSourceUnavailable
}

func normalizeUsage(usage modality.Usage) modality.Usage {
	usage.Source = providerUsageSource(usage)
	return usage
}

func normalizeEmbedUsage(usage modality.EmbedUsage) modality.EmbedUsage {
	if usage.Source == "" {
		usage.Source = countsTokenSource(usage.PromptTokens, 0, usage.TotalTokens)
	}
	return usage
}

func normalizeAudioUsage(usage modality.AudioUsage) modality.AudioUsage {
	if usage.Source == "" {
		usage.Source = countsTokenSource(usage.InputTextTokens, usage.OutputTextTokens, usage.TotalTokens)
	}
	return usage
}
