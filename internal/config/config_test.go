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
version: 2
runtime:
  server:
    host: 127.0.0.1
  auth:
    mode: none
  observability:
    logging:
      format: json
providers:
  openai:
    credentials:
      api_key: ${OPENAI_API_KEY}
    transport:
      base_url: https://api.openai.com/v1
      timeout: 60s
    models:
      use: [gpt-4o]
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
version: 2
runtime:
  server:
    host: 127.0.0.1
  auth:
    mode: none
  observability:
    logging:
      format: json
providers:
  openai:
    credentials:
      api_key: sk-test
    transport:
      base_url: https://api.openai.com/v1
      timeout: 60s
    models:
      use: [gpt-4o]
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
version: 2
runtime:
  server:
    host: 127.0.0.1
  auth:
    mode: none
  observability:
    logging:
      format: json
providers:
  openai:
    credentials:
      api_key: ${MISSING_OPENAI_KEY}
    transport:
      base_url: https://api.openai.com/v1
      timeout: 60s
    models:
      use: [gpt-4o]
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
version: 2
runtime:
  server:
    host: 127.0.0.1
  auth:
    mode: none
  cache:
    driver: redis
  observability:
    logging:
      format: json
providers:
  openai:
    credentials:
      api_key: sk-test
    transport:
      base_url: https://api.openai.com/v1
      timeout: 60s
    models:
      use: [gpt-4o]
`)

	_, _, err := Load(configPath)
	if err == nil || !strings.Contains(err.Error(), "cache.url is required") {
		t.Fatalf("expected redis cache url validation error, got %v", err)
	}
}

func TestLoadRejectsWildcardCORSWithCredentials(t *testing.T) {
	configPath := writeTempConfig(t, `
version: 2
runtime:
  server:
    host: 127.0.0.1
    cors:
      enabled: true
      allowed_origins: ["*"]
      allow_credentials: true
  auth:
    mode: none
  observability:
    logging:
      format: json
providers:
  openai:
    credentials:
      api_key: sk-test
    transport:
      base_url: https://api.openai.com/v1
      timeout: 60s
    models:
      use: [gpt-4o]
`)

	_, _, err := Load(configPath)
	if err == nil || !strings.Contains(err.Error(), "allow_credentials cannot be true") {
		t.Fatalf("expected wildcard CORS validation error, got %v", err)
	}
}

func TestLoadExpandsRedisURLFromEnvironment(t *testing.T) {
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")

	configPath := writeTempConfig(t, `
version: 2
runtime:
  server:
    host: 127.0.0.1
  auth:
    mode: none
  cache:
    driver: redis
    url: ${REDIS_URL}
  observability:
    logging:
      format: json
providers:
  openai:
    credentials:
      api_key: sk-test
    transport:
      base_url: https://api.openai.com/v1
      timeout: 60s
    models:
      use: [gpt-4o]
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

func TestLoadMergesImportsAndRootOverrides(t *testing.T) {
	dir := t.TempDir()
	importPath := filepath.Join(dir, "provider.yaml")
	if err := os.WriteFile(importPath, []byte(strings.TrimSpace(`
version: 2
providers:
  openai:
    credentials:
      api_key: sk-imported
    transport:
      base_url: https://imported.example/v1
      timeout: 30s
    models:
      use: [gpt-4o-mini]
`)), 0o600); err != nil {
		t.Fatalf("write imported config: %v", err)
	}
	rootPath := filepath.Join(dir, "polaris.yaml")
	if err := os.WriteFile(rootPath, []byte(strings.TrimSpace(`
version: 2
imports:
  - ./provider.yaml
runtime:
  server:
    host: 127.0.0.1
  auth:
    mode: none
providers:
  openai:
    transport:
      timeout: 90s
    models:
      use: [gpt-4o]
`)), 0o600); err != nil {
		t.Fatalf("write root config: %v", err)
	}

	cfg, warnings, err := Load(rootPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	provider := cfg.Providers["openai"]
	if provider.APIKey != "sk-imported" {
		t.Fatalf("expected imported credential, got %q", provider.APIKey)
	}
	if provider.BaseURL != "https://imported.example/v1" {
		t.Fatalf("expected imported base_url, got %q", provider.BaseURL)
	}
	if provider.Timeout.String() != "1m30s" {
		t.Fatalf("expected root timeout override, got %s", provider.Timeout)
	}
	if _, ok := provider.Models["gpt-4o"]; !ok {
		t.Fatalf("expected root model use list to replace imported models")
	}
	if _, ok := provider.Models["gpt-4o-mini"]; ok {
		t.Fatalf("did not expect imported model use list after root override")
	}
}

func TestLoadRejectsImportCycles(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first.yaml")
	secondPath := filepath.Join(dir, "second.yaml")
	if err := os.WriteFile(firstPath, []byte("version: 2\nimports:\n  - ./second.yaml\n"), 0o600); err != nil {
		t.Fatalf("write first config: %v", err)
	}
	if err := os.WriteFile(secondPath, []byte("version: 2\nimports:\n  - ./first.yaml\n"), 0o600); err != nil {
		t.Fatalf("write second config: %v", err)
	}

	_, _, err := Load(firstPath)
	if err == nil || !strings.Contains(err.Error(), "import cycle") {
		t.Fatalf("expected import cycle error, got %v", err)
	}
}

func TestLoadExpandsCatalogModelRefs(t *testing.T) {
	configPath := writeTempConfig(t, `
version: 2
runtime:
  server:
    host: 127.0.0.1
  auth:
    mode: none
providers:
  anthropic:
    credentials:
      api_key: sk-test
    transport:
      base_url: https://api.anthropic.com
      timeout: 60s
    models:
      use: [claude-sonnet-4-6]
`)

	cfg, _, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	model := cfg.Providers["anthropic"].Models["claude-sonnet-4-6"]
	if model.Modality != "chat" {
		t.Fatalf("expected catalog modality chat, got %q", model.Modality)
	}
	if len(model.Capabilities) == 0 {
		t.Fatalf("expected catalog capabilities")
	}
}

func TestLoadExpandsDeepSeekV4CatalogModelRefs(t *testing.T) {
	configPath := writeTempConfig(t, `
version: 2
runtime:
  server:
    host: 127.0.0.1
  auth:
    mode: none
providers:
  deepseek:
    credentials:
      api_key: sk-test
    transport:
      base_url: https://api.deepseek.com/v1
      timeout: 60s
    models:
      use: [deepseek-v4-flash, deepseek-v4-pro]
`)

	cfg, _, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for _, name := range []string{"deepseek-v4-flash", "deepseek-v4-pro"} {
		model := cfg.Providers["deepseek"].Models[name]
		if model.Modality != "chat" {
			t.Fatalf("%s modality = %q, want chat", name, model.Modality)
		}
		if model.ContextWindow != 1000000 {
			t.Fatalf("%s context_window = %d, want 1000000", name, model.ContextWindow)
		}
		if model.MaxOutputTokens != 384000 {
			t.Fatalf("%s max_output_tokens = %d, want 384000", name, model.MaxOutputTokens)
		}
		if !modelHasCapability(model, "reasoning") {
			t.Fatalf("%s missing reasoning capability: %#v", name, model.Capabilities)
		}
	}
}

func TestLoadRejectsUnknownModelRefWithoutModality(t *testing.T) {
	configPath := writeTempConfig(t, `
version: 2
runtime:
  server:
    host: 127.0.0.1
  auth:
    mode: none
providers:
  openai:
    credentials:
      api_key: sk-test
    transport:
      base_url: https://api.openai.com/v1
      timeout: 60s
    models:
      use: [private-model]
      overrides:
        private-model:
          capabilities: [streaming]
`)

	_, _, err := Load(configPath)
	if err == nil || !strings.Contains(err.Error(), "must define overrides.private-model.modality") {
		t.Fatalf("expected unknown model modality error, got %v", err)
	}
}

func TestLoadAllowsUnknownModelRefWithOverrideModality(t *testing.T) {
	configPath := writeTempConfig(t, `
version: 2
runtime:
  server:
    host: 127.0.0.1
  auth:
    mode: none
providers:
  openai:
    credentials:
      api_key: sk-test
    transport:
      base_url: https://api.openai.com/v1
      timeout: 60s
    models:
      use: [private-model]
      overrides:
        private-model:
          modality: chat
          capabilities: [streaming]
`)

	cfg, _, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Providers["openai"].Models["private-model"].Modality != "chat" {
		t.Fatalf("expected custom model to load")
	}
}

func TestConfigFilesReturnsRootAndImports(t *testing.T) {
	dir := t.TempDir()
	importPath := filepath.Join(dir, "provider.yaml")
	if err := os.WriteFile(importPath, []byte("version: 2\n"), 0o600); err != nil {
		t.Fatalf("write imported config: %v", err)
	}
	rootPath := filepath.Join(dir, "polaris.yaml")
	if err := os.WriteFile(rootPath, []byte("version: 2\nimports:\n  - ./provider.yaml\n"), 0o600); err != nil {
		t.Fatalf("write root config: %v", err)
	}

	files, err := ConfigFiles(rootPath)
	if err != nil {
		t.Fatalf("ConfigFiles() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected root plus one import, got %v", files)
	}
	if files[0] != filepath.Clean(rootPath) || files[1] != filepath.Clean(importPath) {
		t.Fatalf("unexpected files: %v", files)
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

func modelHasCapability(model ModelConfig, capability string) bool {
	for _, item := range model.Capabilities {
		if string(item) == capability {
			return true
		}
	}
	return false
}
