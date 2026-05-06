package openai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/chattools"
	tiktoken "github.com/pkoukk/tiktoken-go"
)

type ChatAdapter struct {
	client *Client
	model  string
}

func NewChatAdapter(client *Client, model string) *ChatAdapter {
	return &ChatAdapter{
		client: client,
		model:  model,
	}
}

func (a *ChatAdapter) Complete(ctx context.Context, req *modality.ChatRequest) (*modality.ChatResponse, error) {
	payload := translateChatRequest(req, false, providerModelName(req.Model, a.model))

	var response modality.ChatResponse
	if err := a.client.JSON(ctx, "/chat/completions", payload, &response); err != nil {
		return nil, err
	}

	normalizeChatResponse(&response, req.Model, a.model)
	return &response, nil
}

func (a *ChatAdapter) Stream(ctx context.Context, req *modality.ChatRequest) (<-chan modality.ChatChunk, error) {
	payload := translateChatRequest(req, true, providerModelName(req.Model, a.model))

	resp, err := a.client.Stream(ctx, "/chat/completions", payload)
	if err != nil {
		return nil, err
	}

	stream := make(chan modality.ChatChunk)
	go func() {
		defer close(stream)
		defer func() {
			_ = resp.Body.Close()
		}()

		if err := decodeOpenAIStream(resp.Body, req.Model, a.model, stream); err != nil {
			stream <- modality.ChatChunk{Err: err}
		}
	}()

	return stream, nil
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

func decodeOpenAIStream(r io.Reader, canonicalModel string, fallbackModel string, dst chan<- modality.ChatChunk) error {
	reader := bufio.NewReader(r)
	var dataLines []string

	flush := func() (bool, error) {
		if len(dataLines) == 0 {
			return false, nil
		}
		payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = nil
		if payload == "" {
			return false, nil
		}
		if payload == "[DONE]" {
			return true, nil
		}

		var envelope struct {
			Error *struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
				Param   string `json:"param"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &envelope); err == nil && envelope.Error != nil {
			message := envelope.Error.Message
			if strings.TrimSpace(message) == "" {
				message = "OpenAI streaming request failed."
			}
			return false, httputil.NewError(502, "provider_error", "provider_stream_error", "", message)
		}

		var chunk modality.ChatChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return false, fmt.Errorf("decode openai stream chunk: %w", err)
		}
		normalizeChatChunk(&chunk, canonicalModel, fallbackModel)
		dst <- chunk
		return false, nil
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			if err == io.EOF {
				_, flushErr := flush()
				return flushErr
			}
			return fmt.Errorf("read openai stream: %w", err)
		}

		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			done, flushErr := flush()
			if flushErr != nil {
				return flushErr
			}
			if done {
				return nil
			}
		} else if strings.HasPrefix(trimmed, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
		}

		if err == io.EOF {
			done, flushErr := flush()
			if flushErr != nil {
				return flushErr
			}
			if done {
				return nil
			}
			return nil
		}
	}
}

func normalizeChatResponse(response *modality.ChatResponse, canonicalModel string, fallbackModel string) {
	if response.Object == "" {
		response.Object = "chat.completion"
	}
	if response.Created == 0 {
		response.Created = time.Now().Unix()
	}
	if response.Model == "" || !strings.Contains(response.Model, "/") {
		response.Model = firstNonEmpty(canonicalModel, fallbackModel)
	}
}

func normalizeChatChunk(chunk *modality.ChatChunk, canonicalModel string, fallbackModel string) {
	if chunk.Object == "" {
		chunk.Object = "chat.completion.chunk"
	}
	if chunk.Created == 0 {
		chunk.Created = time.Now().Unix()
	}
	if chunk.Model == "" || !strings.Contains(chunk.Model, "/") {
		chunk.Model = firstNonEmpty(canonicalModel, fallbackModel)
	}
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
