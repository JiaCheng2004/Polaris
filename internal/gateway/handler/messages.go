package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gin-gonic/gin"
)

type messagesRequest struct {
	Model         string                   `json:"model"`
	Routing       *modality.RoutingOptions `json:"routing,omitempty"`
	System        string                   `json:"system,omitempty"`
	Messages      []messagesInputMessage   `json:"messages"`
	MaxTokens     int                      `json:"max_tokens,omitempty"`
	Temperature   *float64                 `json:"temperature,omitempty"`
	TopP          *float64                 `json:"top_p,omitempty"`
	Stream        bool                     `json:"stream,omitempty"`
	StopSequences []string                 `json:"stop_sequences,omitempty"`
	Tools         []messagesToolDefinition `json:"tools,omitempty"`
	ToolChoice    json.RawMessage          `json:"tool_choice,omitempty"`
	Metadata      map[string]string        `json:"metadata,omitempty"`
}

type messagesToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type messagesInputMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type messagesInputBlock struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text,omitempty"`
	Source    *messagesContentSource `json:"source,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     json.RawMessage        `json:"input,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	Content   json.RawMessage        `json:"content,omitempty"`
}

type messagesContentSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type messagesAPIResponse struct {
	ID           string                `json:"id"`
	Type         string                `json:"type"`
	Role         string                `json:"role"`
	Content      []messagesOutputBlock `json:"content"`
	Model        string                `json:"model"`
	StopReason   string                `json:"stop_reason,omitempty"`
	StopSequence *string               `json:"stop_sequence,omitempty"`
	Usage        messagesUsage         `json:"usage"`
}

type messagesOutputBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type messagesUsage struct {
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	Source       string `json:"source,omitempty"`
}

func (h *ChatHandler) Messages(c *gin.Context) {
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		if httputil.IsRequestBodyTooLarge(err) {
			httputil.WriteError(c, httputil.RequestBodyTooLargeError(0))
			return
		}
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}

	var req messagesRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}

	chatReq, err := normalizeMessagesRequest(&req)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if err := validateChatRequest(chatReq); err != nil {
		httputil.WriteError(c, err)
		return
	}

	if cacheCtl := newResponseCache(c, h.runtime, h.cache); cacheCtl != nil {
		cacheCtl.markBypass(c)
	} else {
		c.Header(cacheHeader, "bypass")
	}

	if primary, fallbacks, prepErr := h.prepareConversation(c, chatReq); prepErr == nil {
		applyResolvedRoutingHeaders(c, primary.resolution)
		if adapter, ok := primary.adapter.(modality.NativeMessagesAdapter); ok {
			if req.Stream {
				if err := h.streamNativeMessages(c, adapter, primary, rawBody); err == nil {
					return
				} else if shouldRetryWithFallback(apiErrorFrom(err)) && len(fallbacks) > 0 {
					stream, selected, outcome, fallbackModel, fallbackErr := h.openFallbackConversationStream(c, primary, fallbacks, chatReq, "messages")
					if fallbackErr != nil {
						middleware.SetRequestOutcome(c, outcome)
						writeConversationTargetError(c, "messages", fallbackErr)
						return
					}
					writeConversationFallbackHeaders(c, h, primary.model.ID, outcome, fallbackModel)
					h.streamMessages(c, selected, stream, outcome)
					return
				} else {
					writeNativeConversationError(c, primary, "messages", err)
				}
				return
			}
			if err := h.nativeMessages(c, adapter, primary, rawBody); err == nil {
				return
			} else if shouldRetryWithFallback(apiErrorFrom(err)) && len(fallbacks) > 0 {
				response, outcome, fallbackModel, fallbackErr := h.completeFallbackConversation(c, primary, fallbacks, chatReq, "messages")
				if fallbackErr != nil {
					middleware.SetRequestOutcome(c, outcome)
					writeConversationTargetError(c, "messages", fallbackErr)
					return
				}
				writeConversationFallbackHeaders(c, h, primary.model.ID, outcome, fallbackModel)
				middleware.SetRequestOutcome(c, outcome)
				c.JSON(http.StatusOK, renderMessagesResponse(response))
				return
			} else {
				writeNativeConversationError(c, primary, "messages", err)
			}
			return
		}
	}
	if req.Stream {
		stream, selected, outcome, fallbackModel, err := h.openConversationStream(c, chatReq, "messages")
		if err != nil {
			middleware.SetRequestOutcome(c, outcome)
			writeConversationTargetError(c, "messages", err)
			return
		}
		if fallbackModel != "" {
			c.Header("X-Polaris-Fallback", fallbackModel)
			h.metrics.IncFailover(chatReq.Model, fallbackModel)
			c.Header("X-Polaris-Resolved-Model", outcome.Model)
			c.Header("X-Polaris-Resolved-Provider", outcome.Provider)
		}
		h.streamMessages(c, selected, stream, outcome)
		return
	}

	response, outcome, fallbackModel, err := h.executeConversation(c, chatReq, "messages")
	if err != nil {
		middleware.SetRequestOutcome(c, outcome)
		writeConversationTargetError(c, "messages", err)
		return
	}
	if fallbackModel != "" {
		c.Header("X-Polaris-Fallback", fallbackModel)
		h.metrics.IncFailover(chatReq.Model, fallbackModel)
		c.Header("X-Polaris-Resolved-Model", outcome.Model)
		c.Header("X-Polaris-Resolved-Provider", outcome.Provider)
	}
	middleware.SetRequestOutcome(c, outcome)
	c.JSON(http.StatusOK, renderMessagesResponse(response))
}

func (h *ChatHandler) nativeMessages(c *gin.Context, adapter modality.NativeMessagesAdapter, target chatTarget, rawBody json.RawMessage) error {
	response, err := adapter.CreateMessage(c.Request.Context(), rawBody, target.model.ID)
	outcome := middleware.RequestOutcome{
		Model:           target.model.ID,
		Provider:        target.model.Provider,
		Modality:        modality.ModalityChat,
		InterfaceFamily: "messages",
		StatusCode:      http.StatusOK,
	}
	if err != nil {
		return err
	}
	if response.Usage != nil {
		outcome.PromptTokens = response.Usage.PromptTokens
		outcome.CompletionTokens = response.Usage.CompletionTokens
		outcome.TotalTokens = response.Usage.TotalTokens
		outcome.TokenSource = providerUsageSource(*response.Usage)
	}
	middleware.SetRequestOutcome(c, outcome)
	c.Data(http.StatusOK, "application/json; charset=utf-8", response.Payload)
	return nil
}

func (h *ChatHandler) streamNativeMessages(c *gin.Context, adapter modality.NativeMessagesAdapter, target chatTarget, rawBody json.RawMessage) error {
	stream, err := adapter.StreamMessage(c.Request.Context(), rawBody, target.model.ID)
	outcome := middleware.RequestOutcome{
		Model:           target.model.ID,
		Provider:        target.model.Provider,
		Modality:        modality.ModalityChat,
		InterfaceFamily: "messages",
		StatusCode:      http.StatusOK,
	}
	if err != nil {
		return err
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)
	c.Writer.Flush()

	for event := range stream {
		if event.Err != nil {
			apiErr := apiErrorFrom(event.Err)
			outcome.ErrorType = apiErr.Type
			middleware.SetRequestOutcome(c, outcome)
			if err := writeSSEErrorEvent(c, "error", apiErr); err != nil {
				return nil
			}
			return nil
		}
		if event.Usage != nil {
			outcome.PromptTokens = event.Usage.PromptTokens
			outcome.CompletionTokens = event.Usage.CompletionTokens
			outcome.TotalTokens = event.Usage.TotalTokens
			outcome.TokenSource = providerUsageSource(*event.Usage)
		}
		if err := writeRawSSEEvent(c, event.Event, event.Payload); err != nil {
			middleware.SetRequestOutcome(c, outcome)
			return nil
		}
	}

	middleware.SetRequestOutcome(c, outcome)
	return nil
}

func normalizeMessagesRequest(req *messagesRequest) (*modality.ChatRequest, error) {
	if strings.TrimSpace(req.Model) == "" {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	messages, err := normalizeMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	if system := strings.TrimSpace(req.System); system != "" {
		messages = append([]modality.ChatMessage{{
			Role:    "system",
			Content: modality.NewTextContent(system),
		}}, messages...)
	}
	tools := make([]modality.ToolDefinition, 0, len(req.Tools))
	for _, tool := range req.Tools {
		tools = append(tools, modality.ToolDefinition{
			Type: "function",
			Function: modality.FunctionDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			},
		})
	}
	return &modality.ChatRequest{
		Model:       req.Model,
		Routing:     req.Routing,
		Messages:    messages,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
		Tools:       tools,
		ToolChoice:  append(json.RawMessage(nil), req.ToolChoice...),
		Stop:        append([]string(nil), req.StopSequences...),
		Metadata:    cloneStringMap(req.Metadata),
	}, nil
}

func normalizeMessages(messages []messagesInputMessage) ([]modality.ChatMessage, error) {
	var normalized []modality.ChatMessage
	for _, message := range messages {
		converted, err := normalizeMessage(message)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, converted...)
	}
	return normalized, nil
}

func normalizeMessage(message messagesInputMessage) ([]modality.ChatMessage, error) {
	role := strings.TrimSpace(message.Role)
	if role == "" {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_role", "messages.role", "Messages must include a role.")
	}

	content, blocks, err := decodeMessageContent(message.Content)
	if err != nil {
		return nil, err
	}
	switch role {
	case "user":
		toolOnly := true
		for _, block := range blocks {
			if block.Type != "tool_result" {
				toolOnly = false
				break
			}
		}
		if toolOnly && len(blocks) > 0 {
			converted := make([]modality.ChatMessage, 0, len(blocks))
			for _, block := range blocks {
				text, err := decodeToolResultContent(block.Content)
				if err != nil {
					return nil, err
				}
				converted = append(converted, modality.ChatMessage{
					Role:       "tool",
					ToolCallID: block.ToolUseID,
					Content:    modality.NewTextContent(text),
				})
			}
			return converted, nil
		}
		return []modality.ChatMessage{{Role: "user", Content: content}}, nil
	case "assistant":
		toolCalls := make([]modality.ToolCall, 0)
		for _, block := range blocks {
			if block.Type != "tool_use" {
				continue
			}
			arguments := strings.TrimSpace(string(block.Input))
			if arguments == "" {
				arguments = "{}"
			}
			toolCalls = append(toolCalls, modality.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: modality.ToolCallFunction{
					Name:      block.Name,
					Arguments: arguments,
				},
			})
		}
		assistantContent := content
		if assistantContent.Text == nil && len(assistantContent.Parts) > 0 {
			filtered := make([]modality.ContentPart, 0, len(assistantContent.Parts))
			for _, part := range assistantContent.Parts {
				if part.Type == "text" || part.Type == "image_url" || part.Type == "input_audio" {
					filtered = append(filtered, part)
				}
			}
			assistantContent = modality.NewPartContent(filtered...)
		}
		return []modality.ChatMessage{{Role: "assistant", Content: assistantContent, ToolCalls: toolCalls}}, nil
	default:
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_role", "messages.role", "Messages must use the roles user or assistant.")
	}
}

func decodeMessageContent(raw json.RawMessage) (modality.MessageContent, []messagesInputBlock, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return modality.MessageContent{}, nil, nil
	}
	if trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return modality.MessageContent{}, nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_content", "messages.content", "Message content must be a string or block array.")
		}
		return modality.NewTextContent(text), nil, nil
	}
	if trimmed[0] != '[' {
		return modality.MessageContent{}, nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_content", "messages.content", "Message content must be a string or block array.")
	}

	var blocks []messagesInputBlock
	if err := json.Unmarshal(trimmed, &blocks); err != nil {
		return modality.MessageContent{}, nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_content", "messages.content", "Message content must be a valid block array.")
	}
	parts := make([]modality.ContentPart, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case "text":
			parts = append(parts, modality.ContentPart{Type: "text", Text: block.Text})
		case "image":
			part, err := normalizeMessageImage(block.Source)
			if err != nil {
				return modality.MessageContent{}, nil, err
			}
			parts = append(parts, part)
		case "audio":
			part, err := normalizeMessageAudio(block.Source)
			if err != nil {
				return modality.MessageContent{}, nil, err
			}
			parts = append(parts, part)
		case "tool_use", "tool_result":
			// handled by higher-level message normalization
		default:
			return modality.MessageContent{}, nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_content_part", "messages.content.type", "Unsupported message content block.")
		}
	}
	return modality.NewPartContent(parts...), blocks, nil
}

func normalizeMessageImage(source *messagesContentSource) (modality.ContentPart, error) {
	if source == nil {
		return modality.ContentPart{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image", "messages.content.source", "Image content must include a source.")
	}
	switch strings.ToLower(strings.TrimSpace(source.Type)) {
	case "url":
		if strings.TrimSpace(source.URL) == "" {
			return modality.ContentPart{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image", "messages.content.source.url", "Image source URL is required.")
		}
		return modality.ContentPart{
			Type: "image_url",
			ImageURL: &modality.ImageURLPart{
				URL: source.URL,
			},
		}, nil
	case "base64":
		if strings.TrimSpace(source.Data) == "" || strings.TrimSpace(source.MediaType) == "" {
			return modality.ContentPart{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image", "messages.content.source", "Base64 image sources require media_type and data.")
		}
		return modality.ContentPart{
			Type: "image_url",
			ImageURL: &modality.ImageURLPart{
				URL: fmt.Sprintf("data:%s;base64,%s", source.MediaType, source.Data),
			},
		}, nil
	default:
		return modality.ContentPart{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image", "messages.content.source.type", "Image source type must be url or base64.")
	}
}

func normalizeMessageAudio(source *messagesContentSource) (modality.ContentPart, error) {
	if source == nil || strings.TrimSpace(source.Data) == "" || strings.TrimSpace(source.MediaType) == "" {
		return modality.ContentPart{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_audio", "messages.content.source", "Audio content must include media_type and data.")
	}
	format := audioFormatFromMediaType(source.MediaType)
	if format == "" {
		return modality.ContentPart{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_audio", "messages.content.source.media_type", "Audio media_type is not supported.")
	}
	return modality.ContentPart{
		Type: "input_audio",
		InputAudio: &modality.InputAudioPart{
			Data:   source.Data,
			Format: format,
		},
	}, nil
}

func audioFormatFromMediaType(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "audio/wav", "audio/x-wav":
		return "wav"
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	case "audio/flac":
		return "flac"
	case "audio/ogg", "audio/opus":
		return "opus"
	case "audio/aac":
		return "aac"
	default:
		return ""
	}
}

func decodeToolResultContent(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", nil
	}
	if trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return "", httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_tool_result", "messages.content", "tool_result content must be text or block array.")
		}
		return text, nil
	}
	if trimmed[0] != '[' {
		return "", httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_tool_result", "messages.content", "tool_result content must be text or block array.")
	}
	var blocks []messagesInputBlock
	if err := json.Unmarshal(trimmed, &blocks); err != nil {
		return "", httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_tool_result", "messages.content", "tool_result content must be valid blocks.")
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n"), nil
}

func renderMessagesResponse(response *modality.ChatResponse) messagesAPIResponse {
	response.Usage = normalizeUsage(response.Usage)
	rendered := messagesAPIResponse{
		ID:      response.ID,
		Type:    "message",
		Role:    "assistant",
		Model:   response.Model,
		Content: []messagesOutputBlock{},
		Usage: messagesUsage{
			InputTokens:  response.Usage.PromptTokens,
			OutputTokens: response.Usage.CompletionTokens,
			Source:       string(response.Usage.Source),
		},
	}
	if len(response.Choices) == 0 {
		return rendered
	}
	choice := response.Choices[0]
	rendered.StopReason = anthropicStopReason(choice.FinishReason)
	message := choice.Message
	if message.Content.Text != nil && strings.TrimSpace(*message.Content.Text) != "" {
		rendered.Content = append(rendered.Content, messagesOutputBlock{
			Type: "text",
			Text: *message.Content.Text,
		})
	}
	for _, part := range message.Content.Parts {
		if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
			rendered.Content = append(rendered.Content, messagesOutputBlock{
				Type: "text",
				Text: part.Text,
			})
		}
	}
	for _, toolCall := range message.ToolCalls {
		rendered.Content = append(rendered.Content, messagesOutputBlock{
			Type:  "tool_use",
			ID:    toolCall.ID,
			Name:  toolCall.Function.Name,
			Input: rawToolInput(toolCall.Function.Arguments),
		})
	}
	return rendered
}

func anthropicStopReason(finishReason string) string {
	switch strings.TrimSpace(finishReason) {
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	case "stop":
		return "end_turn"
	default:
		return strings.TrimSpace(finishReason)
	}
}

func rawToolInput(arguments string) json.RawMessage {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return json.RawMessage(`{}`)
	}
	if json.Valid([]byte(trimmed)) {
		return json.RawMessage(trimmed)
	}
	encoded, _ := json.Marshal(map[string]string{"_raw_arguments": trimmed})
	return encoded
}
