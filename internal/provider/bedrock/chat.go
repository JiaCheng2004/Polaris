package bedrock

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	awsstream "github.com/JiaCheng2004/Polaris/internal/provider/common/aws"
)

type ChatAdapter struct {
	client *Client
	model  string
}

type converseRequest struct {
	Messages        []bedrockMessage          `json:"messages,omitempty"`
	System          []bedrockSystemContent    `json:"system,omitempty"`
	InferenceConfig *bedrockInferenceConfig   `json:"inferenceConfig,omitempty"`
	ToolConfig      *bedrockToolConfiguration `json:"toolConfig,omitempty"`
}

type bedrockMessage struct {
	Role    string                `json:"role"`
	Content []bedrockContentBlock `json:"content"`
}

type bedrockSystemContent struct {
	Text string `json:"text"`
}

type bedrockContentBlock struct {
	Text       string                  `json:"text,omitempty"`
	Image      *bedrockImageBlock      `json:"image,omitempty"`
	ToolUse    *bedrockToolUseBlock    `json:"toolUse,omitempty"`
	ToolResult *bedrockToolResultBlock `json:"toolResult,omitempty"`
}

type bedrockImageBlock struct {
	Format string             `json:"format"`
	Source bedrockImageSource `json:"source"`
}

type bedrockImageSource struct {
	Bytes      string                  `json:"bytes,omitempty"`
	S3Location *bedrockImageS3Location `json:"s3Location,omitempty"`
}

type bedrockImageS3Location struct {
	URI string `json:"uri"`
}

type bedrockToolUseBlock struct {
	ToolUseID string         `json:"toolUseId"`
	Name      string         `json:"name"`
	Input     map[string]any `json:"input,omitempty"`
}

type bedrockToolResultBlock struct {
	ToolUseID string                     `json:"toolUseId"`
	Status    string                     `json:"status,omitempty"`
	Content   []bedrockToolResultContent `json:"content"`
}

type bedrockToolResultContent struct {
	Text string         `json:"text,omitempty"`
	JSON map[string]any `json:"json,omitempty"`
}

type bedrockInferenceConfig struct {
	MaxTokens     int      `json:"maxTokens,omitempty"`
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"topP,omitempty"`
	StopSequences []string `json:"stopSequences,omitempty"`
}

type bedrockToolConfiguration struct {
	Tools      []bedrockTool `json:"tools"`
	ToolChoice any           `json:"toolChoice,omitempty"`
}

type bedrockTool struct {
	ToolSpec bedrockToolSpec `json:"toolSpec"`
}

type bedrockToolSpec struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema bedrockToolInputSchema `json:"inputSchema"`
}

type bedrockToolInputSchema struct {
	JSON json.RawMessage `json:"json"`
}

type converseResponse struct {
	Output struct {
		Message bedrockMessage `json:"message"`
	} `json:"output"`
	StopReason string `json:"stopReason"`
	Usage      struct {
		InputTokens  int `json:"inputTokens"`
		OutputTokens int `json:"outputTokens"`
		TotalTokens  int `json:"totalTokens"`
	} `json:"usage"`
}

type converseStreamEvent struct {
	MessageStart *struct {
		Role string `json:"role"`
	} `json:"messageStart,omitempty"`
	ContentBlockStart *struct {
		ContentBlockIndex int `json:"contentBlockIndex"`
		Start             struct {
			ToolUse *struct {
				Name      string `json:"name"`
				ToolUseID string `json:"toolUseId"`
			} `json:"toolUse,omitempty"`
		} `json:"start"`
	} `json:"contentBlockStart,omitempty"`
	ContentBlockDelta *struct {
		ContentBlockIndex int `json:"contentBlockIndex"`
		Delta             struct {
			Text    string `json:"text,omitempty"`
			ToolUse *struct {
				Input string `json:"input"`
			} `json:"toolUse,omitempty"`
		} `json:"delta"`
	} `json:"contentBlockDelta,omitempty"`
	ContentBlockStop *struct {
		ContentBlockIndex int `json:"contentBlockIndex"`
	} `json:"contentBlockStop,omitempty"`
	MessageStop *struct {
		StopReason string `json:"stopReason"`
	} `json:"messageStop,omitempty"`
	Metadata *struct {
		Usage struct {
			InputTokens  int `json:"inputTokens"`
			OutputTokens int `json:"outputTokens"`
			TotalTokens  int `json:"totalTokens"`
		} `json:"usage"`
	} `json:"metadata,omitempty"`
	ValidationException         *bedrockStreamException `json:"validationException,omitempty"`
	ThrottlingException         *bedrockStreamException `json:"throttlingException,omitempty"`
	ModelStreamErrorException   *bedrockStreamException `json:"modelStreamErrorException,omitempty"`
	InternalServerException     *bedrockStreamException `json:"internalServerException,omitempty"`
	ServiceUnavailableException *bedrockStreamException `json:"serviceUnavailableException,omitempty"`
}

type bedrockStreamException struct {
	Message string `json:"message"`
}

type bedrockStreamToolState struct {
	name  string
	id    string
	input strings.Builder
}

func NewChatAdapter(client *Client, model string) *ChatAdapter {
	return &ChatAdapter{client: client, model: model}
}

func (a *ChatAdapter) Complete(ctx context.Context, req *modality.ChatRequest) (*modality.ChatResponse, error) {
	payload, providerModel, err := a.translateRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	var response converseResponse
	if err := a.client.JSON(ctx, conversePath(providerModel, false), payload, &response); err != nil {
		return nil, err
	}
	return a.translateResponse(response, req.Model)
}

func (a *ChatAdapter) Stream(ctx context.Context, req *modality.ChatRequest) (<-chan modality.ChatChunk, error) {
	payload, providerModel, err := a.translateRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.Stream(ctx, conversePath(providerModel, true), payload)
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

func conversePath(model string, stream bool) string {
	suffix := "converse"
	if stream {
		suffix = "converse-stream"
	}
	return "/model/" + url.PathEscape(model) + "/" + suffix
}

func (a *ChatAdapter) translateRequest(ctx context.Context, req *modality.ChatRequest) (converseRequest, string, error) {
	providerModel := providerModelName(req.Model, a.model)
	payload := converseRequest{}

	if req.MaxTokens > 0 || req.Temperature != nil || req.TopP != nil || len(req.Stop) > 0 {
		payload.InferenceConfig = &bedrockInferenceConfig{
			MaxTokens:     req.MaxTokens,
			Temperature:   req.Temperature,
			TopP:          req.TopP,
			StopSequences: append([]string(nil), req.Stop...),
		}
	}

	for _, message := range req.Messages {
		switch message.Role {
		case "system":
			text, err := contentToText(message.Content)
			if err != nil {
				return converseRequest{}, "", err
			}
			if strings.TrimSpace(text) != "" {
				payload.System = append(payload.System, bedrockSystemContent{Text: text})
			}
		case "user":
			content, err := a.translateContentBlocks(ctx, message.Content)
			if err != nil {
				return converseRequest{}, "", err
			}
			if len(content) > 0 {
				payload.Messages = append(payload.Messages, bedrockMessage{Role: "user", Content: content})
			}
		case "assistant":
			content, err := a.translateContentBlocks(ctx, message.Content)
			if err != nil {
				return converseRequest{}, "", err
			}
			for _, toolCall := range message.ToolCalls {
				input := map[string]any{}
				if strings.TrimSpace(toolCall.Function.Arguments) != "" {
					if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &input); err != nil {
						input["_raw_arguments"] = toolCall.Function.Arguments
					}
				}
				content = append(content, bedrockContentBlock{
					ToolUse: &bedrockToolUseBlock{
						ToolUseID: toolCall.ID,
						Name:      toolCall.Function.Name,
						Input:     input,
					},
				})
			}
			if len(content) > 0 {
				payload.Messages = append(payload.Messages, bedrockMessage{Role: "assistant", Content: content})
			}
		case "tool":
			text, err := contentToText(message.Content)
			if err != nil {
				return converseRequest{}, "", err
			}
			result := bedrockToolResultContent{Text: text}
			if parsed := parseJSONMap(text); parsed != nil {
				result = bedrockToolResultContent{JSON: parsed}
			}
			payload.Messages = append(payload.Messages, bedrockMessage{
				Role: "user",
				Content: []bedrockContentBlock{{
					ToolResult: &bedrockToolResultBlock{
						ToolUseID: message.ToolCallID,
						Status:    "success",
						Content:   []bedrockToolResultContent{result},
					},
				}},
			})
		default:
			return converseRequest{}, "", httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_message_role", "messages.role", "Unsupported chat message role for Amazon Bedrock.")
		}
	}

	if len(req.Tools) > 0 {
		toolConfig, err := translateTools(req.Tools, req.ToolChoice)
		if err != nil {
			return converseRequest{}, "", err
		}
		payload.ToolConfig = toolConfig
	}

	return payload, providerModel, nil
}

func translateTools(tools []modality.ToolDefinition, rawChoice json.RawMessage) (*bedrockToolConfiguration, error) {
	config := &bedrockToolConfiguration{
		Tools: make([]bedrockTool, 0, len(tools)),
	}
	for _, tool := range tools {
		config.Tools = append(config.Tools, bedrockTool{
			ToolSpec: bedrockToolSpec{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				InputSchema: bedrockToolInputSchema{JSON: tool.Function.Parameters},
			},
		})
	}

	if len(rawChoice) == 0 {
		return config, nil
	}

	var stringChoice string
	if err := json.Unmarshal(rawChoice, &stringChoice); err == nil {
		switch stringChoice {
		case "auto":
			config.ToolChoice = map[string]any{"auto": map[string]any{}}
		case "required":
			config.ToolChoice = map[string]any{"any": map[string]any{}}
		case "none":
			return nil, nil
		default:
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_tool_choice", "tool_choice", "Unsupported tool_choice value.")
		}
		return config, nil
	}

	var objectChoice struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(rawChoice, &objectChoice); err != nil {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_tool_choice", "tool_choice", "tool_choice must be a string or function selector object.")
	}
	if objectChoice.Type != "function" || strings.TrimSpace(objectChoice.Function.Name) == "" {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_tool_choice", "tool_choice", "Function tool_choice must include function.name.")
	}
	config.ToolChoice = map[string]any{
		"tool": map[string]any{
			"name": objectChoice.Function.Name,
		},
	}
	return config, nil
}

func (a *ChatAdapter) translateContentBlocks(ctx context.Context, content modality.MessageContent) ([]bedrockContentBlock, error) {
	if content.Text != nil {
		if *content.Text == "" {
			return []bedrockContentBlock{}, nil
		}
		return []bedrockContentBlock{{Text: *content.Text}}, nil
	}

	blocks := make([]bedrockContentBlock, 0, len(content.Parts))
	for _, part := range content.Parts {
		switch part.Type {
		case "text":
			blocks = append(blocks, bedrockContentBlock{Text: part.Text})
		case "image_url":
			if part.ImageURL == nil || strings.TrimSpace(part.ImageURL.URL) == "" {
				return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image", "messages.content.image_url", "Image content must include image_url.url.")
			}
			image, err := a.translateImagePart(ctx, part.ImageURL.URL)
			if err != nil {
				return nil, err
			}
			blocks = append(blocks, bedrockContentBlock{Image: image})
		default:
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_content_part", "messages.content.type", "Unsupported content part type.")
		}
	}
	return blocks, nil
}

func (a *ChatAdapter) translateImagePart(ctx context.Context, raw string) (*bedrockImageBlock, error) {
	if strings.HasPrefix(raw, "data:") {
		mimeType, data, err := decodeDataURI(raw)
		if err != nil {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image_data_uri", "messages.content.image_url.url", "Invalid image data URI.")
		}
		format, err := bedrockImageFormat(mimeType)
		if err != nil {
			return nil, err
		}
		return &bedrockImageBlock{
			Format: format,
			Source: bedrockImageSource{Bytes: data},
		}, nil
	}
	if strings.HasPrefix(raw, "s3://") {
		format, err := bedrockImageFormat(guessMimeType(raw))
		if err != nil {
			return nil, err
		}
		return &bedrockImageBlock{
			Format: format,
			Source: bedrockImageSource{
				S3Location: &bedrockImageS3Location{URI: raw},
			},
		}, nil
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
		if err != nil {
			return nil, fmt.Errorf("build amazon bedrock image request: %w", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_transport_error", "", "Failed to fetch the input image.")
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		if resp.StatusCode >= http.StatusBadRequest {
			return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_transport_error", "", "Failed to fetch the input image.")
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
		if err != nil {
			return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_transport_error", "", "Failed to read the input image.")
		}
		mimeType := strings.TrimSpace(resp.Header.Get("Content-Type"))
		if mimeType == "" {
			mimeType = guessMimeType(raw)
		}
		format, err := bedrockImageFormat(mimeType)
		if err != nil {
			return nil, err
		}
		return &bedrockImageBlock{
			Format: format,
			Source: bedrockImageSource{Bytes: base64.StdEncoding.EncodeToString(data)},
		}, nil
	}
	return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image_reference", "messages.content.image_url.url", "Amazon Bedrock image inputs must be data URIs, S3 URIs, or reachable HTTP URLs.")
}

func (a *ChatAdapter) translateResponse(response converseResponse, canonicalModel string) (*modality.ChatResponse, error) {
	textParts := make([]string, 0)
	toolCalls := make([]modality.ToolCall, 0)
	for _, block := range response.Output.Message.Content {
		if strings.TrimSpace(block.Text) != "" {
			textParts = append(textParts, block.Text)
		}
		if block.ToolUse != nil {
			arguments := "{}"
			if len(block.ToolUse.Input) > 0 {
				data, _ := json.Marshal(block.ToolUse.Input)
				arguments = string(data)
			}
			toolCalls = append(toolCalls, modality.ToolCall{
				ID:   block.ToolUse.ToolUseID,
				Type: "function",
				Function: modality.ToolCallFunction{
					Name:      block.ToolUse.Name,
					Arguments: arguments,
				},
			})
		}
	}

	message := modality.ChatMessage{Role: "assistant", ToolCalls: toolCalls}
	if len(textParts) > 0 {
		message.Content = modality.NewTextContent(strings.Join(textParts, ""))
	}

	return &modality.ChatResponse{
		ID:      bedrockResponseID(canonicalModel),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   firstNonEmpty(canonicalModel, a.model),
		Choices: []modality.ChatChoice{{
			Index:        0,
			Message:      message,
			FinishReason: normalizeStopReason(response.StopReason),
		}},
		Usage: modality.Usage{
			PromptTokens:     response.Usage.InputTokens,
			CompletionTokens: response.Usage.OutputTokens,
			TotalTokens:      response.Usage.TotalTokens,
			Source:           modality.TokenCountSourceProviderReported,
		},
	}, nil
}

func (a *ChatAdapter) decodeStream(r io.Reader, canonicalModel string, dst chan<- modality.ChatChunk) error {
	toolStates := map[int]*bedrockStreamToolState{}

	return awsstream.DecodeEventStream(r, func(payload []byte) error {
		var event converseStreamEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return fmt.Errorf("decode amazon bedrock stream event: %w", err)
		}

		if event.ValidationException != nil {
			return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_stream_error", "", firstNonEmpty(event.ValidationException.Message, "Amazon Bedrock stream validation failed."))
		}
		if event.ThrottlingException != nil {
			return httputil.NewError(http.StatusTooManyRequests, "provider_error", "provider_rate_limited", "", firstNonEmpty(event.ThrottlingException.Message, "Amazon Bedrock rate limited the request."))
		}
		if event.ModelStreamErrorException != nil {
			return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_stream_error", "", firstNonEmpty(event.ModelStreamErrorException.Message, "Amazon Bedrock stream failed."))
		}
		if event.InternalServerException != nil {
			return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_stream_error", "", firstNonEmpty(event.InternalServerException.Message, "Amazon Bedrock stream failed."))
		}
		if event.ServiceUnavailableException != nil {
			return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_stream_error", "", firstNonEmpty(event.ServiceUnavailableException.Message, "Amazon Bedrock stream failed."))
		}

		if event.MessageStart != nil {
			dst <- normalizedChunk(canonicalModel, a.model, modality.ChatChunkChoice{
				Index: 0,
				Delta: modality.ChatDelta{Role: event.MessageStart.Role},
			})
		}
		if event.ContentBlockStart != nil && event.ContentBlockStart.Start.ToolUse != nil {
			toolStates[event.ContentBlockStart.ContentBlockIndex] = &bedrockStreamToolState{
				name: event.ContentBlockStart.Start.ToolUse.Name,
				id:   event.ContentBlockStart.Start.ToolUse.ToolUseID,
			}
		}
		if event.ContentBlockDelta != nil {
			if strings.TrimSpace(event.ContentBlockDelta.Delta.Text) != "" {
				dst <- normalizedChunk(canonicalModel, a.model, modality.ChatChunkChoice{
					Index: 0,
					Delta: modality.ChatDelta{Content: event.ContentBlockDelta.Delta.Text},
				})
			}
			if event.ContentBlockDelta.Delta.ToolUse != nil {
				state := toolStates[event.ContentBlockDelta.ContentBlockIndex]
				if state == nil {
					state = &bedrockStreamToolState{}
					toolStates[event.ContentBlockDelta.ContentBlockIndex] = state
				}
				state.input.WriteString(event.ContentBlockDelta.Delta.ToolUse.Input)
			}
		}
		if event.ContentBlockStop != nil {
			state := toolStates[event.ContentBlockStop.ContentBlockIndex]
			if state != nil && strings.TrimSpace(state.id) != "" && strings.TrimSpace(state.name) != "" {
				dst <- normalizedChunk(canonicalModel, a.model, modality.ChatChunkChoice{
					Index: 0,
					Delta: modality.ChatDelta{
						ToolCalls: []modality.ToolCall{{
							ID:   state.id,
							Type: "function",
							Function: modality.ToolCallFunction{
								Name:      state.name,
								Arguments: firstNonEmpty(strings.TrimSpace(state.input.String()), "{}"),
							},
						}},
					},
				})
			}
			delete(toolStates, event.ContentBlockStop.ContentBlockIndex)
		}
		if event.MessageStop != nil {
			finishReason := normalizeStopReason(event.MessageStop.StopReason)
			dst <- normalizedChunk(canonicalModel, a.model, modality.ChatChunkChoice{
				Index:        0,
				FinishReason: &finishReason,
			})
		}
		if event.Metadata != nil {
			chunk := normalizedChunk(canonicalModel, a.model, modality.ChatChunkChoice{Index: 0})
			chunk.Usage = &modality.Usage{
				PromptTokens:     event.Metadata.Usage.InputTokens,
				CompletionTokens: event.Metadata.Usage.OutputTokens,
				TotalTokens:      event.Metadata.Usage.TotalTokens,
				Source:           modality.TokenCountSourceProviderReported,
			}
			dst <- chunk
		}
		return nil
	})
}

func normalizedChunk(canonicalModel string, fallbackModel string, choice modality.ChatChunkChoice) modality.ChatChunk {
	return modality.ChatChunk{
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   firstNonEmpty(canonicalModel, fallbackModel),
		Choices: []modality.ChatChunkChoice{choice},
	}
}

func normalizeStopReason(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "", "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	case "guardrail_intervened", "content_filtered":
		return "content_filter"
	default:
		return strings.TrimSpace(strings.ToLower(value))
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func bedrockResponseID(canonicalModel string) string {
	model := firstNonEmpty(canonicalModel, "bedrock")
	model = strings.ReplaceAll(model, "/", "-")
	return "chatcmpl-bedrock-" + model
}

func contentToText(content modality.MessageContent) (string, error) {
	if content.Text != nil {
		return *content.Text, nil
	}
	var parts []string
	for _, part := range content.Parts {
		if part.Type != "text" {
			return "", httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_system_content", "messages.content", "System and tool messages must use text content.")
		}
		parts = append(parts, part.Text)
	}
	return strings.Join(parts, "\n"), nil
}

func parseJSONMap(value string) map[string]any {
	var out map[string]any
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil
	}
	return out
}

func decodeDataURI(value string) (string, string, error) {
	parts := strings.SplitN(value, ",", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid data uri")
	}
	meta := strings.TrimPrefix(parts[0], "data:")
	mimeType := "application/octet-stream"
	if idx := strings.Index(meta, ";"); idx >= 0 {
		mimeType = meta[:idx]
	} else if meta != "" {
		mimeType = meta
	}
	return mimeType, parts[1], nil
}

func bedrockImageFormat(mediaType string) (string, error) {
	if parsed, _, err := mime.ParseMediaType(mediaType); err == nil {
		mediaType = parsed
	}
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image/png":
		return "png", nil
	case "image/jpeg", "image/jpg":
		return "jpeg", nil
	default:
		return "", httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image_format", "messages.content.image_url.url", "Amazon Bedrock only supports PNG and JPEG chat image inputs.")
	}
}

func guessMimeType(raw string) string {
	switch strings.ToLower(path.Ext(raw)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	default:
		return "image/png"
	}
}
