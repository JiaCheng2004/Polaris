package provider

import (
	"fmt"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/anthropic"
	"github.com/JiaCheng2004/Polaris/internal/provider/bedrock"
	"github.com/JiaCheng2004/Polaris/internal/provider/bytedance"
	"github.com/JiaCheng2004/Polaris/internal/provider/deepseek"
	"github.com/JiaCheng2004/Polaris/internal/provider/elevenlabs"
	"github.com/JiaCheng2004/Polaris/internal/provider/featherless"
	"github.com/JiaCheng2004/Polaris/internal/provider/fireworks"
	"github.com/JiaCheng2004/Polaris/internal/provider/glm"
	"github.com/JiaCheng2004/Polaris/internal/provider/google"
	googlevertex "github.com/JiaCheng2004/Polaris/internal/provider/googlevertex"
	"github.com/JiaCheng2004/Polaris/internal/provider/groq"
	"github.com/JiaCheng2004/Polaris/internal/provider/minimax"
	"github.com/JiaCheng2004/Polaris/internal/provider/mistral"
	"github.com/JiaCheng2004/Polaris/internal/provider/moonshot"
	"github.com/JiaCheng2004/Polaris/internal/provider/nvidia"
	"github.com/JiaCheng2004/Polaris/internal/provider/ollama"
	"github.com/JiaCheng2004/Polaris/internal/provider/openai"
	"github.com/JiaCheng2004/Polaris/internal/provider/openrouter"
	"github.com/JiaCheng2004/Polaris/internal/provider/qwen"
	"github.com/JiaCheng2004/Polaris/internal/provider/replicate"
	"github.com/JiaCheng2004/Polaris/internal/provider/together"
	"github.com/JiaCheng2004/Polaris/internal/provider/xai"
)

type providerRegistrar func(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig)

var providerFamilyRegistrars = map[string]providerRegistrar{
	"openai":        registerOpenAIProvider,
	"anthropic":     registerAnthropicProvider,
	"deepseek":      registerDeepSeekProvider,
	"bytedance":     registerByteDanceProvider,
	"google":        registerGoogleProvider,
	"google-vertex": registerGoogleVertexProvider,
	"minimax":       registerMiniMaxProvider,
	"elevenlabs":    registerElevenLabsProvider,
	"xai":           registerXAIProvider,
	"qwen":          registerQwenProvider,
	"ollama":        registerOllamaProvider,
	"openrouter":    registerOpenRouterProvider,
	"together":      registerTogetherProvider,
	"groq":          registerGroqProvider,
	"fireworks":     registerFireworksProvider,
	"featherless":   registerFeatherlessProvider,
	"moonshot":      registerMoonshotProvider,
	"glm":           registerGLMProvider,
	"mistral":       registerMistralProvider,
	"bedrock":       registerBedrockProvider,
	"nvidia":        registerNVIDIAProvider,
	"replicate":     registerReplicateProvider,
}

func supportedProviderFamilies() []string {
	names := make([]string, 0, len(providerFamilyRegistrars))
	for name := range providerFamilyRegistrars {
		names = append(names, name)
	}
	return names
}

func registerProviderFamily(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) bool {
	registrar, ok := providerFamilyRegistrars[providerName]
	if !ok {
		return false
	}
	registrar(registry, warnings, providerName, providerCfg)
	return true
}

func ProviderRuntimeEnabled(name string, providerCfg config.ProviderConfig) bool {
	switch name {
	case "bedrock":
		return strings.TrimSpace(providerCfg.AccessKeyID) != "" &&
			strings.TrimSpace(providerCfg.AccessKeySecret) != "" &&
			strings.TrimSpace(providerCfg.Location) != ""
	case "bytedance":
		return strings.TrimSpace(providerCfg.APIKey) != "" ||
			(strings.TrimSpace(providerCfg.AccessKeyID) != "" && strings.TrimSpace(providerCfg.AccessKeySecret) != "") ||
			strings.TrimSpace(providerCfg.SpeechAPIKey) != "" ||
			(strings.TrimSpace(providerCfg.AppID) != "" && strings.TrimSpace(providerCfg.SpeechAccessToken) != "")
	case "google-vertex":
		return strings.TrimSpace(providerCfg.ProjectID) != "" && strings.TrimSpace(providerCfg.Location) != "" && strings.TrimSpace(providerCfg.SecretKey) != ""
	case "ollama":
		return true
	default:
		return strings.TrimSpace(providerCfg.APIKey) != ""
	}
}

func providerEnabled(name string, providerCfg config.ProviderConfig) bool {
	return ProviderRuntimeEnabled(name, providerCfg)
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

func registerAnthropicProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := anthropic.NewClient(providerCfg)
	for modelName, modelCfg := range providerCfg.Models {
		if !runtimeSupportedModality(modelCfg.Modality) {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s in this runtime build", providerName, modelName, modelCfg.Modality))
			continue
		}
		id := fmt.Sprintf("%s/%s", providerName, modelName)
		registry.models[id] = modelFromConfig(id, providerName, modelName, modelCfg)
		if modelCfg.Modality == modality.ModalityChat {
			registry.chatAdapters[id] = anthropic.NewChatAdapter(client, id, modelCfg.MaxOutputTokens)
		}
	}
}

func registerDeepSeekProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := deepseek.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return deepseek.NewChatAdapter(client, modelID)
	})
}

func registerByteDanceProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := bytedance.NewClient(providerCfg)
	if strings.TrimSpace(providerCfg.AccessKeyID) != "" && strings.TrimSpace(providerCfg.AccessKeySecret) != "" {
		registry.voiceCatalogAdapters[providerName] = bytedance.NewVoiceCatalogAdapter(client)
		registry.voiceAssetAdapters[providerName] = bytedance.NewVoiceAssetAdapter(client)
	}
	for modelName, modelCfg := range providerCfg.Models {
		if !runtimeSupportedModality(modelCfg.Modality) {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s in this runtime build", providerName, modelName, modelCfg.Modality))
			continue
		}
		if (modelCfg.Modality == modality.ModalityChat || modelCfg.Modality == modality.ModalityImage || modelCfg.Modality == modality.ModalityVideo) && strings.TrimSpace(providerCfg.APIKey) == "" {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because providers.%s.api_key is required", providerName, modelName, providerName))
			continue
		}
		if modelCfg.Modality == modality.ModalityTranslation && strings.TrimSpace(providerCfg.SpeechAPIKey) == "" {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because providers.%s.speech_api_key is required for ByteDance translation", providerName, modelName, providerName))
			continue
		}
		if modelCfg.Modality == modality.ModalityPodcast {
			if strings.TrimSpace(providerCfg.AppID) == "" {
				*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because providers.%s.app_id is required for ByteDance podcast generation", providerName, modelName, providerName))
				continue
			}
			if strings.TrimSpace(providerCfg.SpeechAccessToken) == "" {
				*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because providers.%s.speech_access_token is required for ByteDance podcast generation", providerName, modelName, providerName))
				continue
			}
		}
		if modelCfg.Modality == modality.ModalityNotes {
			if strings.TrimSpace(providerCfg.AppID) == "" {
				*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because providers.%s.app_id is required for ByteDance audio notes", providerName, modelName, providerName))
				continue
			}
			if strings.TrimSpace(providerCfg.SpeechAccessToken) == "" {
				*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because providers.%s.speech_access_token is required for ByteDance audio notes", providerName, modelName, providerName))
				continue
			}
		}
		if modelCfg.Modality == modality.ModalityInterpreting {
			if strings.TrimSpace(providerCfg.AppID) == "" {
				*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because providers.%s.app_id is required for ByteDance simultaneous interpretation", providerName, modelName, providerName))
				continue
			}
			if strings.TrimSpace(providerCfg.SpeechAccessToken) == "" {
				*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because providers.%s.speech_access_token is required for ByteDance simultaneous interpretation", providerName, modelName, providerName))
				continue
			}
		}
		if modelCfg.Modality == modality.ModalityVoice {
			if containsCapability(modelCfg.Capabilities, modality.CapabilityTTS) && strings.TrimSpace(providerCfg.SpeechAPIKey) == "" {
				*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because providers.%s.speech_api_key is required for ByteDance TTS", providerName, modelName, providerName))
				continue
			}
			if containsCapability(modelCfg.Capabilities, modality.CapabilitySTT) && strings.TrimSpace(providerCfg.SpeechAPIKey) == "" {
				*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because providers.%s.speech_api_key is required for ByteDance STT", providerName, modelName, providerName))
				continue
			}
			if containsCapability(modelCfg.Capabilities, modality.CapabilityStreaming) {
				if strings.TrimSpace(providerCfg.AppID) == "" {
					*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because providers.%s.app_id is required for ByteDance streaming transcription", providerName, modelName, providerName))
					continue
				}
				if strings.TrimSpace(providerCfg.SpeechAccessToken) == "" {
					*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because providers.%s.speech_access_token is required for ByteDance streaming transcription", providerName, modelName, providerName))
					continue
				}
			}
		}
		if modelCfg.Modality == modality.ModalityAudio {
			if strings.TrimSpace(modelCfg.RealtimeSession.Transport) != "" {
				switch strings.TrimSpace(modelCfg.RealtimeSession.Auth) {
				case "api_key":
					if strings.TrimSpace(providerCfg.SpeechAPIKey) == "" {
						*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because providers.%s.speech_api_key is required for ByteDance native realtime audio sessions with realtime_session.auth=api_key", providerName, modelName, providerName))
						continue
					}
				default:
					if strings.TrimSpace(providerCfg.AppID) == "" {
						*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because providers.%s.app_id is required for ByteDance native realtime audio sessions", providerName, modelName, providerName))
						continue
					}
					if strings.TrimSpace(providerCfg.SpeechAccessToken) == "" {
						*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because providers.%s.speech_access_token is required for ByteDance native realtime audio sessions", providerName, modelName, providerName))
						continue
					}
				}
			} else {
				if strings.TrimSpace(providerCfg.APIKey) == "" {
					*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because providers.%s.api_key is required for ByteDance cascaded audio sessions", providerName, modelName, providerName))
					continue
				}
				if strings.TrimSpace(providerCfg.AppID) == "" {
					*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because providers.%s.app_id is required for ByteDance cascaded audio sessions", providerName, modelName, providerName))
					continue
				}
				if strings.TrimSpace(providerCfg.SpeechAPIKey) == "" {
					*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because providers.%s.speech_api_key is required for ByteDance cascaded audio sessions", providerName, modelName, providerName))
					continue
				}
			}
		}
		id := fmt.Sprintf("%s/%s", providerName, modelName)
		registry.models[id] = modelFromConfig(id, providerName, modelName, modelCfg)
		switch modelCfg.Modality {
		case modality.ModalityChat:
			registry.chatAdapters[id] = bytedance.NewChatAdapter(client, id, modelCfg.Endpoint)
		case modality.ModalityImage:
			registry.imageAdapters[id] = bytedance.NewImageAdapter(client, id, modelCfg.Endpoint)
		case modality.ModalityVideo:
			registry.videoAdapters[id] = bytedance.NewVideoAdapter(client, id, modelCfg.Endpoint)
		case modality.ModalityTranslation:
			registry.translationAdapters[id] = bytedance.NewTranslationAdapter(client, id, modelCfg.Endpoint)
		case modality.ModalityNotes:
			registry.audioNotesAdapters[id] = bytedance.NewAudioNotesAdapter(client, id, modelCfg.Endpoint)
		case modality.ModalityPodcast:
			registry.podcastAdapters[id] = bytedance.NewPodcastAdapter(client, id, modelCfg.Endpoint)
		case modality.ModalityInterpreting:
			registry.interpretingAdapters[id] = bytedance.NewInterpretingAdapter(client, id, modelCfg.Endpoint)
		case modality.ModalityVoice:
			voiceAdapter := bytedance.NewVoiceAdapter(client, id, modelCfg.Endpoint)
			if containsCapability(modelCfg.Capabilities, modality.CapabilityTTS) || containsCapability(modelCfg.Capabilities, modality.CapabilitySTT) {
				registry.voiceAdapters[id] = voiceAdapter
			}
			if containsCapability(modelCfg.Capabilities, modality.CapabilityStreaming) {
				registry.streamingTranscriptionAdapters[id] = bytedance.NewStreamingTranscriptionAdapter(client, id, modelCfg.Endpoint)
			}
		case modality.ModalityAudio:
			adapter := bytedance.NewAudioAdapter(client, id, modelCfg)
			if adapter == nil {
				*warnings = append(*warnings, fmt.Sprintf("model %s/%s is disabled because a valid audio_pipeline or realtime_session configuration is required", providerName, modelName))
				delete(registry.models, id)
				continue
			}
			registry.audioAdapters[id] = adapter
		}
	}
}

func registerGoogleProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := google.NewClient(providerCfg)
	for modelName, modelCfg := range providerCfg.Models {
		if !runtimeSupportedModality(modelCfg.Modality) {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s in this runtime build", providerName, modelName, modelCfg.Modality))
			continue
		}
		id := fmt.Sprintf("%s/%s", providerName, modelName)
		registry.models[id] = modelFromConfig(id, providerName, modelName, modelCfg)
		switch modelCfg.Modality {
		case modality.ModalityChat:
			registry.chatAdapters[id] = google.NewChatAdapter(client, id)
		case modality.ModalityEmbed:
			registry.embedAdapters[id] = google.NewEmbedAdapter(client, id)
		case modality.ModalityImage:
			registry.imageAdapters[id] = google.NewImageAdapter(client, id)
		}
	}
}

func registerGoogleVertexProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := googlevertex.NewClient(providerCfg)
	for modelName, modelCfg := range providerCfg.Models {
		if !runtimeSupportedModality(modelCfg.Modality) {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s in this runtime build", providerName, modelName, modelCfg.Modality))
			continue
		}
		id := fmt.Sprintf("%s/%s", providerName, modelName)
		registry.models[id] = modelFromConfig(id, providerName, modelName, modelCfg)
		if modelCfg.Modality == modality.ModalityVideo {
			registry.videoAdapters[id] = googlevertex.NewVideoAdapter(client, id)
		}
	}
}

func registerMiniMaxProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := minimax.NewClient(providerCfg)
	for modelName, modelCfg := range providerCfg.Models {
		if !runtimeSupportedModality(modelCfg.Modality) {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s in this runtime build", providerName, modelName, modelCfg.Modality))
			continue
		}
		id := fmt.Sprintf("%s/%s", providerName, modelName)
		registry.models[id] = modelFromConfig(id, providerName, modelName, modelCfg)
		if modelCfg.Modality == modality.ModalityMusic {
			registry.musicAdapters[id] = minimax.NewMusicAdapter(client, id)
		}
	}
}

func registerElevenLabsProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := elevenlabs.NewClient(providerCfg)
	for modelName, modelCfg := range providerCfg.Models {
		if !runtimeSupportedModality(modelCfg.Modality) {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s in this runtime build", providerName, modelName, modelCfg.Modality))
			continue
		}
		id := fmt.Sprintf("%s/%s", providerName, modelName)
		registry.models[id] = modelFromConfig(id, providerName, modelName, modelCfg)
		if modelCfg.Modality == modality.ModalityMusic {
			registry.musicAdapters[id] = elevenlabs.NewMusicAdapter(client, id)
		}
	}
}

func registerXAIProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := xai.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return xai.NewChatAdapter(client, modelID)
	})
}

func registerQwenProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := qwen.NewClient(providerCfg)
	for modelName, modelCfg := range providerCfg.Models {
		if !runtimeSupportedModality(modelCfg.Modality) {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s in this runtime build", providerName, modelName, modelCfg.Modality))
			continue
		}
		id := fmt.Sprintf("%s/%s", providerName, modelName)
		registry.models[id] = modelFromConfig(id, providerName, modelName, modelCfg)
		switch modelCfg.Modality {
		case modality.ModalityChat:
			registry.chatAdapters[id] = qwen.NewChatAdapter(client, id)
		case modality.ModalityImage:
			registry.imageAdapters[id] = qwen.NewImageAdapter(client, id)
		}
	}
}

func registerOllamaProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := ollama.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return ollama.NewChatAdapter(client, modelID)
	})
}

func registerOpenRouterProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := openrouter.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return openrouter.NewChatAdapter(client, modelID)
	})
}

func registerTogetherProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := together.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return together.NewChatAdapter(client, modelID)
	})
}

func registerGroqProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := groq.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return groq.NewChatAdapter(client, modelID)
	})
}

func registerFireworksProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := fireworks.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return fireworks.NewChatAdapter(client, modelID)
	})
}

func registerFeatherlessProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := featherless.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return featherless.NewChatAdapter(client, modelID)
	})
}

func registerMoonshotProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := moonshot.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return moonshot.NewChatAdapter(client, modelID)
	})
}

func registerGLMProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := glm.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return glm.NewChatAdapter(client, modelID)
	})
}

func registerMistralProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := mistral.NewClient(providerCfg)
	registerChatOnlyModels(registry, warnings, providerName, providerCfg.Models, func(modelID string) modality.ChatAdapter {
		return mistral.NewChatAdapter(client, modelID)
	})
}

func registerBedrockProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := bedrock.NewClient(providerCfg)
	for modelName, modelCfg := range providerCfg.Models {
		if !runtimeSupportedModality(modelCfg.Modality) {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s in this runtime build", providerName, modelName, modelCfg.Modality))
			continue
		}
		if modelCfg.Modality != modality.ModalityChat && modelCfg.Modality != modality.ModalityEmbed {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s for this provider family", providerName, modelName, modelCfg.Modality))
			continue
		}

		id := fmt.Sprintf("%s/%s", providerName, modelName)
		registry.models[id] = modelFromConfig(id, providerName, modelName, modelCfg)
		switch modelCfg.Modality {
		case modality.ModalityChat:
			registry.chatAdapters[id] = bedrock.NewChatAdapter(client, id)
		case modality.ModalityEmbed:
			registry.embedAdapters[id] = bedrock.NewEmbedAdapter(client, id)
		}
	}
}

func registerNVIDIAProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := nvidia.NewClient(providerCfg)
	for modelName, modelCfg := range providerCfg.Models {
		if !runtimeSupportedModality(modelCfg.Modality) {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s in this runtime build", providerName, modelName, modelCfg.Modality))
			continue
		}
		if modelCfg.Modality != modality.ModalityChat && modelCfg.Modality != modality.ModalityEmbed {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s for this provider family", providerName, modelName, modelCfg.Modality))
			continue
		}

		normalizedModelName := nvidia.NormalizeModelName(modelName)
		id := fmt.Sprintf("%s/%s", providerName, normalizedModelName)
		registry.models[id] = modelFromConfig(id, providerName, normalizedModelName, modelCfg)
		switch modelCfg.Modality {
		case modality.ModalityChat:
			registry.chatAdapters[id] = nvidia.NewChatAdapter(client, id)
		case modality.ModalityEmbed:
			registry.embedAdapters[id] = nvidia.NewEmbedAdapter(client, id)
		}
		if normalizedModelName != modelName {
			shortID := fmt.Sprintf("%s/%s", providerName, modelName)
			if _, exists := registry.aliases[shortID]; !exists {
				registry.aliases[shortID] = id
			}
		}
	}
}

func registerReplicateProvider(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) {
	client := replicate.NewClient(providerCfg)
	for modelName, modelCfg := range providerCfg.Models {
		if !runtimeSupportedModality(modelCfg.Modality) {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s in this runtime build", providerName, modelName, modelCfg.Modality))
			continue
		}
		if modelCfg.Modality != modality.ModalityVideo {
			*warnings = append(*warnings, fmt.Sprintf("model %s/%s uses unsupported modality %s for this provider family", providerName, modelName, modelCfg.Modality))
			continue
		}
		id := fmt.Sprintf("%s/%s", providerName, modelName)
		registry.models[id] = modelFromConfig(id, providerName, modelName, modelCfg)
		registry.videoAdapters[id] = replicate.NewVideoAdapter(client, id)
	}
}
