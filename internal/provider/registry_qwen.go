package provider

import (
	"fmt"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/qwen"
)

func init() {
	registerProviderFamilyRegistrar("qwen", registerQwenProvider)
}

func registerQwenProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := qwen.NewClient(providerCfg)
	for modelName, modelCfg := range providerCfg.Models {
		if !runtimeSupportedModality(modelCfg.Modality) {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s in this runtime build", providerName, modelName, modelCfg.Modality))
			continue
		}
		id := fmt.Sprintf("%s/%s", providerName, modelName)
		registry.models[id] = modelFromConfig(id, providerName, modelName, modelCfg)
		switch modelCfg.Modality {
		case modality.ModalityChat:
			registry.chatAdapters[id] = qwen.NewChatAdapter(client, id)
		case modality.ModalityImage:
			registry.imageAdapters[id] = qwen.NewImageAdapter(client, id)
		}
	}
}
