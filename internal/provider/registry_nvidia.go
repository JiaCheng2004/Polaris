package provider

import (
	"fmt"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/nvidia"
)

func init() {
	registerProviderFamilyRegistrar("nvidia", registerNVIDIAProvider)
}

func registerNVIDIAProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := nvidia.NewClient(providerCfg)
	for modelName, modelCfg := range providerCfg.Models {
		if !runtimeSupportedModality(modelCfg.Modality) {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s in this runtime build", providerName, modelName, modelCfg.Modality))
			continue
		}
		if modelCfg.Modality != modality.ModalityChat && modelCfg.Modality != modality.ModalityEmbed {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s for this provider family", providerName, modelName, modelCfg.Modality))
			continue
		}

		normalizedModelName := nvidia.NormalizeModelName(modelName)
		id := fmt.Sprintf("%s/%s", providerName, normalizedModelName)
		registry.models[id] = modelFromConfig(id, providerName, normalizedModelName, modelCfg)
		switch modelCfg.Modality {
		case modality.ModalityChat:
			registry.chatAdapters[id] = nvidia.NewChatAdapter(client, id)
		case modality.ModalityEmbed:
			registry.embedAdapters[id] = nvidia.NewEmbedAdapter(client, id)
		}
		if normalizedModelName != modelName {
			shortID := fmt.Sprintf("%s/%s", providerName, modelName)
			if _, exists := registry.aliases[shortID]; !exists {
				registry.aliases[shortID] = id
			}
		}
	}
}
