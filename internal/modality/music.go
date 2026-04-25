package modality

import (
	"context"
	"encoding/json"
	"io"
)

type MusicAdapter interface {
	Generate(ctx context.Context, req *MusicGenerationRequest) (*MusicOperationResult, error)
	StreamGenerate(ctx context.Context, req *MusicGenerationRequest) (*MusicStream, error)
	Edit(ctx context.Context, req *MusicEditRequest) (*MusicOperationResult, error)
	StreamEdit(ctx context.Context, req *MusicEditRequest) (*MusicStream, error)
	SeparateStems(ctx context.Context, req *MusicStemRequest) (*MusicOperationResult, error)
	GenerateLyrics(ctx context.Context, req *MusicLyricsRequest) (*MusicLyricsResponse, error)
	CreatePlan(ctx context.Context, req *MusicPlanRequest) (*MusicPlanResponse, error)
}

type MusicGenerationRequest struct {
	Model           string          `json:"model"`
	Routing         *RoutingOptions `json:"routing,omitempty"`
	Prompt          string          `json:"prompt,omitempty"`
	Lyrics          string          `json:"lyrics,omitempty"`
	Plan            json.RawMessage `json:"plan,omitempty"`
	DurationMS      int             `json:"duration_ms,omitempty"`
	Instrumental    bool            `json:"instrumental,omitempty"`
	Seed            *int            `json:"seed,omitempty"`
	OutputFormat    string          `json:"output_format,omitempty"`
	SampleRateHz    int             `json:"sample_rate_hz,omitempty"`
	Bitrate         int             `json:"bitrate,omitempty"`
	StoreForEditing bool            `json:"store_for_editing,omitempty"`
	SignWithC2PA    bool            `json:"sign_with_c2pa,omitempty"`
	WithTimestamps  bool            `json:"with_timestamps,omitempty"`
}

type MusicEditRequest struct {
	Model           string          `json:"model"`
	Routing         *RoutingOptions `json:"routing,omitempty"`
	Operation       string          `json:"operation"`
	Prompt          string          `json:"prompt,omitempty"`
	Lyrics          string          `json:"lyrics,omitempty"`
	Plan            json.RawMessage `json:"plan,omitempty"`
	SourceJobID     string          `json:"source_job_id,omitempty"`
	SourceAudio     string          `json:"source_audio,omitempty"`
	File            []byte          `json:"-"`
	Filename        string          `json:"-"`
	ContentType     string          `json:"-"`
	DurationMS      int             `json:"duration_ms,omitempty"`
	Instrumental    bool            `json:"instrumental,omitempty"`
	Seed            *int            `json:"seed,omitempty"`
	OutputFormat    string          `json:"output_format,omitempty"`
	SampleRateHz    int             `json:"sample_rate_hz,omitempty"`
	Bitrate         int             `json:"bitrate,omitempty"`
	StoreForEditing bool            `json:"store_for_editing,omitempty"`
	SignWithC2PA    bool            `json:"sign_with_c2pa,omitempty"`
	WithTimestamps  bool            `json:"with_timestamps,omitempty"`
}

type MusicStemRequest struct {
	Model        string          `json:"model"`
	Routing      *RoutingOptions `json:"routing,omitempty"`
	SourceJobID  string          `json:"source_job_id,omitempty"`
	SourceAudio  string          `json:"source_audio,omitempty"`
	File         []byte          `json:"-"`
	Filename     string          `json:"-"`
	ContentType  string          `json:"-"`
	StemVariant  string          `json:"stem_variant,omitempty"`
	OutputFormat string          `json:"output_format,omitempty"`
	SignWithC2PA bool            `json:"sign_with_c2pa,omitempty"`
}

type MusicLyricsRequest struct {
	Model   string          `json:"model"`
	Routing *RoutingOptions `json:"routing,omitempty"`
	Mode    string          `json:"mode,omitempty"`
	Prompt  string          `json:"prompt,omitempty"`
	Lyrics  string          `json:"lyrics,omitempty"`
	Title   string          `json:"title,omitempty"`
}

type MusicPlanRequest struct {
	Model      string          `json:"model"`
	Routing    *RoutingOptions `json:"routing,omitempty"`
	Prompt     string          `json:"prompt"`
	DurationMS int             `json:"duration_ms,omitempty"`
	SourcePlan json.RawMessage `json:"source_plan,omitempty"`
}

type MusicJob struct {
	JobID         string `json:"job_id"`
	Status        string `json:"status"`
	Model         string `json:"model,omitempty"`
	Operation     string `json:"operation,omitempty"`
	EstimatedTime int    `json:"estimated_time,omitempty"`
}

type MusicStatus struct {
	JobID       string       `json:"job_id"`
	Status      string       `json:"status"`
	Model       string       `json:"model,omitempty"`
	Operation   string       `json:"operation,omitempty"`
	Progress    float64      `json:"progress,omitempty"`
	Result      *MusicResult `json:"result,omitempty"`
	Error       *MusicError  `json:"error,omitempty"`
	CreatedAt   int64        `json:"created_at,omitempty"`
	CompletedAt int64        `json:"completed_at,omitempty"`
	ExpiresAt   int64        `json:"expires_at,omitempty"`
}

type MusicResult struct {
	SongID       string          `json:"song_id,omitempty"`
	DownloadURL  string          `json:"download_url,omitempty"`
	ContentType  string          `json:"content_type,omitempty"`
	Filename     string          `json:"filename,omitempty"`
	DurationMS   int             `json:"duration_ms,omitempty"`
	SampleRateHz int             `json:"sample_rate_hz,omitempty"`
	Bitrate      int             `json:"bitrate,omitempty"`
	SizeBytes    int             `json:"size_bytes,omitempty"`
	Lyrics       string          `json:"lyrics,omitempty"`
	Plan         json.RawMessage `json:"plan,omitempty"`
}

type MusicError struct {
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type MusicOperationResult struct {
	Asset        *MusicAsset
	SongID       string
	DurationMS   int
	SampleRateHz int
	Bitrate      int
	SizeBytes    int
	Lyrics       string
	Plan         json.RawMessage
}

type MusicAsset struct {
	Data        []byte
	ContentType string
	Filename    string
}

type MusicLyricsResponse struct {
	Title     string `json:"title,omitempty"`
	StyleTags string `json:"style_tags,omitempty"`
	Lyrics    string `json:"lyrics,omitempty"`
}

type MusicPlanResponse struct {
	Plan json.RawMessage `json:"plan"`
}

type MusicStream struct {
	Body        io.ReadCloser
	ContentType string
	Filename    string
}
