package modality

import "context"

const (
	PodcastStatusQueued    = "queued"
	PodcastStatusRunning   = "running"
	PodcastStatusCompleted = "completed"
	PodcastStatusFailed    = "failed"
)

type PodcastAdapter interface {
	GeneratePodcast(ctx context.Context, req *PodcastRequest) (*PodcastResult, error)
}

type PodcastRequest struct {
	Model        string           `json:"model"`
	Routing      *RoutingOptions  `json:"routing,omitempty"`
	Segments     []PodcastSegment `json:"segments"`
	OutputFormat string           `json:"output_format,omitempty"`
	SampleRateHz int              `json:"sample_rate_hz,omitempty"`
	UseHeadMusic *bool            `json:"use_head_music,omitempty"`
}

type PodcastSegment struct {
	Speaker string `json:"speaker"`
	Voice   string `json:"voice,omitempty"`
	Text    string `json:"text"`
}

type PodcastResult struct {
	Audio       []byte         `json:"-"`
	ContentType string         `json:"content_type,omitempty"`
	Usage       Usage          `json:"usage,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type PodcastJob struct {
	ID     string `json:"id"`
	Object string `json:"object"`
	Model  string `json:"model"`
	Status string `json:"status"`
}

type PodcastStatus struct {
	ID     string         `json:"id"`
	Object string         `json:"object"`
	Model  string         `json:"model"`
	Status string         `json:"status"`
	Result *PodcastResult `json:"result,omitempty"`
	Error  *AudioError    `json:"error,omitempty"`
}
