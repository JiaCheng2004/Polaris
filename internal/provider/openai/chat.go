package openai

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/chattools"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/openaicompat"
	tiktoken "github.com/pkoukk/tiktoken-go"
)

// ChatAdapter is the shared OpenAI-compatible chat adapter plus OpenAI's two
// unique behaviors: the max_completion_tokens request translation and local
// tiktoken-based CountTokens.
type ChatAdapter struct {
	*openaicompat.ChatAdapter
	client *Client
	model  string
}

func NewChatAdapter(client *Client, model string) *ChatAdapter {
	return &ChatAdapter{
		ChatAdapter: openaicompat.NewChatAdapter(client.Client, model, translateChatRequest),
		client:      client,
		model:       model,
	}
}

func (a *ChatAdapter) CountTokens(ctx context.Context, req *modality.ChatRequest) (*modality.TokenCountResult, error) {
	_ = ctx
	model := providerModelName(req.Model, a.model)
	encoding, err := tiktoken.EncodingForModel(model)
	if err != nil {
		encoding, err = tiktoken.GetEncoding(tiktoken.MODEL_O200K_BASE)
		if err != nil {
			return nil, err
		}
	}
	total := 0
	for _, message := range req.Messages {
		total += 4
		total += len(encoding.EncodeOrdinary(message.Role))
		total += len(encoding.EncodeOrdinary(message.Name))
		total += len(encoding.EncodeOrdinary(message.ToolCallID))
		if message.Content.Text != nil {
			total += len(encoding.EncodeOrdinary(*message.Content.Text))
		}
		for _, part := range message.Content.Parts {
			total += len(encoding.EncodeOrdinary(part.Type))
			total += len(encoding.EncodeOrdinary(part.Text))
			if part.ImageURL != nil {
				total += 256
			}
			if part.InputAudio != nil {
				total += len(part.InputAudio.Data) / 128
			}
			if part.File != nil {
				total += len(encoding.EncodeOrdinary(part.File.FileID + part.File.Filename + part.File.MimeType))
			}
			if part.Document != nil {
				total += len(encoding.EncodeOrdinary(part.Document.FileID + part.Document.Filename + part.Document.MimeType))
			}
		}
		for _, toolCall := range message.ToolCalls {
			total += 8
			total += len(encoding.EncodeOrdinary(toolCall.ID))
			total += len(encoding.EncodeOrdinary(toolCall.Function.Name))
			total += len(encoding.EncodeOrdinary(toolCall.Function.Arguments))
		}
	}
	return &modality.TokenCountResult{
		InputTokens: total,
		Source:      modality.TokenCountSourceTiktoken,
		Notes:       []string{"input tokens were counted locally with tiktoken-compatible encoding"},
	}, nil
}

// translateChatRequest is the RequestTranslator passed to the compat chat
// adapter. It differs from the default only in routing max_tokens to
// max_completion_tokens for the model families that require it.
func translateChatRequest(req *modality.ChatRequest, stream bool, providerModel string) any {
	maxTokens := req.MaxTokens
	maxCompletionTokens := 0
	if openAIUsesMaxCompletionTokens(providerModel) {
		maxCompletionTokens = req.MaxTokens
		maxTokens = 0
	}
	return struct {
		Model               string                   `json:"model"`
		Messages            []modality.ChatMessage   `json:"messages"`
		Temperature         *float64                 `json:"temperature,omitempty"`
		TopP                *float64                 `json:"top_p,omitempty"`
		MaxTokens           int                      `json:"max_tokens,omitempty"`
		MaxCompletionTokens int                      `json:"max_completion_tokens,omitempty"`
		Stream              bool                     `json:"stream,omitempty"`
		Tools               []map[string]any         `json:"tools,omitempty"`
		ToolChoice          json.RawMessage          `json:"tool_choice,omitempty"`
		ResponseFormat      *modality.ResponseFormat `json:"response_format,omitempty"`
		Stop                []string                 `json:"stop,omitempty"`
		Metadata            map[string]string        `json:"metadata,omitempty"`
		ReasoningEffort     string                   `json:"reasoning_effort,omitempty"`
	}{
		Model:               providerModel,
		Messages:            append([]modality.ChatMessage(nil), req.Messages...),
		Temperature:         req.Temperature,
		TopP:                req.TopP,
		MaxTokens:           maxTokens,
		MaxCompletionTokens: maxCompletionTokens,
		Stream:              stream,
		Tools:               chattools.OpenAITools(req.Tools),
		ToolChoice:          req.ToolChoice,
		ResponseFormat:      req.ResponseFormat,
		Stop:                append([]string(nil), req.Stop...),
		Metadata:            req.Metadata,
		ReasoningEffort:     modality.ReasoningEffort(req.Reasoning),
	}
}

func openAIUsesMaxCompletionTokens(providerModel string) bool {
	model := strings.ToLower(strings.TrimSpace(providerModel))
	if idx := strings.IndexByte(model, '/'); idx >= 0 {
		model = model[idx+1:]
	}
	return strings.HasPrefix(model, "gpt-5") || strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3") || strings.HasPrefix(model, "o4")
}
