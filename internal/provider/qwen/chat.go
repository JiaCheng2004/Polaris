package qwen

import (
	"encoding/json"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/chattools"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/openaicompat"
)

type ChatAdapter = openaicompat.ChatAdapter

type chatRequest struct {
	Model          string                   `json:"model"`
	Messages       []modality.ChatMessage   `json:"messages"`
	Temperature    *float64                 `json:"temperature,omitempty"`
	TopP           *float64                 `json:"top_p,omitempty"`
	MaxTokens      int                      `json:"max_tokens,omitempty"`
	Stream         bool                     `json:"stream,omitempty"`
	Tools          []map[string]any         `json:"tools,omitempty"`
	ToolChoice     json.RawMessage          `json:"tool_choice,omitempty"`
	ResponseFormat *modality.ResponseFormat `json:"response_format,omitempty"`
	Stop           []string                 `json:"stop,omitempty"`
	StreamOptions  *streamOptions           `json:"stream_options,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

func NewChatAdapter(client *Client, model string) *ChatAdapter {
	return openaicompat.NewChatAdapter(client, model, translateRequest)
}

func translateRequest(req *modality.ChatRequest, stream bool, providerModel string) any {
	messages := translateFileExtractMessages(req.Messages)
	payload := chatRequest{
		Model:          providerModel,
		Messages:       messages,
		Temperature:    req.Temperature,
		TopP:           req.TopP,
		MaxTokens:      req.MaxTokens,
		Stream:         stream,
		Tools:          chattools.OpenAITools(req.Tools),
		ToolChoice:     req.ToolChoice,
		ResponseFormat: req.ResponseFormat,
		Stop:           append([]string(nil), req.Stop...),
	}
	if stream {
		payload.StreamOptions = &streamOptions{IncludeUsage: true}
	}
	return payload
}

func translateFileExtractMessages(messages []modality.ChatMessage) []modality.ChatMessage {
	translated := append([]modality.ChatMessage(nil), messages...)
	var fileIDs []string
	for messageIndex := range translated {
		message := &translated[messageIndex]
		if len(message.Content.Parts) == 0 {
			continue
		}
		parts := make([]modality.ContentPart, 0, len(message.Content.Parts))
		for _, part := range message.Content.Parts {
			if part.Type != "file" && part.Type != "document" {
				parts = append(parts, part)
				continue
			}
			filePart := part.File
			if part.Type == "document" {
				filePart = part.Document
			}
			if filePart != nil && strings.TrimSpace(filePart.FileID) != "" {
				fileIDs = append(fileIDs, "fileid://"+strings.TrimSpace(filePart.FileID))
			}
		}
		message.Content.Parts = parts
	}
	if len(fileIDs) == 0 {
		return translated
	}
	prefix := modality.ChatMessage{
		Role:    "system",
		Content: modality.NewTextContent(strings.Join(fileIDs, "\n")),
	}
	return append([]modality.ChatMessage{prefix}, translated...)
}
