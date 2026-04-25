package groq

import (
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/openaicompat"
)

type Client = openaicompat.Client

func NewClient(cfg config.ProviderConfig) *Client {
	return openaicompat.NewClient("groq", "Groq", cfg, "https://api.groq.com/openai/v1", nil)
}
