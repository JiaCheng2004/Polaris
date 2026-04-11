package store

import (
	"errors"
	"log/slog"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/modality"
)

type APIKey struct {
	ID            string
	Name          string
	KeyHash       string
	KeyPrefix     string
	OwnerID       string
	RateLimit     string
	AllowedModels []string
	IsAdmin       bool
	CreatedAt     time.Time
	LastUsedAt    *time.Time
	ExpiresAt     *time.Time
	IsRevoked     bool
}

type RequestLog struct {
	ID                string
	RequestID         string
	KeyID             string
	Model             string
	Modality          modality.Modality
	ProviderLatencyMs int
	TotalLatencyMs    int
	InputTokens       int
	OutputTokens      int
	TotalTokens       int
	EstimatedCost     float64
	StatusCode        int
	ErrorType         string
	CreatedAt         time.Time
}

type UsageFilter struct {
	KeyID    string
	OwnerID  string
	Model    string
	Modality modality.Modality
	From     *time.Time
	To       *time.Time
}

type DailyUsage struct {
	Date     string  `json:"date"`
	Requests int64   `json:"requests"`
	Tokens   int64   `json:"tokens"`
	CostUSD  float64 `json:"cost_usd"`
}

type ModelUsage struct {
	Model    string  `json:"model"`
	Requests int64   `json:"requests"`
	Tokens   int64   `json:"tokens"`
	CostUSD  float64 `json:"cost_usd"`
}

type UsageReport struct {
	TotalRequests int64        `json:"total_requests"`
	TotalTokens   int64        `json:"total_tokens"`
	TotalCost     float64      `json:"total_cost_usd"`
	ByDay         []DailyUsage `json:"by_day,omitempty"`
	ByModel       []ModelUsage `json:"by_model,omitempty"`
}

var (
	ErrNotFound       = errors.New("record not found")
	ErrNotImplemented = errors.New("not implemented")
)

type LoggerConfig struct {
	BufferSize    int
	FlushInterval time.Duration
}

func NewLoggerConfig(bufferSize int, flushInterval time.Duration) LoggerConfig {
	return LoggerConfig{
		BufferSize:    bufferSize,
		FlushInterval: flushInterval,
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(nil, nil))
}
