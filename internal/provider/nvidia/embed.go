package nvidia

import "github.com/JiaCheng2004/Polaris/internal/provider/common/openaicompat"

type EmbedAdapter = openaicompat.EmbedAdapter

func NewEmbedAdapter(client *Client, model string) *EmbedAdapter {
	return openaicompat.NewEmbedAdapter(client, model)
}
