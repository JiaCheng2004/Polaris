package store

import (
	"errors"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/modality"
)

type Project struct {
	ID          string
	Name        string
	Description string
	CreatedAt   time.Time
	ArchivedAt  *time.Time
}

type VirtualKey struct {
	ID                string
	ProjectID         string
	Name              string
	KeyHash           string
	KeyPrefix         string
	RateLimit         string
	AllowedModels     []string
	AllowedModalities []modality.Modality
	AllowedToolsets   []string
	AllowedMCP        []string
	IsAdmin           bool
	CreatedAt         time.Time
	LastUsedAt        *time.Time
	ExpiresAt         *time.Time
	IsRevoked         bool
}

type Policy struct {
	ID                string
	ProjectID         string
	Name              string
	Description       string
	AllowedModels     []string
	AllowedModalities []modality.Modality
	AllowedToolsets   []string
	AllowedMCP        []string
	CreatedAt         time.Time
}

type BudgetMode string

const (
	BudgetModeSoft BudgetMode = "soft"
	BudgetModeHard BudgetMode = "hard"
)

func (m BudgetMode) Valid() bool {
	switch m {
	case BudgetModeSoft, BudgetModeHard:
		return true
	default:
		return false
	}
}

type Budget struct {
	ID            string
	ProjectID     string
	Name          string
	Mode          BudgetMode
	LimitUSD      float64
	LimitRequests int64
	Window        string
	CreatedAt     time.Time
}

type AuditEvent struct {
	ID           string
	ProjectID    string
	ActorKeyID   string
	Kind         string
	ResourceType string
	ResourceID   string
	MetadataJSON string
	CreatedAt    time.Time
}

type ToolDefinition struct {
	ID             string
	Name           string
	Description    string
	Implementation string
	InputSchema    string
	Enabled        bool
	CreatedAt      time.Time
}

type Toolset struct {
	ID          string
	Name        string
	Description string
	ToolIDs     []string
	CreatedAt   time.Time
}

type MCPBindingKind string

const (
	MCPBindingKindUpstreamProxy MCPBindingKind = "upstream_proxy"
	MCPBindingKindLocalToolset  MCPBindingKind = "local_toolset"
)

func (k MCPBindingKind) Valid() bool {
	switch k {
	case MCPBindingKindUpstreamProxy, MCPBindingKindLocalToolset:
		return true
	default:
		return false
	}
}

type MCPBinding struct {
	ID          string
	Name        string
	Kind        MCPBindingKind
	UpstreamURL string
	ToolsetID   string
	HeadersJSON string
	Enabled     bool
	CreatedAt   time.Time
}

type ArchivedVoice struct {
	Provider   string
	Model      string
	VoiceID    string
	ArchivedAt time.Time
}

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
	ProjectID         string
	Model             string
	Modality          modality.Modality
	InterfaceFamily   string
	TokenSource       string
	CacheStatus       string
	FallbackModel     string
	TraceID           string
	Toolset           string
	MCPBinding        string
	ProviderLatencyMs int
	TotalLatencyMs    int
	InputTokens       int
	OutputTokens      int
	TotalTokens       int
	EstimatedCost     float64
	CostSource        string
	StatusCode        int
	ErrorType         string
	CreatedAt         time.Time
}

type UsageFilter struct {
	KeyID     string
	OwnerID   string
	ProjectID string
	Model     string
	Modality  modality.Modality
	From      *time.Time
	To        *time.Time
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
	TotalRequests       int64            `json:"total_requests"`
	TotalTokens         int64            `json:"total_tokens"`
	TotalCost           float64          `json:"total_cost_usd"`
	CostSourceBreakdown map[string]int64 `json:"cost_source_breakdown,omitempty"`
	ByDay               []DailyUsage     `json:"by_day,omitempty"`
	ByModel             []ModelUsage     `json:"by_model,omitempty"`
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
