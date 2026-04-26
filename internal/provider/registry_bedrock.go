package provider

import (
	"fmt"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/bedrock"
)

func init() {
	registerProviderFamilyRegistrar("bedrock", registerBedrockProvider)
}

func registerBedrockProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := bedrock.NewClient(providerCfg)
	for modelName, modelCfg := range providerCfg.Models {
		if !runtimeSupportedModality(modelCfg.Modality) {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s in this runtime build", providerName, modelName, modelCfg.Modality))
			continue
		}
		if modelCfg.Modality != modality.ModalityChat && modelCfg.Modality != modality.ModalityEmbed {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s for this provider family", providerName, modelName, modelCfg.Modality))
			continue
		}

		id := fmt.Sprintf("%s/%s", providerName, modelName)
		registry.models[id] = modelFromConfig(id, providerName, modelName, modelCfg)
		switch modelCfg.Modality {
		case modality.ModalityChat:
			registry.chatAdapters[id] = bedrock.NewChatAdapter(client, id)
		case modality.ModalityEmbed:
			registry.embedAdapters[id] = bedrock.NewEmbedAdapter(client, id)
		}
	}
}
