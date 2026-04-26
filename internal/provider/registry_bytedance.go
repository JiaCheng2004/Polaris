package provider

import (
	"fmt"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/bytedance"
)

func init() {
	registerProviderFamilyRegistrar("bytedance", registerByteDanceProvider)
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
