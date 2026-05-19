package minimaxtoken

import "github.com/JiaCheng2004/Polaris/internal/provider/common/anthropiccompat"

type ChatAdapter = anthropiccompat.ChatAdapter

func NewChatAdapter(client *Client, model string, defaultMaxTokens int) *ChatAdapter {
	return anthropiccompat.NewChatAdapterWithModelMapper(client, model, defaultMaxTokens, nil, WireModelName)
}
