package zaitoken

import "github.com/JiaCheng2004/Polaris/internal/provider/common/anthropiccompat"

type NativeMessagesAdapter = anthropiccompat.NativeMessagesAdapter

func NewNativeMessagesAdapter(client *Client, model string) *NativeMessagesAdapter {
	return anthropiccompat.NewNativeMessagesAdapter(client, model)
}
