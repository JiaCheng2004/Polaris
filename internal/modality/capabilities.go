package modality

import "errors"

type Modality string

const (
	ModalityChat         Modality = "chat"
	ModalityImage        Modality = "image"
	ModalityVideo        Modality = "video"
	ModalityVoice        Modality = "voice"
	ModalityEmbed        Modality = "embed"
	ModalityAudio        Modality = "audio"
	ModalityInterpreting Modality = "interpreting"
	ModalityMusic        Modality = "music"
	ModalityTranslation  Modality = "translation"
	ModalityNotes        Modality = "notes"
	ModalityPodcast      Modality = "podcast"
	ModalityFiles        Modality = "files"
	ModalityBatch        Modality = "batch"
)

func (m Modality) Valid() bool {
	switch m {
	case ModalityChat, ModalityImage, ModalityVideo, ModalityVoice, ModalityEmbed, ModalityAudio, ModalityInterpreting, ModalityMusic, ModalityTranslation, ModalityNotes, ModalityPodcast, ModalityFiles, ModalityBatch:
		return true
	default:
		return false
	}
}

type Capability string

const (
	CapabilityVision                    Capability = "vision"
	CapabilityFunctionCalling           Capability = "function_calling"
	CapabilityStreaming                 Capability = "streaming"
	CapabilityJSONMode                  Capability = "json_mode"
	CapabilityAudioInput                Capability = "audio_input"
	CapabilityAudioOutput               Capability = "audio_output"
	CapabilityPDF                       Capability = "pdf"
	CapabilityPDFInput                  Capability = "pdf_input"
	CapabilityDocumentInput             Capability = "document_input"
	CapabilityFileReference             Capability = "file_reference"
	CapabilityExtendedThinking          Capability = "extended_thinking"
	CapabilityVideoInput                Capability = "video_input"
	CapabilityGeneration                Capability = "generation"
	CapabilityEditing                   Capability = "editing"
	CapabilityMultiReference            Capability = "multi_reference"
	CapabilityTTS                       Capability = "tts"
	CapabilitySTT                       Capability = "stt"
	CapabilityVoiceCloning              Capability = "voice_cloning"
	CapabilityVoiceDesign               Capability = "voice_design"
	CapabilityTextToVideo               Capability = "text_to_video"
	CapabilityImageToVideo              Capability = "image_to_video"
	CapabilityLastFrame                 Capability = "last_frame"
	CapabilityReferenceImages           Capability = "reference_images"
	CapabilityNativeAudio               Capability = "native_audio"
	CapabilityReasoning                 Capability = "reasoning"
	CapabilityHostedToolWebSearch       Capability = "hosted_tool_web_search"
	CapabilityHostedToolCodeInterpreter Capability = "hosted_tool_code_interpreter"
	CapabilityHostedToolComputerUse     Capability = "hosted_tool_computer_use"
	CapabilityHostedToolFileSearch      Capability = "hosted_tool_file_search"
	CapabilityHostedToolImageGeneration Capability = "hosted_tool_image_generation"
	CapabilityHostedToolMCP             Capability = "hosted_tool_mcp"
	CapabilityHostedToolURLContext      Capability = "hosted_tool_url_context"
	CapabilityAudioNotes                Capability = "audio_notes"
	CapabilityPodcastGeneration         Capability = "podcast_generation"
	CapabilityMusicGeneration           Capability = "music_generation"
	CapabilityMusicStreaming            Capability = "music_streaming"
	CapabilityMusicEditing              Capability = "music_editing"
	CapabilityMusicExtension            Capability = "music_extension"
	CapabilityMusicCover                Capability = "music_cover"
	CapabilityMusicInpainting           Capability = "music_inpainting"
	CapabilityMusicStems                Capability = "music_stems"
	CapabilityLyricsGeneration          Capability = "lyrics_generation"
	CapabilityCompositionPlans          Capability = "composition_plans"
	CapabilityInstrumental              Capability = "instrumental"
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
		CapabilityPDFInput,
		CapabilityDocumentInput,
		CapabilityFileReference,
		CapabilityExtendedThinking,
		CapabilityVideoInput,
		CapabilityGeneration,
		CapabilityEditing,
		CapabilityMultiReference,
		CapabilityTTS,
		CapabilitySTT,
		CapabilityVoiceCloning,
		CapabilityVoiceDesign,
		CapabilityTextToVideo,
		CapabilityImageToVideo,
		CapabilityLastFrame,
		CapabilityReferenceImages,
		CapabilityNativeAudio,
		CapabilityReasoning,
		CapabilityHostedToolWebSearch,
		CapabilityHostedToolCodeInterpreter,
		CapabilityHostedToolComputerUse,
		CapabilityHostedToolFileSearch,
		CapabilityHostedToolImageGeneration,
		CapabilityHostedToolMCP,
		CapabilityHostedToolURLContext,
		CapabilityAudioNotes,
		CapabilityPodcastGeneration,
		CapabilityMusicGeneration,
		CapabilityMusicStreaming,
		CapabilityMusicEditing,
		CapabilityMusicExtension,
		CapabilityMusicCover,
		CapabilityMusicInpainting,
		CapabilityMusicStems,
		CapabilityLyricsGeneration,
		CapabilityCompositionPlans,
		CapabilityInstrumental:
		return true
	default:
		return false
	}
}

var ErrCapabilityNotSupported = errors.New("capability not supported")
