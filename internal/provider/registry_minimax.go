package provider

import (
	"fmt"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/minimax"
)

func init() {
	registerProviderFamilyRegistrar("minimax", registerMiniMaxProvider)
}

func registerMiniMaxProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := minimax.NewClient(providerCfg)
	for modelName, modelCfg := range providerCfg.Models {
		if !runtimeSupportedModality(modelCfg.Modality) {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s in this runtime build", providerName, modelName, modelCfg.Modality))
			continue
		}
		id := fmt.Sprintf("%s/%s", providerName, modelName)
		registry.models[id] = modelFromConfig(id, providerName, modelName, modelCfg)
		if modelCfg.Modality == modality.ModalityMusic {
			registry.musicAdapters[id] = minimax.NewMusicAdapter(client, id)
		}
	}
}
