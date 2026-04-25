package qwen

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/openaicompat"
)

type Client = openaicompat.Client

func NewClient(cfg config.ProviderConfig) *Client {
	return openaicompat.NewClient("qwen", "Qwen", cfg, "https://dashscope.aliyuncs.com/compatible-mode/v1", nil)
}
