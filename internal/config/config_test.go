package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAppliesEnvAndRuntimeOverrides(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("POLARIS_PORT", "9001")
	t.Setenv("POLARIS_LOG_LEVEL", "debug")
	t.Setenv("POLARIS_EXTERNAL_AUTH_SECRET", "external-secret")

	configPath := writeTempConfig(t, `
server:
  host: 127.0.0.1
auth:
  mode: none
providers:
  openai:
    api_key: ${OPENAI_API_KEY}
    base_url: https://api.openai.com/v1
    timeout: 60s
    models:
      gpt-4o:
        modality: chat
        capabilities: [streaming]
observability:
  logging:
    format: json
`)

	cfg, warnings, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if cfg.Server.Port != 9001 {
		t.Fatalf("expected env override port 9001, got %d", cfg.Server.Port)
	}
	if cfg.Observability.Logging.Level != "debug" {
		t.Fatalf("expected env override log level debug, got %q", cfg.Observability.Logging.Level)
	}
	if cfg.Providers["openai"].APIKey != "sk-test" {
		t.Fatalf("expected env expansion for provider api key")
	}
	if cfg.Auth.External.SharedSecret != "external-secret" {
		t.Fatalf("expected external auth secret env override")
	}

	ApplyRuntimeOverrides(cfg, RuntimeOverrides{Port: 9100, LogLevel: "warn"})
	if cfg.Server.Port != 9100 {
		t.Fatalf("expected runtime override port 9100, got %d", cfg.Server.Port)
	}
	if cfg.Observability.Logging.Level != "warn" {
		t.Fatalf("expected runtime override log level warn, got %q", cfg.Observability.Logging.Level)
	}
}

func TestLoadCanEnableExternalAuthFromEnvironment(t *testing.T) {
	t.Setenv("POLARIS_AUTH_MODE", "external")
	t.Setenv("POLARIS_EXTERNAL_AUTH_SECRET", "external-secret")
	t.Setenv("POLARIS_EXTERNAL_AUTH_MAX_CLOCK_SKEW", "30s")
	t.Setenv("POLARIS_EXTERNAL_AUTH_CACHE_TTL", "15s")

	configPath := writeTempConfig(t, `
server:
  host: 127.0.0.1
auth:
  mode: none
providers:
  openai:
    api_key: sk-test
    base_url: https://api.openai.com/v1
    timeout: 60s
    models:
      gpt-4o:
        modality: chat
        capabilities: [streaming]
observability:
  logging:
    format: json
`)

	cfg, warnings, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if cfg.Auth.Mode != AuthModeExternal {
		t.Fatalf("expected external auth mode, got %q", cfg.Auth.Mode)
	}
	if cfg.Auth.External.Provider != "signed_headers" || cfg.Auth.External.SharedSecret != "external-secret" {
		t.Fatalf("unexpected external auth config: %#v", cfg.Auth.External)
	}
	if cfg.Auth.External.MaxClockSkew.String() != "30s" || cfg.Auth.External.CacheTTL.String() != "15s" {
		t.Fatalf("unexpected external auth durations: %#v", cfg.Auth.External)
	}
}

func TestLoadWarnsOnMissingEnvironmentVariables(t *testing.T) {
	configPath := writeTempConfig(t, `
server:
  host: 127.0.0.1
auth:
  mode: none
providers:
  openai:
    api_key: ${MISSING_OPENAI_KEY}
    base_url: https://api.openai.com/v1
    timeout: 60s
    models:
      gpt-4o:
        modality: chat
        capabilities: [streaming]
observability:
  logging:
    format: json
`)

	_, warnings, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d (%v)", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "MISSING_OPENAI_KEY") {
		t.Fatalf("unexpected warning: %v", warnings[0])
	}
}

func TestLoadRequiresRedisURLWhenRedisCacheIsEnabled(t *testing.T) {
	configPath := writeTempConfig(t, `
server:
  host: 127.0.0.1
auth:
  mode: none
cache:
  driver: redis
providers:
  openai:
    api_key: sk-test
    base_url: https://api.openai.com/v1
    timeout: 60s
    models:
      gpt-4o:
        modality: chat
        capabilities: [streaming]
observability:
  logging:
    format: json
`)

	_, _, err := Load(configPath)
	if err == nil || !strings.Contains(err.Error(), "cache.url is required") {
		t.Fatalf("expected redis cache url validation error, got %v", err)
	}
}

func TestLoadExpandsRedisURLFromEnvironment(t *testing.T) {
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")

	configPath := writeTempConfig(t, `
server:
  host: 127.0.0.1
auth:
  mode: none
cache:
  driver: redis
  url: ${REDIS_URL}
providers:
  openai:
    api_key: sk-test
    base_url: https://api.openai.com/v1
    timeout: 60s
    models:
      gpt-4o:
        modality: chat
        capabilities: [streaming]
observability:
  logging:
    format: json
`)

	cfg, warnings, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if cfg.Cache.URL != "redis://localhost:6379/0" {
		t.Fatalf("expected redis url expansion, got %q", cfg.Cache.URL)
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "polaris.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}
