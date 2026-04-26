package provider

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/glm"
)

func init() {
	registerProviderFamilyRegistrar("glm", registerGLMProvider)
}

func registerGLMProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := glm.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return glm.NewChatAdapter(client, modelID)
	})
}
