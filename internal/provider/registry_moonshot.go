package provider

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/moonshot"
)

func init() {
	registerProviderFamilyRegistrar("moonshot", registerMoonshotProvider)
}

func registerMoonshotProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := moonshot.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return moonshot.NewChatAdapter(client, modelID)
	})
}
