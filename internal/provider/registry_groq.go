package provider

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/groq"
)

func init() {
	registerProviderFamilyRegistrar("groq", registerGroqProvider)
}

func registerGroqProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := groq.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return groq.NewChatAdapter(client, modelID)
	})
}
