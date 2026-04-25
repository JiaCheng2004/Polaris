package ollama

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
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
	payload, err := a.translateRequest(req, false)
	if err != nil {
		return nil, err
	}

	var response ollamaChatResponse
	if err := a.client.JSON(ctx, "/chat", payload, &response); err != nil {
		return nil, err
	}

	translated, err := a.translateResponse(response, req.Model)
	if err != nil {
		return nil, err
	}
	return translated, nil
}

func (a *ChatAdapter) Stream(ctx context.Context, req *modality.ChatRequest) (<-chan modality.ChatChunk, error) {
	payload, err := a.translateRequest(req, true)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.Stream(ctx, "/chat", payload)
	if err != nil {
		return nil, err
	}

	stream := make(chan modality.ChatChunk)
	go func() {
		defer close(stream)
		defer func() {
			_ = resp.Body.Close()
		}()

		if err := a.decodeStream(resp.Body, req.Model, stream); err != nil {
			stream <- modality.ChatChunk{Err: err}
		}
	}()

	return stream, nil
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Tools    []ollamaToolDef `json:"tools,omitempty"`
	Format   any             `json:"format,omitempty"`
	Options  map[string]any  `json:"options,omitempty"`
	Stream   bool            `json:"stream"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content,omitempty"`
	Images    []string         `json:"images,omitempty"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
	ToolName  string           `json:"tool_name,omitempty"`
}

type ollamaToolDef struct {
	Type     string                `json:"type"`
	Function ollamaToolDefFunction `json:"function"`
}

type ollamaToolDefFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaFunctionCall `json:"function"`
}

type ollamaFunctionCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type ollamaChatResponse struct {
	Model           string        `json:"model"`
	CreatedAt       string        `json:"created_at"`
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	DoneReason      string        `json:"done_reason"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	EvalCount       int           `json:"eval_count"`
}

func (a *ChatAdapter) translateRequest(req *modality.ChatRequest, stream bool) (ollamaChatRequest, error) {
	payload := ollamaChatRequest{
		Model:  providerModelName(req.Model, a.model),
		Stream: stream,
	}

	for _, message := range req.Messages {
		translated, err := translateMessage(message)
		if err != nil {
			return ollamaChatRequest{}, err
		}
		payload.Messages = append(payload.Messages, translated)
	}

	for _, tool := range req.Tools {
		payload.Tools = append(payload.Tools, ollamaToolDef{
			Type: tool.Type,
			Function: ollamaToolDefFunction{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		})
	}

	if req.ResponseFormat != nil {
		format, err := translateFormat(req.ResponseFormat)
		if err != nil {
			return ollamaChatRequest{}, err
		}
		payload.Format = format
	}

	options := map[string]any{}
	if req.Temperature != nil {
		options["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		options["top_p"] = *req.TopP
	}
	if req.MaxTokens > 0 {
		options["num_predict"] = req.MaxTokens
	}
	if len(req.Stop) > 0 {
		options["stop"] = append([]string(nil), req.Stop...)
	}
	if len(options) > 0 {
		payload.Options = options
	}

	return payload, nil
}

func translateMessage(message modality.ChatMessage) (ollamaMessage, error) {
	text, images, err := translateContent(message.Content)
	if err != nil {
		return ollamaMessage{}, err
	}

	translated := ollamaMessage{
		Role:    message.Role,
		Content: text,
		Images:  images,
	}
	if message.Role == "tool" && strings.TrimSpace(message.Name) != "" {
		translated.ToolName = message.Name
	}
	if len(message.ToolCalls) > 0 {
		translated.ToolCalls = translateToolCalls(message.ToolCalls)
	}
	return translated, nil
}

func translateContent(content modality.MessageContent) (string, []string, error) {
	if content.Text != nil {
		return *content.Text, nil, nil
	}

	var textParts []string
	var images []string
	for _, part := range content.Parts {
		switch part.Type {
		case "text":
			textParts = append(textParts, part.Text)
		case "image_url":
			if part.ImageURL == nil || strings.TrimSpace(part.ImageURL.URL) == "" {
				return "", nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image", "messages.content.image_url", "Image content must include image_url.url.")
			}
			encoded, err := imageValue(part.ImageURL.URL)
			if err != nil {
				return "", nil, err
			}
			images = append(images, encoded)
		case "input_audio":
			return "", nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_audio_input", "messages.content.input_audio", "Ollama chat does not support audio input in this build.")
		default:
			return "", nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_content_part", "messages.content.type", "Unsupported content part type.")
		}
	}

	return strings.Join(textParts, "\n"), images, nil
}

func imageValue(raw string) (string, error) {
	if strings.HasPrefix(raw, "data:") {
		_, data, ok := strings.Cut(strings.TrimPrefix(raw, "data:"), ",")
		if !ok {
			return "", httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image_data_uri", "messages.content.image_url.url", "Invalid image data URI.")
		}
		header := raw[:strings.Index(raw, ",")]
		if !strings.Contains(header, ";base64") {
			data = base64.StdEncoding.EncodeToString([]byte(data))
		}
		return data, nil
	}
	return raw, nil
}

func translateToolCalls(calls []modality.ToolCall) []ollamaToolCall {
	result := make([]ollamaToolCall, 0, len(calls))
	for _, call := range calls {
		args := map[string]any{}
		if strings.TrimSpace(call.Function.Arguments) != "" {
			if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
				args["_raw_arguments"] = call.Function.Arguments
			}
		}
		result = append(result, ollamaToolCall{
			Function: ollamaFunctionCall{
				Name:      call.Function.Name,
				Arguments: args,
			},
		})
	}
	return result
}

func translateFormat(format *modality.ResponseFormat) (any, error) {
	switch format.Type {
	case "json_object":
		return "json", nil
	case "json_schema":
		if format.JSONSchema == nil || len(format.JSONSchema.Schema) == 0 {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_json_schema", "response_format", "json_schema response formats must include json_schema.schema.")
		}
		var schema any
		if err := json.Unmarshal(format.JSONSchema.Schema, &schema); err != nil {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json_schema", "response_format", "json_schema.schema must be valid JSON.")
		}
		return schema, nil
	default:
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_response_format", "response_format", "Unsupported response_format type.")
	}
}

func (a *ChatAdapter) translateResponse(response ollamaChatResponse, canonicalModel string) (*modality.ChatResponse, error) {
	created := createdUnix(response.CreatedAt)
	message := modality.ChatMessage{
		Role:      firstNonEmpty(response.Message.Role, "assistant"),
		ToolCalls: translateResponseToolCalls(response.Message.ToolCalls),
	}
	if response.Message.Content != "" {
		message.Content = modality.NewTextContent(response.Message.Content)
	}

	usage := modality.Usage{
		PromptTokens:     response.PromptEvalCount,
		CompletionTokens: response.EvalCount,
		TotalTokens:      response.PromptEvalCount + response.EvalCount,
	}

	return &modality.ChatResponse{
		ID:      responseID(response.Model, created),
		Object:  "chat.completion",
		Created: created,
		Model:   firstNonEmpty(canonicalModel, response.Model),
		Choices: []modality.ChatChoice{
			{
				Index:        0,
				Message:      message,
				FinishReason: firstNonEmpty(response.DoneReason, "stop"),
			},
		},
		Usage: usage,
	}, nil
}

func translateResponseToolCalls(calls []ollamaToolCall) []modality.ToolCall {
	if len(calls) == 0 {
		return nil
	}

	result := make([]modality.ToolCall, 0, len(calls))
	for index, call := range calls {
		arguments := "{}"
		if len(call.Function.Arguments) > 0 {
			if raw, err := json.Marshal(call.Function.Arguments); err == nil {
				arguments = string(raw)
			}
		}
		result = append(result, modality.ToolCall{
			ID:   fmt.Sprintf("call_%d", index),
			Type: "function",
			Function: modality.ToolCallFunction{
				Name:      call.Function.Name,
				Arguments: arguments,
			},
		})
	}
	return result
}

func (a *ChatAdapter) decodeStream(body io.Reader, canonicalModel string, dst chan<- modality.ChatChunk) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

	firstChunk := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var response ollamaChatResponse
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			return fmt.Errorf("decode ollama stream chunk: %w", err)
		}

		created := createdUnix(response.CreatedAt)
		model := firstNonEmpty(canonicalModel, response.Model)

		if response.Done {
			finishReason := firstNonEmpty(response.DoneReason, "stop")
			usage := &modality.Usage{
				PromptTokens:     response.PromptEvalCount,
				CompletionTokens: response.EvalCount,
				TotalTokens:      response.PromptEvalCount + response.EvalCount,
			}
			dst <- modality.ChatChunk{
				ID:      responseID(response.Model, created),
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   model,
				Choices: []modality.ChatChunkChoice{
					{
						Index:        0,
						Delta:        modality.ChatDelta{},
						FinishReason: &finishReason,
					},
				},
				Usage: usage,
			}
			continue
		}

		delta := modality.ChatDelta{}
		if firstChunk {
			delta.Role = firstNonEmpty(response.Message.Role, "assistant")
			firstChunk = false
		}
		if response.Message.Content != "" {
			delta.Content = response.Message.Content
		}
		if len(response.Message.ToolCalls) > 0 {
			delta.ToolCalls = translateResponseToolCalls(response.Message.ToolCalls)
		}
		if delta.Role == "" && delta.Content == "" && len(delta.ToolCalls) == 0 {
			continue
		}

		dst <- modality.ChatChunk{
			ID:      responseID(response.Model, created),
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   model,
			Choices: []modality.ChatChunkChoice{
				{
					Index: 0,
					Delta: delta,
				},
			},
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read ollama stream: %w", err)
	}
	return nil
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

func createdUnix(raw string) int64 {
	if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return parsed.Unix()
	}
	return time.Now().Unix()
}

func responseID(model string, created int64) string {
	return fmt.Sprintf("ollama-%s-%d", strings.ReplaceAll(firstNonEmpty(model, "chat"), "/", "-"), created)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
