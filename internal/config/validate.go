package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func Validate(cfg *Config) error {
	var problems []error

	if cfg.Server.Host == "" {
		problems = append(problems, errors.New("server.host is required"))
	}
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		problems = append(problems, fmt.Errorf("server.port must be between 1 and 65535, got %d", cfg.Server.Port))
	}
	if cfg.Server.ReadTimeout <= 0 {
		problems = append(problems, errors.New("server.read_timeout must be greater than zero"))
	}
	if cfg.Server.WriteTimeout <= 0 {
		problems = append(problems, errors.New("server.write_timeout must be greater than zero"))
	}
	if cfg.Server.ShutdownTimeout <= 0 {
		problems = append(problems, errors.New("server.shutdown_timeout must be greater than zero"))
	}
	if cfg.Server.MaxBodyBytes <= 0 {
		problems = append(problems, errors.New("server.max_body_bytes must be greater than zero"))
	}
	if cfg.Server.CORS.Enabled {
		if len(cfg.Server.CORS.AllowedOrigins) == 0 {
			problems = append(problems, errors.New("server.cors.allowed_origins must not be empty when cors is enabled"))
		}
		if len(cfg.Server.CORS.AllowedMethods) == 0 {
			problems = append(problems, errors.New("server.cors.allowed_methods must not be empty when cors is enabled"))
		}
		if len(cfg.Server.CORS.AllowedHeaders) == 0 {
			problems = append(problems, errors.New("server.cors.allowed_headers must not be empty when cors is enabled"))
		}
		if cfg.Server.CORS.AllowCredentials {
			for _, origin := range cfg.Server.CORS.AllowedOrigins {
				if strings.TrimSpace(origin) == "*" {
					problems = append(problems, errors.New("server.cors.allow_credentials cannot be true when allowed_origins contains *"))
				}
			}
		}
		if cfg.Server.CORS.MaxAge < 0 {
			problems = append(problems, errors.New("server.cors.max_age must not be negative"))
		}
		for _, method := range cfg.Server.CORS.AllowedMethods {
			if strings.TrimSpace(method) == "" {
				problems = append(problems, errors.New("server.cors.allowed_methods must not contain empty values"))
			}
		}
		for _, header := range cfg.Server.CORS.AllowedHeaders {
			if strings.TrimSpace(header) == "" {
				problems = append(problems, errors.New("server.cors.allowed_headers must not contain empty values"))
			}
		}
		for _, origin := range cfg.Server.CORS.AllowedOrigins {
			if strings.TrimSpace(origin) == "" {
				problems = append(problems, errors.New("server.cors.allowed_origins must not contain empty values"))
			}
		}
	}

	if !cfg.Auth.Mode.Valid() {
		problems = append(problems, fmt.Errorf("auth.mode %q is invalid", cfg.Auth.Mode))
	}
	if cfg.Auth.Mode == AuthModeStatic && len(cfg.Auth.StaticKeys) == 0 {
		problems = append(problems, errors.New("auth.static_keys must contain at least one key in static mode"))
	}
	if cfg.Auth.Mode == AuthModeExternal {
		provider := strings.TrimSpace(cfg.Auth.External.Provider)
		if provider == "" {
			problems = append(problems, errors.New("auth.external.provider is required when auth.mode=external"))
		} else if provider != "signed_headers" {
			problems = append(problems, fmt.Errorf("auth.external.provider %q is invalid; expected signed_headers", cfg.Auth.External.Provider))
		}
		if strings.TrimSpace(cfg.Auth.External.SharedSecret) == "" {
			problems = append(problems, errors.New("auth.external.shared_secret is required when auth.mode=external"))
		}
		if cfg.Auth.External.MaxClockSkew <= 0 {
			problems = append(problems, errors.New("auth.external.max_clock_skew must be greater than zero"))
		}
		if cfg.Auth.External.CacheTTL < 0 {
			problems = append(problems, errors.New("auth.external.cache_ttl must not be negative"))
		}
	}
	if cfg.ControlPlane.Enabled {
		switch cfg.Auth.Mode {
		case AuthModeVirtualKeys, AuthModeExternal, AuthModeMultiUser:
		default:
			problems = append(problems, errors.New("control_plane.enabled requires auth.mode=virtual_keys, external, or multi-user"))
		}
	}
	adminKeyHash := strings.TrimSpace(cfg.Auth.AdminKeyHash)
	if adminKeyHash != "" && !strings.HasPrefix(adminKeyHash, "sha256:") {
		problems = append(problems, errors.New("auth.admin_key_hash must use the sha256: prefix"))
	}
	bootstrapAdminKeyHash := strings.TrimSpace(cfg.Auth.BootstrapAdminKeyHash)
	if cfg.Auth.Mode == AuthModeVirtualKeys {
		if bootstrapAdminKeyHash == "" {
			problems = append(problems, errors.New("auth.bootstrap_admin_key_hash is required when auth.mode=virtual_keys"))
		} else if !strings.HasPrefix(bootstrapAdminKeyHash, "sha256:") {
			problems = append(problems, errors.New("auth.bootstrap_admin_key_hash must use the sha256: prefix"))
		}
	}
	for i, key := range cfg.Auth.StaticKeys {
		if key.Name == "" {
			problems = append(problems, fmt.Errorf("auth.static_keys[%d].name is required", i))
		}
		if !strings.HasPrefix(key.KeyHash, "sha256:") {
			problems = append(problems, fmt.Errorf("auth.static_keys[%d].key_hash must use the sha256: prefix", i))
		}
		if len(key.AllowedModels) == 0 {
			problems = append(problems, fmt.Errorf("auth.static_keys[%d].allowed_models must not be empty", i))
		}
	}

	switch cfg.Store.Driver {
	case "sqlite", "postgres":
	default:
		problems = append(problems, fmt.Errorf("store.driver %q is invalid", cfg.Store.Driver))
	}
	if cfg.Store.DSN == "" {
		problems = append(problems, errors.New("store.dsn is required"))
	}
	if cfg.Store.MaxConnections <= 0 {
		problems = append(problems, errors.New("store.max_connections must be greater than zero"))
	}
	if cfg.Store.LogRetentionDays <= 0 {
		problems = append(problems, errors.New("store.log_retention_days must be greater than zero"))
	}
	if cfg.Store.LogBufferSize <= 0 {
		problems = append(problems, errors.New("store.log_buffer_size must be greater than zero"))
	}
	if cfg.Store.LogFlushInterval <= 0 {
		problems = append(problems, errors.New("store.log_flush_interval must be greater than zero"))
	}

	switch cfg.Cache.Driver {
	case "memory", "redis":
	default:
		problems = append(problems, fmt.Errorf("cache.driver %q is invalid", cfg.Cache.Driver))
	}
	if cfg.Cache.Driver == "redis" && cfg.Cache.URL == "" {
		problems = append(problems, errors.New("cache.url is required when cache.driver=redis"))
	}
	if cfg.Cache.RateLimit.Window != "" && cfg.Cache.RateLimit.Window != "sliding" {
		problems = append(problems, fmt.Errorf("cache.rate_limit.window %q is invalid", cfg.Cache.RateLimit.Window))
	}
	if cfg.Cache.ResponseCache.TTL < 0 {
		problems = append(problems, errors.New("cache.response_cache.ttl must not be negative"))
	}
	if cfg.Cache.ResponseCache.MaxEntriesPerModel < 0 {
		problems = append(problems, errors.New("cache.response_cache.max_entries_per_model must not be negative"))
	}
	if cfg.Cache.ResponseCache.SimilarityThreshold < 0 || cfg.Cache.ResponseCache.SimilarityThreshold > 1 {
		problems = append(problems, errors.New("cache.response_cache.similarity_threshold must be between 0 and 1"))
	}
	if cfg.Pricing.ReloadIntervalSeconds < 0 {
		problems = append(problems, errors.New("pricing.reload_interval_seconds must not be negative"))
	}
	if strings.TrimSpace(cfg.Pricing.File) != "" {
		if info, err := os.Stat(cfg.Pricing.File); err != nil {
			problems = append(problems, fmt.Errorf("pricing.file must be readable: %w", err))
		} else if info.IsDir() {
			problems = append(problems, errors.New("pricing.file must be a file, not a directory"))
		}
	}

	for providerName, provider := range cfg.Providers {
		if len(provider.Models) == 0 {
			problems = append(problems, fmt.Errorf("providers.%s.models must not be empty", providerName))
		}
		if providerName == "bedrock" {
			hasAccessKeyID := strings.TrimSpace(provider.AccessKeyID) != ""
			hasAccessKeySecret := strings.TrimSpace(provider.AccessKeySecret) != ""
			if hasAccessKeyID != hasAccessKeySecret {
				problems = append(problems, errors.New("providers.bedrock.access_key_id and providers.bedrock.access_key_secret are required together"))
			}
			if hasAccessKeyID && strings.TrimSpace(provider.Location) == "" {
				problems = append(problems, errors.New("providers.bedrock.location is required when Bedrock credentials are configured"))
			}
		}
		if providerName == "bytedance" {
			hasAccessKeyID := strings.TrimSpace(provider.AccessKeyID) != ""
			hasAccessKeySecret := strings.TrimSpace(provider.AccessKeySecret) != ""
			if hasAccessKeyID != hasAccessKeySecret {
				problems = append(problems, errors.New("providers.bytedance.access_key_id and providers.bytedance.access_key_secret are required together"))
			}
		}
		if providerName == "minimax" && providerHasMusicModels(provider.Models) {
			baseURL := strings.TrimRight(strings.TrimSpace(provider.BaseURL), "/")
			switch baseURL {
			case "":
				problems = append(problems, errors.New("providers.minimax.base_url is required when MiniMax music models are configured"))
			case "https://api.minimax.io", "https://api.minimaxi.com":
			default:
				problems = append(problems, fmt.Errorf("providers.minimax.base_url %q is invalid; expected https://api.minimax.io or https://api.minimaxi.com", provider.BaseURL))
			}
		}
		for modelName, model := range provider.Models {
			if !model.Modality.Valid() {
				problems = append(problems, fmt.Errorf("providers.%s.models.%s.modality %q is invalid", providerName, modelName, model.Modality))
			}
			for _, capability := range model.Capabilities {
				if !capability.Valid() {
					problems = append(problems, fmt.Errorf("providers.%s.models.%s has invalid capability %q", providerName, modelName, capability))
				}
			}
			if model.Modality == modality.ModalityAudio {
				hasPipeline := strings.TrimSpace(model.AudioPipeline.ChatModel) != "" ||
					strings.TrimSpace(model.AudioPipeline.STTModel) != "" ||
					strings.TrimSpace(model.AudioPipeline.TTSModel) != ""
				hasRealtime := strings.TrimSpace(model.RealtimeSession.Transport) != ""
				if !hasPipeline && !hasRealtime {
					problems = append(problems, fmt.Errorf("providers.%s.models.%s must define either audio_pipeline.chat_model, stt_model, and tts_model or realtime_session.transport for audio models", providerName, modelName))
				}
				if hasPipeline && (strings.TrimSpace(model.AudioPipeline.ChatModel) == "" || strings.TrimSpace(model.AudioPipeline.STTModel) == "" || strings.TrimSpace(model.AudioPipeline.TTSModel) == "") {
					problems = append(problems, fmt.Errorf("providers.%s.models.%s.audio_pipeline.chat_model, stt_model, and tts_model are required together when audio_pipeline is configured", providerName, modelName))
				}
				if hasRealtime {
					switch strings.TrimSpace(model.RealtimeSession.Transport) {
					case "bytedance_dialog":
						if strings.TrimSpace(model.RealtimeSession.Model) == "" {
							problems = append(problems, fmt.Errorf("providers.%s.models.%s.realtime_session.model is required for bytedance_dialog audio sessions", providerName, modelName))
						}
						auth := strings.TrimSpace(model.RealtimeSession.Auth)
						switch auth {
						case "", "access_token", "api_key":
						default:
							problems = append(problems, fmt.Errorf("providers.%s.models.%s.realtime_session.auth %q is invalid", providerName, modelName, model.RealtimeSession.Auth))
						}
					case "openai_realtime":
						if providerName != "openai" {
							problems = append(problems, fmt.Errorf("providers.%s.models.%s.realtime_session.transport %q is only supported for the openai provider", providerName, modelName, model.RealtimeSession.Transport))
						}
					default:
						problems = append(problems, fmt.Errorf("providers.%s.models.%s.realtime_session.transport %q is invalid", providerName, modelName, model.RealtimeSession.Transport))
					}
				}
				if model.SessionTTL < 0 {
					problems = append(problems, fmt.Errorf("providers.%s.models.%s.session_ttl must not be negative", providerName, modelName))
				}
			}
			if model.Modality == modality.ModalityInterpreting {
				if model.SessionTTL < 0 {
					problems = append(problems, fmt.Errorf("providers.%s.models.%s.session_ttl must not be negative", providerName, modelName))
				}
			}
			if model.MinDurationMs < 0 {
				problems = append(problems, fmt.Errorf("providers.%s.models.%s.min_duration_ms must not be negative", providerName, modelName))
			}
			if model.MaxDurationMs < 0 {
				problems = append(problems, fmt.Errorf("providers.%s.models.%s.max_duration_ms must not be negative", providerName, modelName))
			}
			if model.MinDurationMs > 0 && model.MaxDurationMs > 0 && model.MinDurationMs > model.MaxDurationMs {
				problems = append(problems, fmt.Errorf("providers.%s.models.%s.min_duration_ms must be less than or equal to max_duration_ms", providerName, modelName))
			}
		}
	}

	for alias, target := range cfg.Routing.Aliases {
		if strings.TrimSpace(alias) == "" {
			problems = append(problems, errors.New("routing.aliases keys must not be empty"))
		}
		if strings.TrimSpace(target) == "" {
			problems = append(problems, fmt.Errorf("routing.aliases.%s must not be empty", alias))
		}
	}
	for alias, selector := range cfg.Routing.Selectors {
		if strings.TrimSpace(alias) == "" {
			problems = append(problems, errors.New("routing.selectors keys must not be empty"))
			continue
		}
		if !selector.Modality.Valid() {
			problems = append(problems, fmt.Errorf("routing.selectors.%s.modality %q is invalid", alias, selector.Modality))
		}
		for _, capability := range selector.Capabilities {
			if !capability.Valid() {
				problems = append(problems, fmt.Errorf("routing.selectors.%s has invalid capability %q", alias, capability))
			}
		}
		for _, status := range selector.Statuses {
			switch strings.TrimSpace(status) {
			case "ga", "preview", "experimental":
			default:
				problems = append(problems, fmt.Errorf("routing.selectors.%s has invalid status %q", alias, status))
			}
		}
		for _, class := range selector.VerificationClasses {
			switch strings.TrimSpace(class) {
			case "strict", "opt_in", "skipped":
			default:
				problems = append(problems, fmt.Errorf("routing.selectors.%s has invalid verification_class %q", alias, class))
			}
		}
		for _, providerName := range selector.Providers {
			if strings.TrimSpace(providerName) == "" {
				problems = append(problems, fmt.Errorf("routing.selectors.%s.providers must not contain empty values", alias))
			}
		}
		for _, providerName := range selector.ExcludeProviders {
			if strings.TrimSpace(providerName) == "" {
				problems = append(problems, fmt.Errorf("routing.selectors.%s.exclude_providers must not contain empty values", alias))
			}
		}
		for _, preferred := range selector.Prefer {
			if strings.TrimSpace(preferred) == "" {
				problems = append(problems, fmt.Errorf("routing.selectors.%s.prefer must not contain empty values", alias))
			}
		}
		switch strings.TrimSpace(selector.CostTier) {
		case "", modality.CostTierLow, modality.CostTierBalanced, modality.CostTierPremium:
		default:
			problems = append(problems, fmt.Errorf("routing.selectors.%s.cost_tier %q is invalid", alias, selector.CostTier))
		}
		switch strings.TrimSpace(selector.LatencyTier) {
		case "", modality.LatencyTierFast, modality.LatencyTierBalanced, modality.LatencyTierBestQuality:
		default:
			problems = append(problems, fmt.Errorf("routing.selectors.%s.latency_tier %q is invalid", alias, selector.LatencyTier))
		}
	}

	if cfg.Observability.Metrics.Path == "" {
		problems = append(problems, errors.New("observability.metrics.path is required"))
	}
	if level := normalizeLogLevel(cfg.Observability.Logging.Level); level != "debug" && level != "info" && level != "warn" && level != "error" {
		problems = append(problems, fmt.Errorf("observability.logging.level %q is invalid", cfg.Observability.Logging.Level))
	}
	if format := strings.ToLower(cfg.Observability.Logging.Format); format != "json" && format != "text" {
		problems = append(problems, fmt.Errorf("observability.logging.format %q is invalid", cfg.Observability.Logging.Format))
	}
	if cfg.Observability.Traces.SampleRatio < 0 || cfg.Observability.Traces.SampleRatio > 1 {
		problems = append(problems, errors.New("observability.traces.sample_ratio must be between 0 and 1"))
	}
	if cfg.Observability.Traces.Enabled && strings.TrimSpace(cfg.Observability.Traces.ServiceName) == "" {
		problems = append(problems, errors.New("observability.traces.service_name is required when observability.traces.enabled=true"))
	}
	if cfg.Observability.Traces.Enabled && strings.TrimSpace(cfg.Observability.Traces.Endpoint) == "" {
		problems = append(problems, errors.New("observability.traces.endpoint is required when observability.traces.enabled=true"))
	}
	for name, localTool := range cfg.Tools.Local {
		if strings.TrimSpace(name) == "" {
			problems = append(problems, errors.New("tools.local keys must not be empty"))
			continue
		}
		if strings.TrimSpace(localTool.Implementation) == "" {
			problems = append(problems, fmt.Errorf("tools.local.%s.implementation is required", name))
		}
	}

	return errors.Join(problems...)
}

func providerHasMusicModels(models map[string]ModelConfig) bool {
	for _, model := range models {
		if model.Modality == modality.ModalityMusic {
			return true
		}
	}
	return false
}
