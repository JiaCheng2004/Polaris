package provider

import (
	"fmt"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/google"
)

func init() {
	registerProviderFamilyRegistrar("google", registerGoogleProvider)
}

func registerGoogleProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := google.NewClient(providerCfg)
	for modelName, modelCfg := range providerCfg.Models {
		if !runtimeSupportedModality(modelCfg.Modality) {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s in this runtime build", providerName, modelName, modelCfg.Modality))
			continue
		}
		id := fmt.Sprintf("%s/%s", providerName, modelName)
		registry.models[id] = modelFromConfig(id, providerName, modelName, modelCfg)
		switch modelCfg.Modality {
		case modality.ModalityChat:
			registry.chatAdapters[id] = google.NewChatAdapter(client, id)
		case modality.ModalityEmbed:
			registry.embedAdapters[id] = google.NewEmbedAdapter(client, id)
		case modality.ModalityImage:
			registry.imageAdapters[id] = google.NewImageAdapter(client, id)
		}
	}
}
