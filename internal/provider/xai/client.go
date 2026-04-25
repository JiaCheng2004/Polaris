package xai

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/openaicompat"
)

type Client = openaicompat.Client

func NewClient(cfg config.ProviderConfig) *Client {
	return openaicompat.NewClient("xai", "xAI", cfg, "https://api.x.ai/v1", nil)
}
