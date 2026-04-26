package provider

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/openrouter"
)

func init() {
	registerProviderFamilyRegistrar("openrouter", registerOpenRouterProvider)
}

func registerOpenRouterProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := openrouter.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return openrouter.NewChatAdapter(client, modelID)
	})
}
