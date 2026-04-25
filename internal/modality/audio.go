package modality

import "context"

const (
	AudioFormatPCM16 = "pcm16"

	TurnDetectionManual    = "manual"
	TurnDetectionServerVAD = "server_vad"

	AudioClientEventSessionUpdate    = "session.update"
	AudioClientEventInputAudioAppend = "input_audio.append"
	AudioClientEventInputAudioCommit = "input_audio.commit"
	AudioClientEventInputText        = "input_text"
	AudioClientEventResponseCreate   = "response.create"
	AudioClientEventResponseCancel   = "response.cancel"
	AudioClientEventSessionClose     = "session.close"

	AudioServerEventSessionCreated          = "session.created"
	AudioServerEventSessionUpdated          = "session.updated"
	AudioServerEventInputAudioCommitted     = "input_audio.committed"
	AudioServerEventResponseAudioDelta      = "response.audio.delta"
	AudioServerEventResponseAudioDone       = "response.audio.done"
	AudioServerEventResponseTranscriptDelta = "response.transcript.delta"
	AudioServerEventResponseTranscriptDone  = "response.transcript.done"
	AudioServerEventResponseTextDelta       = "response.text.delta"
	AudioServerEventResponseTextDone        = "response.text.done"
	AudioServerEventResponseCompleted       = "response.completed"
	AudioServerEventError                   = "error"
)

// Full-duplex audio uses a provider-neutral session contract so gateway
// handlers can expose one realtime surface across different providers and
// transport implementations.
type AudioAdapter interface {
	Connect(ctx context.Context, cfg *AudioSessionConfig) (AudioSession, error)
}

type AudioSession interface {
	Send(event AudioClientEvent) error
	Events() <-chan AudioServerEvent
	Close() error
}

type AudioSessionConfig struct {
	Model             string               `json:"model"`
	Routing           *RoutingOptions      `json:"routing,omitempty"`
	Voice             string               `json:"voice,omitempty"`
	Instructions      string               `json:"instructions,omitempty"`
	InputAudioFormat  string               `json:"input_audio_format,omitempty"`
	OutputAudioFormat string               `json:"output_audio_format,omitempty"`
	SampleRateHz      int                  `json:"sample_rate_hz,omitempty"`
	TurnDetection     *TurnDetectionConfig `json:"turn_detection,omitempty"`
}

type AudioSessionDescriptor struct {
	ID           string `json:"id"`
	Object       string `json:"object"`
	Model        string `json:"model"`
	ExpiresAt    int64  `json:"expires_at"`
	WebSocketURL string `json:"websocket_url"`
	ClientSecret string `json:"client_secret"`
}

type TurnDetectionConfig struct {
	Mode            string `json:"mode"`
	SilenceMS       int    `json:"silence_ms,omitempty"`
	PrefixPaddingMS int    `json:"prefix_padding_ms,omitempty"`
}

type AudioResponseConfig struct {
	Voice        string `json:"voice,omitempty"`
	Instructions string `json:"instructions,omitempty"`
}

type AudioClientEvent struct {
	Type       string               `json:"type"`
	EventID    string               `json:"event_id,omitempty"`
	Session    *AudioSessionConfig  `json:"session,omitempty"`
	Audio      string               `json:"audio,omitempty"`
	Text       string               `json:"text,omitempty"`
	ResponseID string               `json:"response_id,omitempty"`
	Response   *AudioResponseConfig `json:"response,omitempty"`
}

type AudioServerEvent struct {
	Type       string                  `json:"type"`
	EventID    string                  `json:"event_id,omitempty"`
	Session    *AudioSessionDescriptor `json:"session,omitempty"`
	ResponseID string                  `json:"response_id,omitempty"`
	Audio      string                  `json:"audio,omitempty"`
	Text       string                  `json:"text,omitempty"`
	Transcript string                  `json:"transcript,omitempty"`
	Usage      *AudioUsage             `json:"usage,omitempty"`
	Error      *AudioError             `json:"error,omitempty"`
}

type AudioUsage struct {
	InputAudioSeconds  float64          `json:"input_audio_seconds,omitempty"`
	OutputAudioSeconds float64          `json:"output_audio_seconds,omitempty"`
	InputTextTokens    int              `json:"input_text_tokens,omitempty"`
	OutputTextTokens   int              `json:"output_text_tokens,omitempty"`
	TotalTokens        int              `json:"total_tokens,omitempty"`
	Source             TokenCountSource `json:"source,omitempty"`
}

type AudioError struct {
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Param   string `json:"param,omitempty"`
}
