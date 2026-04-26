package provider

import (
	"fmt"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	googlevertex "github.com/JiaCheng2004/Polaris/internal/provider/googlevertex"
)

func init() {
	registerProviderFamilyRegistrar("google-vertex", registerGoogleVertexProvider)
}

func registerGoogleVertexProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := googlevertex.NewClient(providerCfg)
	for modelName, modelCfg := range providerCfg.Models {
		if !runtimeSupportedModality(modelCfg.Modality) {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s in this runtime build", providerName, modelName, modelCfg.Modality))
			continue
		}
		id := fmt.Sprintf("%s/%s", providerName, modelName)
		registry.models[id] = modelFromConfig(id, providerName, modelName, modelCfg)
		if modelCfg.Modality == modality.ModalityVideo {
			registry.videoAdapters[id] = googlevertex.NewVideoAdapter(client, id)
		}
	}
}
