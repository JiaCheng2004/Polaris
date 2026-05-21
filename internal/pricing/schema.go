package pricing

import "strings"

const (
	SourceTable        = "table"
	SourceFallbackZero = "fallback_zero"
	SourceMissing      = "missing"

	LookupHit     = "hit"
	LookupGlobHit = "glob_hit"
	LookupMiss    = "miss"
)

type File struct {
	Version int              `yaml:"version"`
	Models  map[string]Entry `yaml:"models"`
}

type Entry struct {
	Mode             string             `yaml:"mode"`
	BillingMode      string             `yaml:"billing_mode,omitempty"`
	Unit             string             `yaml:"unit,omitempty"`
	QuotaBucket      string             `yaml:"quota_bucket,omitempty"`
	DailyLimit       *float64           `yaml:"daily_limit,omitempty"`
	ConcurrencyLimit *int               `yaml:"concurrency_limit,omitempty"`
	Currency         string             `yaml:"currency,omitempty"`
	ContextWindow    int                `yaml:"context_window,omitempty"`
	Source           string             `yaml:"source,omitempty"`
	EffectiveFrom    string             `yaml:"effective_from,omitempty"`
	EffectiveUntil   string             `yaml:"effective_until,omitempty"`
	Notes            string             `yaml:"notes,omitempty"`
	Pricing          *Rates             `yaml:"pricing,omitempty"`
	Tiers            map[string]Rates   `yaml:"tiers,omitempty"`
	TieredPricing    []TieredRates      `yaml:"tiered_pricing,omitempty"`
	AdditionalUnits  map[string]float64 `yaml:"additional_units,omitempty"`
	Deployments      map[string]Rates   `yaml:"deployments,omitempty"`
	Deprecation      *Deprecation       `yaml:"deprecation,omitempty"`
}

type Rates struct {
	Multiplier float64 `yaml:"multiplier,omitempty"`

	InputPerMTok           float64 `yaml:"input_per_mtok,omitempty"`
	OutputPerMTok          float64 `yaml:"output_per_mtok,omitempty"`
	OutputReasoningPerMTok float64 `yaml:"output_reasoning_per_mtok,omitempty"`

	CacheReadPerMTok     float64 `yaml:"cache_read_per_mtok,omitempty"`
	CacheWrite5mPerMTok  float64 `yaml:"cache_write_5m_per_mtok,omitempty"`
	CacheWrite1hPerMTok  float64 `yaml:"cache_write_1h_per_mtok,omitempty"`
	InputCacheHitPerMTok float64 `yaml:"input_cache_hit_per_mtok,omitempty"`

	InputImageTokenPerMTok  float64 `yaml:"input_image_token_per_mtok,omitempty"`
	OutputImageTokenPerMTok float64 `yaml:"output_image_token_per_mtok,omitempty"`

	InputPerAudioSecond     float64 `yaml:"input_per_audio_second,omitempty"`
	OutputPerAudioSecond    float64 `yaml:"output_per_audio_second,omitempty"`
	InputPerVideoSecond     float64 `yaml:"input_per_video_second,omitempty"`
	OutputPerVideoSecond    float64 `yaml:"output_per_video_second,omitempty"`
	InputPerPDFPage         float64 `yaml:"input_per_pdf_page,omitempty"`
	InputPerImageTile       float64 `yaml:"input_per_image_tile,omitempty"`
	InputPerAudioFileSecond float64 `yaml:"input_per_audio_file_second,omitempty"`
	PerFileStorageGBHour    float64 `yaml:"per_file_storage_gb_hour,omitempty"`
	PerFileBytesIngestedGB  float64 `yaml:"per_file_bytes_ingested_gb,omitempty"`
	InputPerCharacter       float64 `yaml:"input_per_character,omitempty"`
	InputPerImage           float64 `yaml:"input_per_image,omitempty"`
	OutputPerImage          float64 `yaml:"output_per_image,omitempty"`
	OutputPerPixel          float64 `yaml:"output_per_pixel,omitempty"`
	PerCall                 float64 `yaml:"per_call,omitempty"`
}

type TieredRates struct {
	ID    string `yaml:"id,omitempty"`
	Range [2]int `yaml:"range"`
	Rates `yaml:",inline"`
}

type Deprecation struct {
	Replacement string `yaml:"replacement,omitempty"`
	Sunset      string `yaml:"sunset,omitempty"`
}

type EstimateRequest struct {
	Model                 string
	Tier                  string
	Deployment            string
	InputTokens           int
	CachedInputTokens     int
	CacheWrite5mTokens    int
	CacheWrite1hTokens    int
	OutputTokens          int
	ReasoningTokens       int
	InputImageTokens      int
	OutputImageTokens     int
	AudioSeconds          float64
	VideoSeconds          float64
	PDFPages              int
	ImageTiles            int
	InputAudioFileSeconds float64
	FileStorageGBHours    float64
	FileBytesIngested     int64
	Characters            int
	Images                int
	UnitCounts            map[string]int
}

type EstimateResult struct {
	TotalUSD      float64
	Currency      string
	Source        string
	BreakdownUSD  map[string]float64
	EffectiveTier string
	EffectiveBand string
	LookupStatus  string
}

type Estimator interface {
	Estimate(req EstimateRequest) EstimateResult
	Lookup(modelID string) (Entry, bool)
}

func validMode(mode string) bool {
	switch strings.TrimSpace(mode) {
	case "chat",
		"embedding",
		"embed",
		"image",
		"image_generation",
		"audio",
		"audio_transcription",
		"audio_speech",
		"voice",
		"video",
		"interpreting",
		"music",
		"files",
		"batch",
		"podcast",
		"notes",
		"translation":
		return true
	default:
		return false
	}
}

func validBillingMode(mode string) bool {
	switch strings.TrimSpace(mode) {
	case "", "usage_based", "token_plan", "credit", "quota", "unknown":
		return true
	default:
		return false
	}
}

func validTier(name string) bool {
	switch strings.TrimSpace(name) {
	case "batch", "flex", "priority", "standard":
		return true
	default:
		return false
	}
}
