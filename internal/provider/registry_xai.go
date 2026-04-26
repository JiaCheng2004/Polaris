package provider

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/xai"
)

func init() {
	registerProviderFamilyRegistrar("xai", registerXAIProvider)
}

func registerXAIProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := xai.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return xai.NewChatAdapter(client, modelID)
	})
}
