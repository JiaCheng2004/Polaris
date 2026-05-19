package minimaxtoken

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/anthropiccompat"
)

type Client = anthropiccompat.Client

func NewClient(cfg config.ProviderConfig) *Client {
	return anthropiccompat.NewClient(
		"minimax-token",
		"MiniMax Token Plan",
		cfg,
		"https://api.minimax.io/anthropic",
		map[string]string{
			"Authorization":     "Bearer " + cfg.APIKey,
			"anthropic-version": "2023-06-01",
		},
	)
}
