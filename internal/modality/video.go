package modality

import "context"

type VideoAdapter interface {
	Generate(ctx context.Context, req *VideoRequest) (*VideoJob, error)
	GetStatus(ctx context.Context, jobID string) (*VideoStatus, error)
	Cancel(ctx context.Context, jobID string) error
	Download(ctx context.Context, jobID string, status *VideoStatus) (*VideoAsset, error)
}

type VideoRequest struct {
	Model           string          `json:"model"`
	Routing         *RoutingOptions `json:"routing,omitempty"`
	Prompt          string          `json:"prompt"`
	Duration        int             `json:"duration,omitempty"`
	AspectRatio     string          `json:"aspect_ratio,omitempty"`
	Resolution      string          `json:"resolution,omitempty"`
	FirstFrame      string          `json:"first_frame,omitempty"`
	ReferenceImages []string        `json:"reference_images,omitempty"`
	WithAudio       bool            `json:"with_audio,omitempty"`

	LastFrame       string   `json:"last_frame,omitempty"`
	ReferenceVideos []string `json:"reference_videos,omitempty"`
	Audio           string   `json:"audio,omitempty"`
}

type VideoJob struct {
	JobID         string `json:"job_id"`
	Status        string `json:"status"`
	EstimatedTime int    `json:"estimated_time,omitempty"`
	Model         string `json:"model,omitempty"`
}

type VideoStatus struct {
	JobID       string       `json:"job_id"`
	Status      string       `json:"status"`
	Progress    float64      `json:"progress,omitempty"`
	Result      *VideoResult `json:"result,omitempty"`
	Error       *VideoError  `json:"error,omitempty"`
	CreatedAt   int64        `json:"created_at,omitempty"`
	CompletedAt int64        `json:"completed_at,omitempty"`
	ExpiresAt   int64        `json:"expires_at,omitempty"`
}

type VideoResult struct {
	VideoURL    string `json:"video_url,omitempty"`
	AudioURL    string `json:"audio_url,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Duration    int    `json:"duration,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

type VideoError struct {
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type VideoAsset struct {
	Data        []byte
	ContentType string
}
