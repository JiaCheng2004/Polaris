package openrouter

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/openaicompat"
)

type Client = openaicompat.Client

func NewClient(cfg config.ProviderConfig) *Client {
	return openaicompat.NewClient("openrouter", "OpenRouter", cfg, "https://openrouter.ai/api/v1", nil)
}
