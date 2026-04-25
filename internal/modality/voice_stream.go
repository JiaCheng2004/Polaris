package modality

import "context"

const (
	StreamingTranscriptionClientEventSessionUpdate    = "session.update"
	StreamingTranscriptionClientEventInputAudioAppend = "input_audio.append"
	StreamingTranscriptionClientEventInputAudioCommit = "input_audio.commit"
	StreamingTranscriptionClientEventSessionClose     = "session.close"

	StreamingTranscriptionServerEventSessionCreated      = "session.created"
	StreamingTranscriptionServerEventSessionUpdated      = "session.updated"
	StreamingTranscriptionServerEventInputAudioCommitted = "input_audio.committed"
	StreamingTranscriptionServerEventTranscriptDelta     = "transcript.delta"
	StreamingTranscriptionServerEventTranscriptSegment   = "transcript.segment"
	StreamingTranscriptionServerEventTranscriptCompleted = "transcript.completed"
	StreamingTranscriptionServerEventError               = "error"
)

type StreamingTranscriptionAdapter interface {
	ConnectStreamingTranscription(ctx context.Context, cfg *StreamingTranscriptionSessionConfig) (StreamingTranscriptionSession, error)
}

type StreamingTranscriptionSession interface {
	Send(event StreamingTranscriptionClientEvent) error
	Events() <-chan StreamingTranscriptionServerEvent
	Close() error
}

type StreamingTranscriptionSessionConfig struct {
	Model            string          `json:"model"`
	Routing          *RoutingOptions `json:"routing,omitempty"`
	InputAudioFormat string          `json:"input_audio_format,omitempty"`
	SampleRateHz     int             `json:"sample_rate_hz,omitempty"`
	Language         string          `json:"language,omitempty"`
	InterimResults   *bool           `json:"interim_results,omitempty"`
	ReturnUtterances *bool           `json:"return_utterances,omitempty"`
}

type StreamingTranscriptionSessionDescriptor struct {
	ID           string `json:"id"`
	Object       string `json:"object"`
	Model        string `json:"model"`
	ExpiresAt    int64  `json:"expires_at"`
	WebSocketURL string `json:"websocket_url"`
	ClientSecret string `json:"client_secret"`
}

type StreamingTranscriptSegment struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
	Final bool    `json:"final"`
}

type StreamingTranscriptionClientEvent struct {
	Type    string                               `json:"type"`
	EventID string                               `json:"event_id,omitempty"`
	Session *StreamingTranscriptionSessionConfig `json:"session,omitempty"`
	Audio   string                               `json:"audio,omitempty"`
}

type StreamingTranscriptionServerEvent struct {
	Type       string                                   `json:"type"`
	EventID    string                                   `json:"event_id,omitempty"`
	Session    *StreamingTranscriptionSessionDescriptor `json:"session,omitempty"`
	Text       string                                   `json:"text,omitempty"`
	Segment    *StreamingTranscriptSegment              `json:"segment,omitempty"`
	Transcript *TranscriptResponse                      `json:"transcript,omitempty"`
	Error      *AudioError                              `json:"error,omitempty"`
}
