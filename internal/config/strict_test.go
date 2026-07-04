package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeStrictConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "polaris.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestStrictModeRejectsUnknownKeys(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "unknown runtime section",
			body: "version: 2\nruntime:\n  serverr:\n    port: 8080\n",
			want: "runtime.serverr",
		},
		{
			name: "unknown server field",
			body: "version: 2\nruntime:\n  server:\n    porttt: 8080\n",
			want: "porttt",
		},
		{
			name: "unknown top-level key",
			body: "version: 2\nnope: true\n",
			want: "nope",
		},
		{
			name: "unknown provider credential",
			body: "version: 2\nproviders:\n  openai:\n    credentials:\n      api_keyy: sk-x\n    models:\n      use: [gpt-4o]\n",
			want: "providers.openai.credentials.api_keyy",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeStrictConfig(t, tc.body)
			if _, _, err := LoadWithOptions(path, LoadOptions{}); err == nil {
				t.Fatalf("strict load should have rejected %q", tc.want)
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err.Error(), tc.want)
			}
		})
	}
}

func TestLenientModeToleratesUnknownKeys(t *testing.T) {
	path := writeStrictConfig(t, "version: 2\nruntime:\n  serverr:\n    port: 8080\n")
	if _, _, err := LoadWithOptions(path, LoadOptions{Lenient: true}); err != nil {
		t.Fatalf("lenient load should tolerate the typo, got %v", err)
	}
}

func TestStrictModeAcceptsValidConfig(t *testing.T) {
	path := writeStrictConfig(t, "version: 2\nruntime:\n  server:\n    port: 8080\n  auth:\n    mode: none\n")
	if _, _, err := LoadWithOptions(path, LoadOptions{}); err != nil {
		t.Fatalf("strict load should accept a valid config, got %v", err)
	}
}
