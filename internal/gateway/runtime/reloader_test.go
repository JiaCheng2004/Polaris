package runtime

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/provider"
)

func TestReloaderReloadsAliasesFallbacksAndRateLimits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "polaris.yaml")
	writeTestConfig(t, path, testConfigFileOptions{
		AliasTarget:      "openai/gpt-4o",
		DefaultRateLimit: "1/min",
		KeyRateLimit:     "1/min",
		StoreDSN:         "./polaris.db",
	})

	cfg, _, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	registry, warnings, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no registry warnings, got %v", warnings)
	}

	holder := NewHolder(cfg, registry)
	reloader := NewReloader(path, config.RuntimeOverrides{}, holder, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)

	writeTestConfig(t, path, testConfigFileOptions{
		AliasTarget:      "deepseek/deepseek-chat",
		DefaultRateLimit: "3/min",
		KeyRateLimit:     "5/min",
		StoreDSN:         "./polaris.db",
		Fallbacks: []string{
			"- from: openai/gpt-4o",
			"  to: [deepseek/deepseek-chat]",
			"  on: [rate_limit, timeout, server_error]",
		},
	})

	if err := reloader.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	snapshot := holder.Current()
	if snapshot.Config.Cache.RateLimit.Default != "3/min" {
		t.Fatalf("expected updated default rate limit, got %q", snapshot.Config.Cache.RateLimit.Default)
	}
	if snapshot.StaticKeys["sha256:test"].RateLimit != "5/min" {
		t.Fatalf("expected updated key rate limit, got %q", snapshot.StaticKeys["sha256:test"].RateLimit)
	}

	model, err := snapshot.Registry.ResolveModel("default-chat")
	if err != nil {
		t.Fatalf("ResolveModel(default-chat) error = %v", err)
	}
	if model.ID != "deepseek/deepseek-chat" {
		t.Fatalf("expected alias to reload to deepseek, got %q", model.ID)
	}

	fallbacks := snapshot.Registry.GetFallbacks("openai/gpt-4o")
	if len(fallbacks) != 1 || fallbacks[0] != "deepseek/deepseek-chat" {
		t.Fatalf("expected updated fallback list, got %v", fallbacks)
	}
}

func TestReloaderKeepsPreviousStateOnInvalidReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "polaris.yaml")
	writeTestConfig(t, path, testConfigFileOptions{
		AliasTarget:      "openai/gpt-4o",
		DefaultRateLimit: "1/min",
		KeyRateLimit:     "1/min",
		StoreDSN:         "./polaris.db",
	})

	cfg, _, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	registry, _, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}

	holder := NewHolder(cfg, registry)
	reloader := NewReloader(path, config.RuntimeOverrides{}, holder, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)

	if err := os.WriteFile(path, []byte(strings.TrimSpace(`
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
      level: info
      format: json
providers:
  openai:
    credentials:
      api_key: sk-openai
    transport:
      base_url: https://api.openai.com/v1
      timeout: 1s
    models:
      use: [gpt-4o]
`)), 0o600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}

	if err := reloader.Reload(); err == nil {
		t.Fatal("expected reload validation error")
	}

	snapshot := holder.Current()
	model, err := snapshot.Registry.ResolveModel("default-chat")
	if err != nil {
		t.Fatalf("ResolveModel(default-chat) error = %v", err)
	}
	if model.ID != "openai/gpt-4o" {
		t.Fatalf("expected previous alias to stay active, got %q", model.ID)
	}
	if snapshot.Config.Cache.RateLimit.Default != "1/min" {
		t.Fatalf("expected previous rate limit to stay active, got %q", snapshot.Config.Cache.RateLimit.Default)
	}
}

func TestReloaderRejectsNonReloadableChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "polaris.yaml")
	writeTestConfig(t, path, testConfigFileOptions{
		AliasTarget:      "openai/gpt-4o",
		DefaultRateLimit: "1/min",
		KeyRateLimit:     "1/min",
		StoreDSN:         "./polaris.db",
	})

	cfg, _, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	registry, _, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}

	holder := NewHolder(cfg, registry)
	reloader := NewReloader(path, config.RuntimeOverrides{}, holder, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)

	writeTestConfig(t, path, testConfigFileOptions{
		AliasTarget:      "deepseek/deepseek-chat",
		DefaultRateLimit: "3/min",
		KeyRateLimit:     "3/min",
		StoreDSN:         "./other.db",
	})

	if err := reloader.Reload(); err == nil || !strings.Contains(err.Error(), "store settings") {
		t.Fatalf("expected non-reloadable store change rejection, got %v", err)
	}
}

func TestReloaderReloadsSelectors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "polaris.yaml")
	writeTestConfig(t, path, testConfigFileOptions{
		AliasTarget:      "openai/gpt-4o",
		DefaultRateLimit: "1/min",
		KeyRateLimit:     "1/min",
		StoreDSN:         "./polaris.db",
	})

	cfg, _, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	registry, _, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}

	holder := NewHolder(cfg, registry)
	reloader := NewReloader(path, config.RuntimeOverrides{}, holder, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)

	writeTestConfig(t, path, testConfigFileOptions{
		AliasTarget:      "openai/gpt-4o",
		DefaultRateLimit: "1/min",
		KeyRateLimit:     "1/min",
		StoreDSN:         "./polaris.db",
		Selectors: []string{
			"  selectors:",
			"    intent-chat:",
			"      modality: chat",
			"      capabilities: [streaming]",
			"      providers: [openai]",
		},
	})

	if err := reloader.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	snapshot := holder.Current()
	model, err := snapshot.Registry.RequireModel("intent-chat", "chat", "streaming")
	if err != nil {
		t.Fatalf("RequireModel(intent-chat) error = %v", err)
	}
	if model.ID != "openai/gpt-4o" {
		t.Fatalf("selector resolved to %q", model.ID)
	}
}

type testConfigFileOptions struct {
	AliasTarget      string
	DefaultRateLimit string
	KeyRateLimit     string
	StoreDSN         string
	Fallbacks        []string
	Selectors        []string
}

func writeTestConfig(t *testing.T, path string, options testConfigFileOptions) {
	t.Helper()

	fallbackBlock := ""
	if len(options.Fallbacks) > 0 {
		fallbackBlock = "\n  fallbacks:\n    " + strings.Join(options.Fallbacks, "\n    ")
	}
	selectorBlock := ""
	if len(options.Selectors) > 0 {
		selectorBlock = "\n" + strings.Join(options.Selectors, "\n")
	}

	content := strings.TrimSpace(`
version: 2
runtime:
  server:
    host: 127.0.0.1
    port: 8080
    read_timeout: 30s
    write_timeout: 30s
    shutdown_timeout: 5s
  auth:
    mode: static
    static_keys:
      - name: default
        key_hash: sha256:test
        rate_limit: ` + options.KeyRateLimit + `
        allowed_models: ["*"]
  store:
    driver: sqlite
    dsn: ` + options.StoreDSN + `
    max_connections: 1
    log_retention_days: 30
    log_buffer_size: 10
    log_flush_interval: 1s
  cache:
    driver: memory
    rate_limit:
      enabled: true
      default: ` + options.DefaultRateLimit + `
      window: sliding
  observability:
    logging:
      level: info
      format: json
providers:
  openai:
    credentials:
      api_key: sk-openai
    transport:
      base_url: https://api.openai.com/v1
      timeout: 1s
    models:
      use: [gpt-4o]
      overrides:
        gpt-4o:
          capabilities: [streaming, json_mode]
  deepseek:
    credentials:
      api_key: sk-deepseek
    transport:
      base_url: https://api.deepseek.com/v1
      timeout: 1s
    models:
      use: [deepseek-chat]
      overrides:
        deepseek-chat:
          capabilities: [streaming]
routing:
  aliases:
    default-chat: ` + options.AliasTarget + fallbackBlock + selectorBlock + `
`)

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}
