package provider

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/mistral"
)

func init() {
	registerProviderFamilyRegistrar("mistral", registerMistralProvider)
}

func registerMistralProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := mistral.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return mistral.NewChatAdapter(client, modelID)
	})
}
