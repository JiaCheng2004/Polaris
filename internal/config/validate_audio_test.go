package config

import (
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestValidateRejectsAudioModelWithoutPipelineOrRealtimeSession(t *testing.T) {
	cfg := Default()
	cfg.Providers["openai"] = ProviderConfig{
		APIKey:  "sk-openai",
		BaseURL: "https://api.openai.com/v1",
		Timeout: time.Minute,
		Models: map[string]ModelConfig{
			"gpt-4o-audio": {
				Modality:     modality.ModalityAudio,
				Capabilities: []modality.Capability{modality.CapabilityAudioInput, modality.CapabilityAudioOutput},
				Voices:       []string{"nova"},
			},
		},
	}

	err := Validate(&cfg)
	if err == nil || !strings.Contains(err.Error(), "must define either audio_pipeline.chat_model, stt_model, and tts_model or realtime_session.transport") {
		t.Fatalf("expected audio pipeline validation error, got %v", err)
	}
}

func TestValidateAcceptsAudioModelWithPipeline(t *testing.T) {
	cfg := Default()
	cfg.Providers["openai"] = ProviderConfig{
		APIKey:  "sk-openai",
		BaseURL: "https://api.openai.com/v1",
		Timeout: time.Minute,
		Models: map[string]ModelConfig{
			"gpt-4o": {
				Modality:     modality.ModalityChat,
				Capabilities: []modality.Capability{modality.CapabilityStreaming},
			},
			"tts-1": {
				Modality:     modality.ModalityVoice,
				Capabilities: []modality.Capability{modality.CapabilityTTS},
				Voices:       []string{"nova"},
			},
			"whisper-1": {
				Modality:     modality.ModalityVoice,
				Capabilities: []modality.Capability{modality.CapabilitySTT},
				Formats:      []string{"wav"},
			},
			"gpt-4o-audio": {
				Modality:     modality.ModalityAudio,
				Capabilities: []modality.Capability{modality.CapabilityAudioInput, modality.CapabilityAudioOutput},
				Voices:       []string{"nova"},
				SessionTTL:   10 * time.Minute,
				AudioPipeline: AudioPipelineConfig{
					ChatModel: "openai/gpt-4o",
					STTModel:  "openai/whisper-1",
					TTSModel:  "openai/tts-1",
				},
			},
		},
	}

	if err := Validate(&cfg); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateAcceptsOpenAIAudioModelWithRealtimeSession(t *testing.T) {
	cfg := Default()
	cfg.Providers["openai"] = ProviderConfig{
		APIKey:  "sk-openai",
		BaseURL: "https://api.openai.com/v1",
		Timeout: time.Minute,
		Models: map[string]ModelConfig{
			"gpt-4o-audio": {
				Modality:     modality.ModalityAudio,
				Capabilities: []modality.Capability{modality.CapabilityAudioInput, modality.CapabilityAudioOutput},
				Voices:       []string{"nova"},
				SessionTTL:   10 * time.Minute,
				RealtimeSession: AudioRealtimeConfig{
					Transport: "openai_realtime",
					Model:     "gpt-realtime",
				},
			},
		},
	}

	if err := Validate(&cfg); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateAcceptsAudioModelWithRealtimeSession(t *testing.T) {
	cfg := Default()
	cfg.Providers["bytedance"] = ProviderConfig{
		AppID:             "app-123",
		SpeechAccessToken: "speech-token",
		SpeechAPIKey:      "speech-api-key",
		BaseURL:           "https://ark.cn-beijing.volces.com/api/v3",
		Timeout:           time.Minute,
		Models: map[string]ModelConfig{
			"doubao-audio": {
				Modality:     modality.ModalityAudio,
				Capabilities: []modality.Capability{modality.CapabilityAudioInput, modality.CapabilityAudioOutput},
				Voices:       []string{"zh_female_vv_jupiter_bigtts"},
				SessionTTL:   10 * time.Minute,
				RealtimeSession: AudioRealtimeConfig{
					Transport: "bytedance_dialog",
					Auth:      "access_token",
					Model:     "1.2.1.1",
				},
			},
		},
	}

	if err := Validate(&cfg); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
