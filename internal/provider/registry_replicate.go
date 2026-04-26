package provider

import (
	"fmt"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/replicate"
)

func init() {
	registerProviderFamilyRegistrar("replicate", registerReplicateProvider)
}

func registerReplicateProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := replicate.NewClient(providerCfg)
	for modelName, modelCfg := range providerCfg.Models {
		if !runtimeSupportedModality(modelCfg.Modality) {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s in this runtime build", providerName, modelName, modelCfg.Modality))
			continue
		}
		if modelCfg.Modality != modality.ModalityVideo {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s for this provider family", providerName, modelName, modelCfg.Modality))
			continue
		}
		id := fmt.Sprintf("%s/%s", providerName, modelName)
		registry.models[id] = modelFromConfig(id, providerName, modelName, modelCfg)
		registry.videoAdapters[id] = replicate.NewVideoAdapter(client, id)
	}
}
