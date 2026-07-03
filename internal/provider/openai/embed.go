package openai

import (
	"github.com/JiaCheng2004/Polaris/internal/provider/common/openaicompat"
)

// NewEmbedAdapter builds the OpenAI embeddings adapter on the shared compat
// implementation (OpenAI's /embeddings surface is the canonical one).
func NewEmbedAdapter(client *Client, model string) *openaicompat.EmbedAdapter {
	return openaicompat.NewEmbedAdapter(client.Client, model)
}
