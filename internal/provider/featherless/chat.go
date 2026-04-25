package featherless

import "github.com/JiaCheng2004/Polaris/internal/provider/common/openaicompat"

type ChatAdapter = openaicompat.ChatAdapter

func NewChatAdapter(client *Client, model string) *ChatAdapter {
	return openaicompat.NewChatAdapter(client, model, nil)
}
