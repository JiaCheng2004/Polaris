package modality

import "errors"

type Modality string

const (
	ModalityChat  Modality = "chat"
	ModalityImage Modality = "image"
	ModalityVideo Modality = "video"
	ModalityVoice Modality = "voice"
	ModalityEmbed Modality = "embed"
	ModalityAudio Modality = "audio"
)

func (m Modality) Valid() bool {
	switch m {
	case ModalityChat, ModalityImage, ModalityVideo, ModalityVoice, ModalityEmbed, ModalityAudio:
		return true
	default:
		return false
	}
}

type Capability string

const (
	CapabilityVision           Capability = "vision"
	CapabilityFunctionCalling  Capability = "function_calling"
	CapabilityStreaming        Capability = "streaming"
	CapabilityJSONMode         Capability = "json_mode"
	CapabilityAudioInput       Capability = "audio_input"
	CapabilityAudioOutput      Capability = "audio_output"
	CapabilityPDF              Capability = "pdf"
	CapabilityExtendedThinking Capability = "extended_thinking"
	CapabilityVideoInput       Capability = "video_input"
	CapabilityGeneration       Capability = "generation"
	CapabilityEditing          Capability = "editing"
	CapabilityMultiReference   Capability = "multi_reference"
	CapabilityTTS              Capability = "tts"
	CapabilitySTT              Capability = "stt"
	CapabilityVoiceCloning     Capability = "voice_cloning"
	CapabilityTextToVideo      Capability = "text_to_video"
	CapabilityImageToVideo     Capability = "image_to_video"
	CapabilityReferenceImages  Capability = "reference_images"
	CapabilityNativeAudio      Capability = "native_audio"
	CapabilityReasoning        Capability = "reasoning"
)

func (c Capability) Valid() bool {
	switch c {
	case CapabilityVision,
		CapabilityFunctionCalling,
		CapabilityStreaming,
		CapabilityJSONMode,
		CapabilityAudioInput,
		CapabilityAudioOutput,
		CapabilityPDF,
		CapabilityExtendedThinking,
		CapabilityVideoInput,
		CapabilityGeneration,
		CapabilityEditing,
		CapabilityMultiReference,
		CapabilityTTS,
		CapabilitySTT,
		CapabilityVoiceCloning,
		CapabilityTextToVideo,
		CapabilityImageToVideo,
		CapabilityReferenceImages,
		CapabilityNativeAudio,
		CapabilityReasoning:
		return true
	default:
		return false
	}
}

var ErrCapabilityNotSupported = errors.New("capability not supported")
