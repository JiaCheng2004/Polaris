package modality

import "context"

type VoiceAssetAdapter interface {
	ListCustomVoices(ctx context.Context, req *VoiceCatalogRequest) (*VoiceCatalogResponse, error)
	GetVoice(ctx context.Context, req *VoiceLookupRequest) (*VoiceCatalogItem, error)
	CreateClone(ctx context.Context, req *VoiceCloneRequest) (*VoiceCatalogItem, error)
	CreateDesign(ctx context.Context, req *VoiceDesignRequest) (*VoiceCatalogItem, error)
	RetrainVoice(ctx context.Context, req *VoiceCloneRequest) (*VoiceCatalogItem, error)
	ActivateVoice(ctx context.Context, req *VoiceLookupRequest) (*VoiceCatalogItem, error)
	DeleteVoice(ctx context.Context, req *VoiceLookupRequest) error
}

type VoiceLookupRequest struct {
	Provider string `json:"-"`
	Model    string `json:"-"`
	ID       string `json:"-"`
}

type VoiceCloneRequest struct {
	Model                  string          `json:"model"`
	Routing                *RoutingOptions `json:"routing,omitempty"`
	VoiceID                string          `json:"voice_id"`
	Audio                  string          `json:"audio"`
	AudioFormat            string          `json:"audio_format"`
	Language               string          `json:"language,omitempty"`
	PromptText             string          `json:"prompt_text,omitempty"`
	PreviewText            string          `json:"preview_text,omitempty"`
	Denoise                *bool           `json:"denoise,omitempty"`
	CheckPromptTextQuality *bool           `json:"check_prompt_text_quality,omitempty"`
	CheckAudioQuality      *bool           `json:"check_audio_quality,omitempty"`
	EnableSourceSeparation *bool           `json:"enable_source_separation,omitempty"`
	DenoiseModel           string          `json:"denoise_model,omitempty"`
}

type VoiceDesignRequest struct {
	Model          string          `json:"model"`
	Routing        *RoutingOptions `json:"routing,omitempty"`
	VoiceID        string          `json:"voice_id"`
	Text           string          `json:"text"`
	PromptText     string          `json:"prompt_text,omitempty"`
	PromptImageURL string          `json:"prompt_image_url,omitempty"`
}
