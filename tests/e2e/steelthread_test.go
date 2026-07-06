//go:build steelthread

// Steel-thread live validation: for each provider with a sandbox key present in
// the environment, build the real registry + adapter and make one live chat call
// against a current model, proving base URL, auth, request translation, and
// response parsing all work end to end. Opt-in (build tag + env-gated):
//
//	set -a; . ./.env; set +a
//	go test -tags steelthread ./tests/e2e -run TestSteelThreadChat -v
package e2e

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
)

func TestSteelThreadChat(t *testing.T) {
	cases := []struct {
		provider string
		envKey   string
		model    string
		baseURL  string
	}{
		{"openai", "OPENAI_API_KEY", "gpt-4o", ""},
		{"anthropic", "ANTHROPIC_API_KEY", "claude-opus-4-8", ""},
		{"deepseek", "DEEPSEEK_API_KEY", "deepseek-v4-flash", ""},
		{"xai", "XAI_API_KEY", "grok-4.20-0309-non-reasoning", ""},
		{"google", "GOOGLE_API_KEY", "gemini-2.5-flash", ""},
		{"bytedance", "VOLCENGINE_ARK_API_KEY", "doubao-1-5-pro-32k-250115", ""},
		// minimax is a music/audio/video provider in Polaris (chat was the pruned
		// token-plan variant); its key + endpoint are validated separately.
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.provider, func(t *testing.T) {
			key := strings.TrimSpace(os.Getenv(tc.envKey))
			if key == "" {
				t.Skipf("%s not set", tc.envKey)
			}
			pc := config.ProviderConfig{
				APIKey:  key,
				BaseURL: tc.baseURL,
				Timeout: 90 * time.Second,
				Models: map[string]config.ModelConfig{
					tc.model: {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming},
					},
				},
			}
			cfg := &config.Config{Providers: map[string]config.ProviderConfig{tc.provider: pc}}
			registry, _, err := provider.New(cfg)
			if err != nil {
				t.Fatalf("provider.New: %v", err)
			}
			modelID := tc.provider + "/" + tc.model
			adapter, _, err := registry.GetChatAdapter(modelID)
			if err != nil {
				t.Fatalf("GetChatAdapter(%s): %v", modelID, err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			resp, err := adapter.Complete(ctx, &modality.ChatRequest{
				Model:     modelID,
				MaxTokens: 32,
				Messages:  []modality.ChatMessage{{Role: "user", Content: modality.NewTextContent("Reply with exactly the word: OK")}},
			})
			if err != nil {
				t.Fatalf("%s live chat failed: %v", tc.provider, err)
			}
			if len(resp.Choices) == 0 {
				t.Fatalf("%s: response had no choices", tc.provider)
			}
			t.Logf("PASS %s/%s — reply=%q, tokens in/out=%d/%d",
				tc.provider, tc.model, steelThreadText(resp), resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
		})
	}
}

func steelThreadText(resp *modality.ChatResponse) string {
	if len(resp.Choices) == 0 {
		return ""
	}
	c := resp.Choices[0].Message.Content
	if c.Text != nil {
		return strings.TrimSpace(*c.Text)
	}
	var b strings.Builder
	for _, p := range c.Parts {
		if p.Type == "text" {
			b.WriteString(p.Text)
		}
	}
	return strings.TrimSpace(b.String())
}
