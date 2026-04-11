package config

import (
	"errors"
	"fmt"
	"strings"
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

	if !cfg.Auth.Mode.Valid() {
		problems = append(problems, fmt.Errorf("auth.mode %q is invalid", cfg.Auth.Mode))
	}
	if cfg.Auth.Mode == AuthModeStatic && len(cfg.Auth.StaticKeys) == 0 {
		problems = append(problems, errors.New("auth.static_keys must contain at least one key in static mode"))
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
	if cfg.Cache.RateLimit.Window != "" && cfg.Cache.RateLimit.Window != "sliding" {
		problems = append(problems, fmt.Errorf("cache.rate_limit.window %q is invalid", cfg.Cache.RateLimit.Window))
	}
	if cfg.Cache.ResponseCache.TTL < 0 {
		problems = append(problems, errors.New("cache.response_cache.ttl must not be negative"))
	}

	for providerName, provider := range cfg.Providers {
		if len(provider.Models) == 0 {
			problems = append(problems, fmt.Errorf("providers.%s.models must not be empty", providerName))
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

	return errors.Join(problems...)
}
