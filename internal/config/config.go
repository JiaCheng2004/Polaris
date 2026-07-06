package config

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/modality"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server        ServerConfig              `yaml:"server"`
	Auth          AuthConfig                `yaml:"auth"`
	Store         StoreConfig               `yaml:"store"`
	Cache         CacheConfig               `yaml:"cache"`
	Providers     map[string]ProviderConfig `yaml:"providers"`
	Routing       RoutingConfig             `yaml:"routing"`
	ControlPlane  ControlPlaneConfig        `yaml:"control_plane"`
	Tools         ToolsConfig               `yaml:"tools"`
	MCP           MCPConfig                 `yaml:"mcp"`
	Files         FilesConfig               `yaml:"files"`
	Pricing       PricingConfig             `yaml:"pricing"`
	Observability ObservabilityConfig       `yaml:"observability"`
	Reliability   ReliabilityConfig         `yaml:"reliability"`
}

// ReliabilityConfig tunes the process-lifetime reliability manager (circuit
// breaking, load shedding, retry budgets, idempotency). Everything defaults to
// off/permissive so behavior is unchanged until an operator opts in.
type ReliabilityConfig struct {
	Shed             ShedConfig        `yaml:"shed"`
	Breaker          BreakerConfig     `yaml:"breaker_defaults"`
	Idempotency      IdempotencyConfig `yaml:"idempotency"`
	RetryBudgetRatio float64           `yaml:"retry_budget_ratio"`
}

type ShedConfig struct {
	Enabled                bool `yaml:"enabled"`
	GlobalMaxInflight      int  `yaml:"global_max_inflight"`
	PerProviderMaxInflight int  `yaml:"per_provider_max_inflight"`
}

type BreakerConfig struct {
	ErrorRate      float64       `yaml:"error_rate"`
	MinSamples     int           `yaml:"min_samples"`
	OpenFor        time.Duration `yaml:"open_for"`
	HalfOpenProbes int           `yaml:"half_open_probes"`
}

type IdempotencyConfig struct {
	Enabled bool          `yaml:"enabled"`
	TTL     time.Duration `yaml:"ttl"`
}

type ServerConfig struct {
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	IdleTimeout     time.Duration `yaml:"idle_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
	MaxBodyBytes    int64         `yaml:"max_body_bytes"`
	CORS            CORSConfig    `yaml:"cors"`
}

type CORSConfig struct {
	Enabled          bool          `yaml:"enabled"`
	AllowedOrigins   []string      `yaml:"allowed_origins"`
	AllowedHeaders   []string      `yaml:"allowed_headers"`
	AllowedMethods   []string      `yaml:"allowed_methods"`
	ExposedHeaders   []string      `yaml:"exposed_headers"`
	AllowCredentials bool          `yaml:"allow_credentials"`
	MaxAge           time.Duration `yaml:"max_age"`
}

type AuthMode string

const (
	AuthModeNone        AuthMode = "none"
	AuthModeStatic      AuthMode = "static"
	AuthModeExternal    AuthMode = "external"
	AuthModeVirtualKeys AuthMode = "virtual_keys"
	AuthModeMultiUser   AuthMode = "multi-user"
)

type AuthConfig struct {
	Mode                  AuthMode           `yaml:"mode"`
	StaticKeys            []StaticKeyConfig  `yaml:"static_keys"`
	External              ExternalAuthConfig `yaml:"external"`
	AdminKeyHash          string             `yaml:"admin_key_hash"`
	BootstrapAdminKeyHash string             `yaml:"bootstrap_admin_key_hash"`
}

type ExternalAuthConfig struct {
	Provider     string        `yaml:"provider"`
	SharedSecret string        `yaml:"shared_secret"`
	MaxClockSkew time.Duration `yaml:"max_clock_skew"`
	CacheTTL     time.Duration `yaml:"cache_ttl"`
}

type StaticKeyConfig struct {
	Name          string   `yaml:"name"`
	KeyHash       string   `yaml:"key_hash"`
	RateLimit     string   `yaml:"rate_limit"`
	AllowedModels []string `yaml:"allowed_models"`
}

type StoreConfig struct {
	Driver           string        `yaml:"driver"`
	DSN              string        `yaml:"dsn"`
	MaxConnections   int           `yaml:"max_connections"`
	LogRetentionDays int           `yaml:"log_retention_days"`
	LogBufferSize    int           `yaml:"log_buffer_size"`
	LogFlushInterval time.Duration `yaml:"log_flush_interval"`
}

type CacheConfig struct {
	Driver        string          `yaml:"driver"`
	URL           string          `yaml:"url"`
	RateLimit     RateLimitConfig `yaml:"rate_limit"`
	ResponseCache ResponseCache   `yaml:"response_cache"`
}

type RateLimitConfig struct {
	Enabled bool   `yaml:"enabled"`
	Default string `yaml:"default"`
	Window  string `yaml:"window"`
	// FailMode controls behavior when the primary limiter (e.g. Redis) is
	// unavailable: "open" (default) degrades to best-effort process-local counting
	// then allows; "closed" returns 503 to cap runaway provider spend.
	FailMode string `yaml:"fail_mode"`
}

type ResponseCache struct {
	Enabled             bool          `yaml:"enabled"`
	TTL                 time.Duration `yaml:"ttl"`
	MaxEntriesPerModel  int           `yaml:"max_entries_per_model"`
	SimilarityThreshold float64       `yaml:"similarity_threshold"`
}

type ProviderConfig struct {
	APIKey            string                 `yaml:"api_key"`
	AccessKeyID       string                 `yaml:"access_key_id"`
	AccessKeySecret   string                 `yaml:"access_key_secret"`
	SessionToken      string                 `yaml:"session_token"`
	AppID             string                 `yaml:"app_id"`
	SpeechAPIKey      string                 `yaml:"speech_api_key"`
	SpeechAccessToken string                 `yaml:"speech_access_token"`
	SecretKey         string                 `yaml:"secret_key"`
	ProjectName       string                 `yaml:"project_name"`
	ProjectID         string                 `yaml:"project_id"`
	Location          string                 `yaml:"location"`
	BaseURL           string                 `yaml:"base_url"`
	ControlBaseURL    string                 `yaml:"control_base_url"`
	Timeout           time.Duration          `yaml:"timeout"`
	Retry             RetryConfig            `yaml:"retry"`
	Models            map[string]ModelConfig `yaml:"models"`
}

type RetryConfig struct {
	MaxAttempts  int           `yaml:"max_attempts"`
	Backoff      string        `yaml:"backoff"`
	InitialDelay time.Duration `yaml:"initial_delay"`
}

type ModelConfig struct {
	Modality         modality.Modality     `yaml:"modality"`
	Capabilities     []modality.Capability `yaml:"capabilities"`
	ContextWindow    int                   `yaml:"context_window"`
	MaxOutputTokens  int                   `yaml:"max_output_tokens"`
	OutputFormats    []string              `yaml:"output_formats"`
	MinDurationMs    int                   `yaml:"min_duration_ms"`
	MaxDurationMs    int                   `yaml:"max_duration_ms"`
	SampleRatesHz    []int                 `yaml:"sample_rates_hz"`
	Dimensions       int                   `yaml:"dimensions"`
	Endpoint         string                `yaml:"endpoint"`
	MaxDuration      int                   `yaml:"max_duration"`
	AllowedDurations []int                 `yaml:"allowed_durations"`
	AspectRatios     []string              `yaml:"aspect_ratios"`
	Resolutions      []string              `yaml:"resolutions"`
	Cancelable       bool                  `yaml:"cancelable"`
	Voices           []string              `yaml:"voices"`
	Formats          []string              `yaml:"formats"`
	AudioPipeline    AudioPipelineConfig   `yaml:"audio_pipeline"`
	RealtimeSession  AudioRealtimeConfig   `yaml:"realtime_session"`
	SessionTTL       time.Duration         `yaml:"session_ttl"`
}

type AudioPipelineConfig struct {
	ChatModel string `yaml:"chat_model"`
	STTModel  string `yaml:"stt_model"`
	TTSModel  string `yaml:"tts_model"`
}

type AudioRealtimeConfig struct {
	Transport  string `yaml:"transport"`
	Auth       string `yaml:"auth"`
	URL        string `yaml:"url"`
	Model      string `yaml:"model"`
	ResourceID string `yaml:"resource_id"`
	AppKey     string `yaml:"app_key"`
}

type RoutingConfig struct {
	Fallbacks []FallbackRule             `yaml:"fallbacks"`
	Aliases   map[string]string          `yaml:"aliases"`
	Selectors map[string]RoutingSelector `yaml:"selectors"`
}

type RoutingSelector struct {
	Modality            modality.Modality     `yaml:"modality"`
	Capabilities        []modality.Capability `yaml:"capabilities"`
	Providers           []string              `yaml:"providers"`
	ExcludeProviders    []string              `yaml:"exclude_providers"`
	Statuses            []string              `yaml:"statuses"`
	VerificationClasses []string              `yaml:"verification_classes"`
	Prefer              []string              `yaml:"prefer"`
	CostTier            string                `yaml:"cost_tier"`
	LatencyTier         string                `yaml:"latency_tier"`
}

type ControlPlaneConfig struct {
	Enabled bool `yaml:"enabled"`
}

type LocalToolConfig struct {
	Implementation string `yaml:"implementation"`
}

type ToolsConfig struct {
	Enabled bool                       `yaml:"enabled"`
	Local   map[string]LocalToolConfig `yaml:"local"`
}

type MCPConfig struct {
	Enabled bool `yaml:"enabled"`
}

type FilesConfig struct {
	Enabled       bool                      `yaml:"enabled"`
	Ingestion     FileIngestionConfig       `yaml:"ingestion"`
	Storage       FileStorageConfig         `yaml:"storage"`
	Downloads     FileDownloadsConfig       `yaml:"downloads"`
	SSRF          FileSSRFConfig            `yaml:"ssrf"`
	Materialize   FileMaterializationConfig `yaml:"materialization"`
	Understanding FileUnderstandingConfig   `yaml:"understanding"`
}

type FileIngestionConfig struct {
	MaxUploadBytes int64    `yaml:"max_upload_bytes"`
	AllowedMime    []string `yaml:"allowed_mime"`
}

type FileStorageConfig struct {
	InlineMaxBytes int64        `yaml:"inline_max_bytes"`
	BlobStore      string       `yaml:"blob_store"`
	DiskPath       string       `yaml:"disk_path"`
	S3             FileS3Config `yaml:"s3"`
}

type FileS3Config struct {
	Endpoint        string `yaml:"endpoint"`
	Bucket          string `yaml:"bucket"`
	Region          string `yaml:"region"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	SessionToken    string `yaml:"session_token"`
	UseSSL          bool   `yaml:"use_ssl"`
}

type FileDownloadsConfig struct {
	TokenTTL time.Duration `yaml:"token_ttl"`
}

type FileSSRFConfig struct {
	AllowedSchemes []string `yaml:"allowed_schemes"`
	DenyHosts      []string `yaml:"deny_hosts"`
}

type FileMaterializationConfig struct {
	InlineFallbackMax int64 `yaml:"inline_fallback_max"`
}

type FileUnderstandingConfig struct {
	Enabled        bool                               `yaml:"enabled"`
	Mode           string                             `yaml:"mode"`
	Profile        string                             `yaml:"profile"`
	MaxBytes       int64                              `yaml:"max_bytes"`
	MaxTextChars   int                                `yaml:"max_text_chars"`
	CacheArtifacts bool                               `yaml:"cache_artifacts"`
	Chunking       FileUnderstandingChunkingConfig    `yaml:"chunking"`
	Processors     []FileUnderstandingProcessorConfig `yaml:"processors"`
}

type FileUnderstandingChunkingConfig struct {
	Enabled      bool `yaml:"enabled"`
	MaxChars     int  `yaml:"max_chars"`
	OverlapChars int  `yaml:"overlap_chars"`
}

type FileUnderstandingProcessorConfig struct {
	Enabled      bool              `yaml:"enabled"`
	Name         string            `yaml:"name"`
	Backend      string            `yaml:"backend"`
	Endpoint     string            `yaml:"endpoint"`
	Method       string            `yaml:"method"`
	Version      string            `yaml:"version"`
	Timeout      time.Duration     `yaml:"timeout"`
	Priority     int               `yaml:"priority"`
	MIMETypes    []string          `yaml:"mime_types"`
	FileClasses  []string          `yaml:"file_classes"`
	Artifacts    []string          `yaml:"artifacts"`
	Capabilities []string          `yaml:"capabilities"`
	Profiles     []string          `yaml:"profiles"`
	Headers      map[string]string `yaml:"headers"`
	OCR          bool              `yaml:"ocr"`
}

type PricingConfig struct {
	File                  string `yaml:"file"`
	ReloadIntervalSeconds int    `yaml:"reload_interval_seconds"`
	FailOnMissing         bool   `yaml:"fail_on_missing"`
}

type FallbackRule struct {
	From string   `yaml:"from"`
	To   []string `yaml:"to"`
	On   []string `yaml:"on"`
}

type ObservabilityConfig struct {
	Metrics MetricsConfig `yaml:"metrics"`
	Logging LoggingConfig `yaml:"logging"`
	Traces  TracesConfig  `yaml:"traces"`
	Audit   AuditConfig   `yaml:"audit"`
}

type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type TracesConfig struct {
	Enabled     bool    `yaml:"enabled"`
	Endpoint    string  `yaml:"endpoint"`
	Insecure    bool    `yaml:"insecure"`
	ServiceName string  `yaml:"service_name"`
	SampleRatio float64 `yaml:"sample_ratio"`
	// GenAI opts into OpenTelemetry GenAI semantic-convention span attributes
	// (gen_ai.*) at request-span close. No prompt/completion content is captured.
	GenAI bool `yaml:"genai"`
}

type AuditConfig struct {
	Enabled bool `yaml:"enabled"`
}

type RuntimeOverrides struct {
	Port     int
	LogLevel string
	// Lenient disables strict unknown-key checking on load and hot-reload.
	Lenient bool
}

var envPattern = regexp.MustCompile(`\$\{([A-Z0-9_]+)\}`)

const DefaultMaxBodyBytes int64 = 64 * 1024 * 1024
const DefaultMaxFileUploadBytes int64 = 500 * 1024 * 1024
const DefaultInlineFileBytes int64 = 5 * 1024 * 1024

func Default() Config {
	return Config{
		Server: ServerConfig{
			Host:            "127.0.0.1",
			Port:            8080,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    120 * time.Second,
			IdleTimeout:     120 * time.Second,
			ShutdownTimeout: 15 * time.Second,
			MaxBodyBytes:    DefaultMaxBodyBytes,
			CORS:            DefaultCORSConfig(),
		},
		Auth: AuthConfig{
			Mode:       AuthModeNone,
			StaticKeys: []StaticKeyConfig{},
			External: ExternalAuthConfig{
				Provider:     "signed_headers",
				MaxClockSkew: 60 * time.Second,
				CacheTTL:     60 * time.Second,
			},
		},
		Store: StoreConfig{
			Driver:           "sqlite",
			DSN:              "./polaris.db",
			MaxConnections:   1,
			LogRetentionDays: 30,
			LogBufferSize:    1000,
			LogFlushInterval: 5 * time.Second,
		},
		Cache: CacheConfig{
			Driver: "memory",
			RateLimit: RateLimitConfig{
				Enabled:  true,
				Default:  "60/min",
				Window:   "sliding",
				FailMode: "open",
			},
			ResponseCache: ResponseCache{
				TTL:                 24 * time.Hour,
				MaxEntriesPerModel:  100,
				SimilarityThreshold: 0.95,
			},
		},
		Providers: map[string]ProviderConfig{},
		Routing: RoutingConfig{
			Aliases:   map[string]string{},
			Selectors: map[string]RoutingSelector{},
		},
		ControlPlane: ControlPlaneConfig{},
		Tools: ToolsConfig{
			Local: map[string]LocalToolConfig{},
		},
		MCP: MCPConfig{},
		Files: FilesConfig{
			Enabled: true,
			Ingestion: FileIngestionConfig{
				MaxUploadBytes: DefaultMaxFileUploadBytes,
				AllowedMime: []string{
					"application/pdf",
					"application/json",
					"application/xml",
					"application/x-yaml",
					"application/yaml",
					"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
					"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
					"application/vnd.openxmlformats-officedocument.presentationml.presentation",
					"text/*",
					"image/*",
					"audio/*",
				},
			},
			Storage: FileStorageConfig{
				InlineMaxBytes: DefaultInlineFileBytes,
				BlobStore:      "none",
				DiskPath:       "./data/files",
			},
			Downloads: FileDownloadsConfig{
				TokenTTL: 10 * time.Minute,
			},
			SSRF: FileSSRFConfig{
				AllowedSchemes: []string{"https"},
				DenyHosts:      []string{"169.254.169.254", "metadata.google.internal"},
			},
			Materialize: FileMaterializationConfig{
				InlineFallbackMax: DefaultInlineFileBytes,
			},
			Understanding: FileUnderstandingConfig{
				Enabled:        false,
				Mode:           "disabled",
				Profile:        "fast",
				MaxBytes:       DefaultInlineFileBytes,
				MaxTextChars:   12000,
				CacheArtifacts: true,
				Chunking: FileUnderstandingChunkingConfig{
					Enabled:      false,
					MaxChars:     1800,
					OverlapChars: 200,
				},
			},
		},
		Pricing: PricingConfig{
			ReloadIntervalSeconds: 30,
		},
		Observability: ObservabilityConfig{
			Metrics: MetricsConfig{
				Enabled: true,
				Path:    "/metrics",
			},
			Logging: LoggingConfig{
				Level:  "info",
				Format: "json",
			},
			Traces: TracesConfig{
				ServiceName: "polaris",
				SampleRatio: 1,
			},
			Audit: AuditConfig{
				Enabled: true,
			},
		},
		Reliability: ReliabilityConfig{
			Shed: ShedConfig{Enabled: false, GlobalMaxInflight: 0, PerProviderMaxInflight: 0},
			Breaker: BreakerConfig{
				ErrorRate:      0.5,
				MinSamples:     20,
				OpenFor:        30 * time.Second,
				HalfOpenProbes: 3,
			},
			Idempotency:      IdempotencyConfig{Enabled: true, TTL: 24 * time.Hour},
			RetryBudgetRatio: 0.2,
		},
	}
}

func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		Enabled: true,
		AllowedOrigins: []string{
			"http://localhost",
			"http://localhost:*",
			"http://127.0.0.1",
			"http://127.0.0.1:*",
			"https://localhost",
			"https://localhost:*",
			"https://127.0.0.1",
			"https://127.0.0.1:*",
		},
		AllowedHeaders: []string{
			"Authorization",
			"Content-Type",
			"X-Request-ID",
			"X-Polaris-Admin-Key",
		},
		AllowedMethods: []string{
			"GET",
			"POST",
			"DELETE",
			"OPTIONS",
		},
		ExposedHeaders: []string{
			"X-Request-ID",
			"X-RateLimit-Remaining",
			"Retry-After",
			"X-Polaris-Fallback",
			"X-Polaris-Resolved-Model",
			"X-Polaris-Resolved-Provider",
		},
		MaxAge: time.Hour,
	}
}

func EffectiveMaxBodyBytes(value int64) int64 {
	if value <= 0 {
		return DefaultMaxBodyBytes
	}
	return value
}

func EffectiveMaxFileUploadBytes(value int64) int64 {
	if value <= 0 {
		return DefaultMaxFileUploadBytes
	}
	return value
}

func EffectiveInlineFileBytes(value int64) int64 {
	if value <= 0 {
		return DefaultInlineFileBytes
	}
	return value
}

func EffectiveFileDownloadTTL(value time.Duration) time.Duration {
	if value <= 0 {
		return 10 * time.Minute
	}
	return value
}

// LoadOptions controls configuration loading.
type LoadOptions struct {
	// Lenient disables strict unknown-key checking. The default (strict) rejects
	// unknown or misspelled configuration keys with a load error so typos cannot
	// be silently ignored; --config-lenient sets this for forward-compatibility.
	Lenient bool
}

func Load(path string) (*Config, []string, error) {
	return LoadWithOptions(path, LoadOptions{})
}

// LoadWithOptions loads the config with explicit options.
func LoadWithOptions(path string, opts LoadOptions) (*Config, []string, error) {
	strict := !opts.Lenient
	cfg := Default()
	data, warnings, err := loadV2(path, strict)
	if err != nil {
		return nil, warnings, err
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(strict)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, warnings, fmt.Errorf("decode config yaml: %w", err)
	}

	if cfg.Providers == nil {
		cfg.Providers = map[string]ProviderConfig{}
	}
	if cfg.Routing.Aliases == nil {
		cfg.Routing.Aliases = map[string]string{}
	}
	if cfg.Routing.Selectors == nil {
		cfg.Routing.Selectors = map[string]RoutingSelector{}
	}
	if cfg.Tools.Local == nil {
		cfg.Tools.Local = map[string]LocalToolConfig{}
	}

	if err := ApplyEnvOverrides(&cfg); err != nil {
		return nil, warnings, err
	}
	if err := Validate(&cfg); err != nil {
		return nil, warnings, err
	}

	return &cfg, warnings, nil
}

func ApplyEnvOverrides(cfg *Config) error {
	if value := os.Getenv("POLARIS_HOST"); value != "" {
		cfg.Server.Host = value
	}
	if value := os.Getenv("POLARIS_PORT"); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse POLARIS_PORT: %w", err)
		}
		cfg.Server.Port = port
	}
	if value := os.Getenv("POLARIS_MAX_BODY_BYTES"); value != "" {
		maxBodyBytes, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("parse POLARIS_MAX_BODY_BYTES: %w", err)
		}
		cfg.Server.MaxBodyBytes = maxBodyBytes
	}
	if value := os.Getenv("POLARIS_LOG_LEVEL"); value != "" {
		cfg.Observability.Logging.Level = value
	}
	if value := os.Getenv("POLARIS_AUTH_MODE"); value != "" {
		cfg.Auth.Mode = AuthMode(value)
	}
	if value := os.Getenv("POLARIS_BOOTSTRAP_ADMIN_KEY_HASH"); value != "" {
		cfg.Auth.BootstrapAdminKeyHash = value
	}
	if value := os.Getenv("POLARIS_EXTERNAL_AUTH_PROVIDER"); value != "" {
		cfg.Auth.External.Provider = value
	}
	if value := os.Getenv("POLARIS_EXTERNAL_AUTH_SECRET"); value != "" {
		cfg.Auth.External.SharedSecret = value
	}
	if value := os.Getenv("POLARIS_EXTERNAL_AUTH_MAX_CLOCK_SKEW"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("parse POLARIS_EXTERNAL_AUTH_MAX_CLOCK_SKEW: %w", err)
		}
		cfg.Auth.External.MaxClockSkew = parsed
	}
	if value := os.Getenv("POLARIS_EXTERNAL_AUTH_CACHE_TTL"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("parse POLARIS_EXTERNAL_AUTH_CACHE_TTL: %w", err)
		}
		cfg.Auth.External.CacheTTL = parsed
	}
	if value := os.Getenv("POLARIS_STORE_DRIVER"); value != "" {
		cfg.Store.Driver = value
	}
	if value := os.Getenv("POLARIS_STORE_DSN"); value != "" {
		cfg.Store.DSN = value
	}
	if value := os.Getenv("POLARIS_CACHE_DRIVER"); value != "" {
		cfg.Cache.Driver = value
	}
	if value := os.Getenv("POLARIS_CACHE_URL"); value != "" {
		cfg.Cache.URL = value
	}
	if value := os.Getenv("POLARIS_FILES_ENABLED"); value != "" {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("parse POLARIS_FILES_ENABLED: %w", err)
		}
		cfg.Files.Enabled = enabled
	}
	if value := os.Getenv("POLARIS_FILES_MAX_UPLOAD_BYTES"); value != "" {
		maxUploadBytes, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("parse POLARIS_FILES_MAX_UPLOAD_BYTES: %w", err)
		}
		cfg.Files.Ingestion.MaxUploadBytes = maxUploadBytes
	}
	if value := os.Getenv("POLARIS_FILES_BLOB_STORE"); value != "" {
		cfg.Files.Storage.BlobStore = value
	}
	if value := os.Getenv("POLARIS_FILES_DISK_PATH"); value != "" {
		cfg.Files.Storage.DiskPath = value
	}
	if value := os.Getenv("POLARIS_OTEL_ENDPOINT"); value != "" {
		cfg.Observability.Traces.Endpoint = value
	}
	if value := os.Getenv("POLARIS_OTEL_SERVICE_NAME"); value != "" {
		cfg.Observability.Traces.ServiceName = value
	}
	if value := os.Getenv("POLARIS_OTEL_INSECURE"); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("parse POLARIS_OTEL_INSECURE: %w", err)
		}
		cfg.Observability.Traces.Insecure = parsed
	}
	if value := os.Getenv("POLARIS_OTEL_SAMPLE_RATIO"); value != "" {
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("parse POLARIS_OTEL_SAMPLE_RATIO: %w", err)
		}
		cfg.Observability.Traces.SampleRatio = parsed
	}
	return nil
}

func ApplyRuntimeOverrides(cfg *Config, overrides RuntimeOverrides) {
	if overrides.Port > 0 {
		cfg.Server.Port = overrides.Port
	}
	if overrides.LogLevel != "" {
		cfg.Observability.Logging.Level = overrides.LogLevel
	}
}

func ExpandEnv(input string) (string, []string) {
	return expandEnv(input)
}

func expandEnv(input string) (string, []string) {
	var warnings []string
	seen := map[string]struct{}{}
	expanded := envPattern.ReplaceAllStringFunc(input, func(match string) string {
		parts := envPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		name := parts[1]
		value, ok := os.LookupEnv(name)
		if !ok {
			if _, exists := seen[name]; !exists {
				warnings = append(warnings, fmt.Sprintf("environment variable %s is not set", name))
				seen[name] = struct{}{}
			}
			return ""
		}
		return value
	})
	slices.Sort(warnings)
	return expanded, warnings
}

func DefaultConfigPath() string {
	if path := os.Getenv("POLARIS_CONFIG"); path != "" {
		return path
	}
	return "./config/polaris.yaml"
}

func (c Config) Address() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func (m AuthMode) Valid() bool {
	switch m {
	case AuthModeNone, AuthModeStatic, AuthModeExternal, AuthModeVirtualKeys, AuthModeMultiUser:
		return true
	default:
		return false
	}
}

func normalizeLogLevel(level string) string {
	if level == "" {
		return "info"
	}
	return strings.ToLower(level)
}
