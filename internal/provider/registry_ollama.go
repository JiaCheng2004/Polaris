package provider

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/ollama"
)

func init() {
	registerProviderFamilyRegistrar("ollama", registerOllamaProvider)
}

func registerOllamaProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := ollama.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return ollama.NewChatAdapter(client, modelID)
	})
}
