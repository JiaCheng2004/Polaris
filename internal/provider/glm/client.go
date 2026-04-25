package glm

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/openaicompat"
)

type Client = openaicompat.Client

func NewClient(cfg config.ProviderConfig) *Client {
	return openaicompat.NewClient("glm", "GLM", cfg, "https://open.bigmodel.cn/api/paas/v4", nil)
}
