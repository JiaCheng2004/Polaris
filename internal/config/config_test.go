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

	ApplyRuntimeOverrides(cfg, RuntimeOverrides{Port: 9100, LogLevel: "warn"})
	if cfg.Server.Port != 9100 {
		t.Fatalf("expected runtime override port 9100, got %d", cfg.Server.Port)
	}
	if cfg.Observability.Logging.Level != "warn" {
		t.Fatalf("expected runtime override log level warn, got %q", cfg.Observability.Logging.Level)
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

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "polaris.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}
