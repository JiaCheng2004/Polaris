package provider

import (
	"fmt"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/minimaxtoken"
)

func init() {
	registerProviderFamilyRegistrar("minimax-token", registerMiniMaxTokenProvider)
}

func registerMiniMaxTokenProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := minimaxtoken.NewClient(providerCfg)
	for modelName, modelCfg := range providerCfg.Models {
		if !runtimeSupportedModality(modelCfg.Modality) {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s in this runtime build", providerName, modelName, modelCfg.Modality))
			continue
		}
		if modelCfg.Modality != modality.ModalityChat {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s for this provider family", providerName, modelName, modelCfg.Modality))
			continue
		}

		id := fmt.Sprintf("%s/%s", providerName, modelName)
		registry.models[id] = modelFromConfig(id, providerName, modelName, modelCfg)
		chatAdapter := minimaxtoken.NewChatAdapter(client, id, modelCfg.MaxOutputTokens)
		registry.chatAdapters[id] = chatAdapter
		registry.nativeMessagesAdapters[id] = minimaxtoken.NewNativeMessagesAdapter(client, id)
	}
}
