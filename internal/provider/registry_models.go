package provider

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func modelFromConfig(id, providerName, modelName string, modelCfg config.ModelConfig) Model {
	model := Model{
		ID:               id,
		Object:           "model",
		Kind:             "provider_variant",
		Provider:         providerName,
		ProviderVariant:  id,
		Name:             modelName,
		Modality:         modelCfg.Modality,
		Capabilities:     append([]modality.Capability(nil), modelCfg.Capabilities...),
		ContextWindow:    modelCfg.ContextWindow,
		MaxOutputTokens:  modelCfg.MaxOutputTokens,
		MaxDuration:      modelCfg.MaxDuration,
		AllowedDurations: append([]int(nil), modelCfg.AllowedDurations...),
		AspectRatios:     append([]string(nil), modelCfg.AspectRatios...),
		Resolutions:      append([]string(nil), modelCfg.Resolutions...),
		Cancelable:       modelCfg.Cancelable,
		Voices:           append([]string(nil), modelCfg.Voices...),
		Formats:          append([]string(nil), modelCfg.Formats...),
		OutputFormats:    append([]string(nil), modelCfg.OutputFormats...),
		MinDurationMs:    modelCfg.MinDurationMs,
		MaxDurationMs:    modelCfg.MaxDurationMs,
		SampleRatesHz:    append([]int(nil), modelCfg.SampleRatesHz...),
		Dimensions:       modelCfg.Dimensions,
		SessionTTL:       int64(modelCfg.SessionTTL.Seconds()),
	}
	if entry, ok := lookupModelCatalog(id); ok {
		model.DisplayName = entry.DisplayName
		model.FamilyID = entry.FamilyID
		model.FamilyDisplayName = entry.FamilyDisplayName
		model.Status = entry.Status
		model.VerificationClass = entry.VerificationClass
		model.CostTier = entry.CostTier
		model.LatencyTier = entry.LatencyTier
		model.DocURL = entry.DocURL
		model.LastVerified = entry.LastVerified
		model.FamilyPriority = entry.FamilyPriority
	}
	return model
}

func containsCapability(capabilities []modality.Capability, candidate modality.Capability) bool {
	for _, capability := range capabilities {
		if capability == candidate {
			return true
		}
	}
	return false
}

func runtimeSupportedModality(candidate modality.Modality) bool {
	switch candidate {
	case modality.ModalityChat, modality.ModalityImage, modality.ModalityVideo, modality.ModalityVoice, modality.ModalityEmbed, modality.ModalityAudio, modality.ModalityInterpreting, modality.ModalityMusic, modality.ModalityTranslation, modality.ModalityNotes, modality.ModalityPodcast:
		return true
	default:
		return false
	}
}
