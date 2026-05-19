package zaitoken

import "github.com/JiaCheng2004/Polaris/internal/provider/common/anthropiccompat"

type ChatAdapter = anthropiccompat.ChatAdapter

func NewChatAdapter(client *Client, model string, defaultMaxTokens int) *ChatAdapter {
	return anthropiccompat.NewChatAdapter(client, model, defaultMaxTokens, nil)
}
