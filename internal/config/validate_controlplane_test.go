package config

import (
	"strings"
	"testing"
)

func TestValidateRejectsVirtualKeysWithoutBootstrapAdmin(t *testing.T) {
	cfg := Default()
	cfg.Auth.Mode = AuthModeVirtualKeys

	err := Validate(&cfg)
	if err == nil || !strings.Contains(err.Error(), "auth.bootstrap_admin_key_hash is required when auth.mode=virtual_keys") {
		t.Fatalf("expected bootstrap admin hash validation error, got %v", err)
	}
}

func TestValidateRejectsEnabledControlPlaneWithoutBootstrapAdmin(t *testing.T) {
	cfg := Default()
	cfg.ControlPlane.Enabled = true

	err := Validate(&cfg)
	if err == nil || !strings.Contains(err.Error(), "auth.bootstrap_admin_key_hash is required when control_plane.enabled=true") {
		t.Fatalf("expected control plane bootstrap validation error, got %v", err)
	}
}

func TestValidateAcceptsVirtualKeysWithTracingAndLocalTools(t *testing.T) {
	cfg := Default()
	cfg.Auth.Mode = AuthModeVirtualKeys
	cfg.Auth.BootstrapAdminKeyHash = "sha256:test"
	cfg.ControlPlane.Enabled = true
	cfg.Tools.Enabled = true
	cfg.Tools.Local = map[string]LocalToolConfig{
		"echo": {Implementation: "echo"},
	}
	cfg.MCP.Enabled = true
	cfg.Observability.Traces.Enabled = true
	cfg.Observability.Traces.Endpoint = "http://otel.example/v1/traces"
	cfg.Observability.Traces.ServiceName = "polaris"
	cfg.Observability.Traces.SampleRatio = 1

	if err := Validate(&cfg); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsPartialByteDanceControlPlaneCredentials(t *testing.T) {
	cfg := Default()
	cfg.Providers["bytedance"] = ProviderConfig{
		AccessKeyID: "ak-only",
		Timeout:     30,
		Models: map[string]ModelConfig{
			"doubao-tts-2.0": {
				Modality: "voice",
			},
		},
	}

	err := Validate(&cfg)
	if err == nil || !strings.Contains(err.Error(), "providers.bytedance.access_key_id and providers.bytedance.access_key_secret are required together") {
		t.Fatalf("expected ByteDance control-plane credential validation error, got %v", err)
	}
}
