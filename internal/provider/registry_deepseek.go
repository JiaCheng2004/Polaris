package provider

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/deepseek"
)

func init() {
	registerProviderFamilyRegistrar("deepseek", registerDeepSeekProvider)
}

func registerDeepSeekProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := deepseek.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return deepseek.NewChatAdapter(client, modelID)
	})
}
