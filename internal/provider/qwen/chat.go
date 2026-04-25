package qwen

import (
	"encoding/json"

	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/openaicompat"
)

type ChatAdapter = openaicompat.ChatAdapter

type chatRequest struct {
	Model          string                    `json:"model"`
	Messages       []modality.ChatMessage    `json:"messages"`
	Temperature    *float64                  `json:"temperature,omitempty"`
	TopP           *float64                  `json:"top_p,omitempty"`
	MaxTokens      int                       `json:"max_tokens,omitempty"`
	Stream         bool                      `json:"stream,omitempty"`
	Tools          []modality.ToolDefinition `json:"tools,omitempty"`
	ToolChoice     json.RawMessage           `json:"tool_choice,omitempty"`
	ResponseFormat *modality.ResponseFormat  `json:"response_format,omitempty"`
	Stop           []string                  `json:"stop,omitempty"`
	StreamOptions  *streamOptions            `json:"stream_options,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

func NewChatAdapter(client *Client, model string) *ChatAdapter {
	return openaicompat.NewChatAdapter(client, model, translateRequest)
}

func translateRequest(req *modality.ChatRequest, stream bool, providerModel string) any {
	payload := chatRequest{
		Model:          providerModel,
		Messages:       append([]modality.ChatMessage(nil), req.Messages...),
		Temperature:    req.Temperature,
		TopP:           req.TopP,
		MaxTokens:      req.MaxTokens,
		Stream:         stream,
		Tools:          append([]modality.ToolDefinition(nil), req.Tools...),
		ToolChoice:     req.ToolChoice,
		ResponseFormat: req.ResponseFormat,
		Stop:           append([]string(nil), req.Stop...),
	}
	if stream {
		payload.StreamOptions = &streamOptions{IncludeUsage: true}
	}
	return payload
}
