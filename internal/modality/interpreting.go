package modality

import "context"

const (
	InterpretingModeSpeechToSpeech = "speech_to_speech"
	InterpretingModeSpeechToText   = "speech_to_text"

	InterpretingAudioFormatPCM16 = "pcm16"
	InterpretingAudioFormatWAV   = "wav"
	InterpretingAudioFormatOpus  = "ogg_opus"

	InterpretingClientEventSessionUpdate    = "session.update"
	InterpretingClientEventInputAudioAppend = "input_audio.append"
	InterpretingClientEventInputAudioCommit = "input_audio.commit"
	InterpretingClientEventSessionClose     = "session.close"

	InterpretingServerEventSessionCreated          = "session.created"
	InterpretingServerEventSessionUpdated          = "session.updated"
	InterpretingServerEventInputAudioCommitted     = "input_audio.committed"
	InterpretingServerEventInputAudioMuted         = "input_audio.muted"
	InterpretingServerEventSourceTranscriptDelta   = "source_transcript.delta"
	InterpretingServerEventSourceTranscriptSegment = "source_transcript.segment"
	InterpretingServerEventTranslationDelta        = "translation.delta"
	InterpretingServerEventTranslationSegment      = "translation.segment"
	InterpretingServerEventResponseAudioDelta      = "response.audio.delta"
	InterpretingServerEventResponseAudioDone       = "response.audio.done"
	InterpretingServerEventResponseCompleted       = "response.completed"
	InterpretingServerEventError                   = "error"
)

type InterpretingAdapter interface {
	ConnectInterpreting(ctx context.Context, cfg *InterpretingSessionConfig) (InterpretingSession, error)
}

type InterpretingSession interface {
	Send(event InterpretingClientEvent) error
	Events() <-chan InterpretingServerEvent
	Close() error
}

type InterpretingSessionConfig struct {
	Model              string          `json:"model"`
	Routing            *RoutingOptions `json:"routing,omitempty"`
	Mode               string          `json:"mode,omitempty"`
	SourceLanguage     string          `json:"source_language,omitempty"`
	TargetLanguage     string          `json:"target_language,omitempty"`
	Voice              string          `json:"voice,omitempty"`
	InputAudioFormat   string          `json:"input_audio_format,omitempty"`
	OutputAudioFormat  string          `json:"output_audio_format,omitempty"`
	InputSampleRateHz  int             `json:"input_sample_rate_hz,omitempty"`
	OutputSampleRateHz int             `json:"output_sample_rate_hz,omitempty"`
	Denoise            *bool           `json:"denoise,omitempty"`
	Glossary           []GlossaryEntry `json:"glossary,omitempty"`
}

type GlossaryEntry struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type InterpretingSessionDescriptor struct {
	ID           string `json:"id"`
	Object       string `json:"object"`
	Model        string `json:"model"`
	ExpiresAt    int64  `json:"expires_at"`
	WebSocketURL string `json:"websocket_url"`
	ClientSecret string `json:"client_secret"`
}

type InterpretingClientEvent struct {
	Type    string                     `json:"type"`
	EventID string                     `json:"event_id,omitempty"`
	Session *InterpretingSessionConfig `json:"session,omitempty"`
	Audio   string                     `json:"audio,omitempty"`
}

type InterpretingServerEvent struct {
	Type       string                         `json:"type"`
	EventID    string                         `json:"event_id,omitempty"`
	Session    *InterpretingSessionDescriptor `json:"session,omitempty"`
	ResponseID string                         `json:"response_id,omitempty"`
	Text       string                         `json:"text,omitempty"`
	Audio      string                         `json:"audio,omitempty"`
	Segment    *TranscriptSegment             `json:"segment,omitempty"`
	Usage      *InterpretingUsage             `json:"usage,omitempty"`
	Error      *AudioError                    `json:"error,omitempty"`
}

type InterpretingUsage struct {
	InputAudioSeconds  float64          `json:"input_audio_seconds,omitempty"`
	OutputAudioSeconds float64          `json:"output_audio_seconds,omitempty"`
	TotalTokens        int              `json:"total_tokens,omitempty"`
	Source             TokenCountSource `json:"source,omitempty"`
}
