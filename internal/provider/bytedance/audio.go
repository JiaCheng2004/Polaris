package bytedance

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
	if strings.TrimSpace(modelCfg.RealtimeSession.Transport) != "" {
		return newRealtimeAudioAdapter(client, model, modelCfg)
	}
	return newCascadeAudioAdapter(client, model, modelCfg.AudioPipeline)
}

func newCascadeAudioAdapter(client *Client, model string, pipeline config.AudioPipelineConfig) *audiohelper.CascadeAdapter {
	chatModel := canonicalAudioPipelineModel("bytedance", pipeline.ChatModel)
	sttModel := canonicalAudioPipelineModel("bytedance", pipeline.STTModel)
	ttsModel := canonicalAudioPipelineModel("bytedance", pipeline.TTSModel)
	if chatModel == "" || sttModel == "" || ttsModel == "" {
		return nil
	}

	chatAdapter := NewChatAdapter(client, chatModel, "")
	sttAdapter := NewVoiceAdapter(client, sttModel, "")
	ttsAdapter := NewVoiceAdapter(client, ttsModel, "")

	return audiohelper.NewCascadeAdapter(
		model,
		chatModel,
		sttModel,
		ttsModel,
		chatAdapter,
		sttAdapter.SpeechToText,
		ttsAdapter.TextToSpeech,
		[]string{modality.TurnDetectionManual},
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
