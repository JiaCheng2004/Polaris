package bytedance

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/chattools"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/openaicompat"
)

type ChatAdapter struct {
	client   *Client
	model    string
	endpoint string
}

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

func NewChatAdapter(client *Client, model string, endpoint string) *ChatAdapter {
	return &ChatAdapter{
		client:   client,
		model:    model,
		endpoint: endpoint,
	}
}

func (a *ChatAdapter) Complete(ctx context.Context, req *modality.ChatRequest) (*modality.ChatResponse, error) {
	payload := translateRequest(req, false, a.model)

	var response modality.ChatResponse
	if err := a.client.JSON(ctx, a.endpoint, "/chat/completions", payload, &response); err != nil {
		return nil, err
	}

	openaicompat.NormalizeChatResponse(&response, req.Model, a.model)
	return &response, nil
}

func (a *ChatAdapter) Stream(ctx context.Context, req *modality.ChatRequest) (<-chan modality.ChatChunk, error) {
	payload := translateRequest(req, true, a.model)

	resp, err := a.client.Stream(ctx, a.endpoint, "/chat/completions", payload)
	if err != nil {
		return nil, err
	}

	stream := make(chan modality.ChatChunk)
	go func() {
		defer close(stream)
		defer func() {
			_ = resp.Body.Close()
		}()

		if err := openaicompat.DecodeStream("ByteDance", resp.Body, req.Model, a.model, stream); err != nil {
			stream <- modality.ChatChunk{Err: err}
		}
	}()

	return stream, nil
}

func translateRequest(req *modality.ChatRequest, stream bool, fallbackModel string) chatRequest {
	payload := chatRequest{
		Model:          providerChatModelName(req.Model, fallbackModel),
		Messages:       append([]modality.ChatMessage(nil), req.Messages...),
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

func providerModelName(requestModel string, fallbackModel string) string {
	if requestModel == "" {
		return strings.TrimPrefix(fallbackModel[strings.Index(fallbackModel, "/")+1:], "/")
	}
	if idx := strings.IndexByte(requestModel, '/'); idx >= 0 {
		return requestModel[idx+1:]
	}
	if fallbackModel != "" {
		if idx := strings.IndexByte(fallbackModel, '/'); idx >= 0 {
			return fallbackModel[idx+1:]
		}
	}
	return requestModel
}
