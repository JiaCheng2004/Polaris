package config

import (
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
	Observability ObservabilityConfig       `yaml:"observability"`
}

type ServerConfig struct {
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

type AuthMode string

const (
	AuthModeNone        AuthMode = "none"
	AuthModeStatic      AuthMode = "static"
	AuthModeVirtualKeys AuthMode = "virtual_keys"
	AuthModeMultiUser   AuthMode = "multi-user"
)

type AuthConfig struct {
	Mode                  AuthMode          `yaml:"mode"`
	StaticKeys            []StaticKeyConfig `yaml:"static_keys"`
	AdminKeyHash          string            `yaml:"admin_key_hash"`
	BootstrapAdminKeyHash string            `yaml:"bootstrap_admin_key_hash"`
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
}

type AuditConfig struct {
	Enabled bool `yaml:"enabled"`
}

type RuntimeOverrides struct {
	Port     int
	LogLevel string
}

var envPattern = regexp.MustCompile(`\$\{([A-Z0-9_]+)\}`)

func Default() Config {
	return Config{
		Server: ServerConfig{
			Host:            "0.0.0.0",
			Port:            8080,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    120 * time.Second,
			ShutdownTimeout: 15 * time.Second,
		},
		Auth: AuthConfig{
			Mode:       AuthModeNone,
			StaticKeys: []StaticKeyConfig{},
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
				Enabled: true,
				Default: "60/min",
				Window:  "sliding",
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
	}
}

func Load(path string) (*Config, []string, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read config: %w", err)
	}

	expanded, warnings := expandEnv(string(data))
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
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
	if value := os.Getenv("POLARIS_LOG_LEVEL"); value != "" {
		cfg.Observability.Logging.Level = value
	}
	if value := os.Getenv("POLARIS_AUTH_MODE"); value != "" {
		cfg.Auth.Mode = AuthMode(value)
	}
	if value := os.Getenv("POLARIS_BOOTSTRAP_ADMIN_KEY_HASH"); value != "" {
		cfg.Auth.BootstrapAdminKeyHash = value
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
	case AuthModeNone, AuthModeStatic, AuthModeVirtualKeys, AuthModeMultiUser:
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
