package openai

import (
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/audiohelper"
)

func NewAudioAdapter(client *Client, model string, modelCfg config.ModelConfig) modality.AudioAdapter {
	if client == nil {
		return nil
	}
	if adapter := newRealtimeAudioAdapter(client, model, modelCfg); adapter != nil {
		return adapter
	}
	pipeline := modelCfg.AudioPipeline
	chatModel := canonicalAudioPipelineModel("openai", pipeline.ChatModel)
	sttModel := canonicalAudioPipelineModel("openai", pipeline.STTModel)
	ttsModel := canonicalAudioPipelineModel("openai", pipeline.TTSModel)
	if chatModel == "" || sttModel == "" || ttsModel == "" {
		return nil
	}

	chatAdapter := NewChatAdapter(client, chatModel)
	sttAdapter := NewVoiceAdapter(client, sttModel)
	ttsAdapter := NewVoiceAdapter(client, ttsModel)

	return audiohelper.NewCascadeAdapter(
		model,
		chatModel,
		sttModel,
		ttsModel,
		chatAdapter,
		sttAdapter.SpeechToText,
		ttsAdapter.TextToSpeech,
		[]string{modality.TurnDetectionManual, modality.TurnDetectionServerVAD},
		10*time.Minute,
	)
}

func canonicalAudioPipelineModel(providerName string, model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	if strings.Contains(model, "/") {
		return model
	}
	return providerName + "/" + model
}
