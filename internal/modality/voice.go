package modality

import "context"

type VoiceAdapter interface {
	TextToSpeech(ctx context.Context, req *TTSRequest) (*AudioResponse, error)
	SpeechToText(ctx context.Context, req *STTRequest) (*TranscriptResponse, error)
}

type TTSRequest struct {
	Model          string          `json:"model"`
	Routing        *RoutingOptions `json:"routing,omitempty"`
	Input          string          `json:"input"`
	Voice          string          `json:"voice"`
	ResponseFormat string          `json:"response_format,omitempty"`
	Speed          *float64        `json:"speed,omitempty"`
}

type STTRequest struct {
	Model          string          `json:"model"`
	Routing        *RoutingOptions `json:"routing,omitempty"`
	File           []byte          `json:"-"`
	Filename       string          `json:"-"`
	ContentType    string          `json:"-"`
	Language       string          `form:"language" json:"language,omitempty"`
	ResponseFormat string          `form:"response_format" json:"response_format,omitempty"`
	Temperature    *float64        `form:"temperature" json:"temperature,omitempty"`
}

type AudioResponse struct {
	Data        []byte
	ContentType string
}

type TranscriptResponse struct {
	Text        string              `json:"text,omitempty"`
	Language    string              `json:"language,omitempty"`
	Duration    float64             `json:"duration,omitempty"`
	Segments    []TranscriptSegment `json:"segments,omitempty"`
	Raw         []byte              `json:"-"`
	ContentType string              `json:"-"`
	Format      string              `json:"-"`
}

type TranscriptSegment struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}
