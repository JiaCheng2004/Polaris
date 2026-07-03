package anthropic

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/anthropiccompat"
)

const anthropicVersion = "2023-06-01"

// Client is the native Anthropic client: the shared Anthropic-compatible
// transport configured with Anthropic's x-api-key + anthropic-version auth
// headers. Per-request anthropic-beta headers are supplied by the chat adapter
// through the compat client's JSONWithBetas/StreamWithBetas.
type Client struct {
	*anthropiccompat.Client
}

func NewClient(cfg config.ProviderConfig) *Client {
	staticHeaders := map[string]string{
		"x-api-key":         cfg.APIKey,
		"anthropic-version": anthropicVersion,
	}
	return &Client{Client: anthropiccompat.NewClient("anthropic", "Anthropic", cfg, "https://api.anthropic.com", staticHeaders)}
}
