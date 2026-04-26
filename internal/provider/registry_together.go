package provider

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/together"
)

func init() {
	registerProviderFamilyRegistrar("together", registerTogetherProvider)
}

func registerTogetherProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := together.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return together.NewChatAdapter(client, modelID)
	})
}
