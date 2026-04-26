package provider

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/fireworks"
)

func init() {
	registerProviderFamilyRegistrar("fireworks", registerFireworksProvider)
}

func registerFireworksProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := fireworks.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return fireworks.NewChatAdapter(client, modelID)
	})
}
