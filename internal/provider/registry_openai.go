package provider

import (
	"fmt"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/openai"
)

func init() {
	registerProviderFamilyRegistrar("openai", registerOpenAIProvider)
}

func registerOpenAIProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := openai.NewClient(providerCfg)
	for modelName, modelCfg := range providerCfg.Models {
		if !runtimeSupportedModality(modelCfg.Modality) {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s in this runtime build", providerName, modelName, modelCfg.Modality))
			continue
		}
		id := fmt.Sprintf("%s/%s", providerName, modelName)
		registry.models[id] = modelFromConfig(id, providerName, modelName, modelCfg)
		switch modelCfg.Modality {
		case modality.ModalityChat:
			registry.chatAdapters[id] = openai.NewChatAdapter(client, id)
		case modality.ModalityEmbed:
			registry.embedAdapters[id] = openai.NewEmbedAdapter(client, id)
		case modality.ModalityImage:
			registry.imageAdapters[id] = openai.NewImageAdapter(client, id)
		case modality.ModalityVideo:
			registry.videoAdapters[id] = openai.NewVideoAdapter(client, id)
		case modality.ModalityVoice:
			registry.voiceAdapters[id] = openai.NewVoiceAdapter(client, id)
		case modality.ModalityAudio:
			adapter := openai.NewAudioAdapter(client, id, modelCfg)
			if adapter == nil {
				*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because a supported realtime_session transport or audio_pipeline.chat_model, stt_model, and tts_model are required", providerName, modelName))
				delete(registry.models, id)
				continue
			}
			registry.audioAdapters[id] = adapter
		}
	}
}
