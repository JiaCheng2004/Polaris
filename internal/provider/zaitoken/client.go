package zaitoken

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/anthropiccompat"
)

type Client = anthropiccompat.Client

func NewClient(cfg config.ProviderConfig) *Client {
	return anthropiccompat.NewClient(
		"zai-token",
		"Z.ai Token Plan",
		cfg,
		"https://api.z.ai/api/anthropic",
		map[string]string{
			"x-api-key":         cfg.APIKey,
			"anthropic-version": "2023-06-01",
		},
	)
}
