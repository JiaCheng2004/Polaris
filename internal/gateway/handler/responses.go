package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gin-gonic/gin"
)

type responsesRequest struct {
	Model           string                    `json:"model"`
	Routing         *modality.RoutingOptions  `json:"routing,omitempty"`
	Input           json.RawMessage           `json:"input"`
	Instructions    string                    `json:"instructions,omitempty"`
	Temperature     *float64                  `json:"temperature,omitempty"`
	TopP            *float64                  `json:"top_p,omitempty"`
	MaxOutputTokens int                       `json:"max_output_tokens,omitempty"`
	Stream          bool                      `json:"stream,omitempty"`
	Tools           []modality.ToolDefinition `json:"tools,omitempty"`
	ToolChoice      json.RawMessage           `json:"tool_choice,omitempty"`
	Text            *responsesTextConfig      `json:"text,omitempty"`
	Metadata        map[string]string         `json:"metadata,omitempty"`
}

type responsesTextConfig struct {
	Format *modality.ResponseFormat `json:"format,omitempty"`
}

type responsesAPIResponse struct {
	ID         string                `json:"id"`
	Object     string                `json:"object"`
	CreatedAt  int64                 `json:"created_at"`
	Status     string                `json:"status"`
	Model      string                `json:"model"`
	Output     []responsesOutputItem `json:"output"`
	OutputText string                `json:"output_text,omitempty"`
	Usage      responsesUsage        `json:"usage"`
	Metadata   map[string]string     `json:"metadata,omitempty"`
}

type responsesOutputItem struct {
	ID        string                 `json:"id,omitempty"`
	Type      string                 `json:"type"`
	Role      string                 `json:"role,omitempty"`
	Content   []responsesContentItem `json:"content,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Arguments string                 `json:"arguments,omitempty"`
	CallID    string                 `json:"call_id,omitempty"`
}

type responsesContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type responsesUsage struct {
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	TotalTokens  int    `json:"total_tokens"`
	Source       string `json:"source,omitempty"`
}

func (h *ChatHandler) Responses(c *gin.Context) {
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		if httputil.IsRequestBodyTooLarge(err) {
			httputil.WriteError(c, httputil.RequestBodyTooLargeError(0))
			return
		}
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}

	var req responsesRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}

	chatReq, err := normalizeResponsesRequest(&req)
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
		if adapter, ok := primary.adapter.(modality.NativeResponsesAdapter); ok {
			if req.Stream {
				if err := h.streamNativeResponses(c, adapter, primary, rawBody); err == nil {
					return
				} else if shouldRetryWithFallback(apiErrorFrom(err)) && len(fallbacks) > 0 {
					stream, selected, outcome, fallbackModel, fallbackErr := h.openFallbackConversationStream(c, primary, fallbacks, chatReq, "responses")
					if fallbackErr != nil {
						middleware.SetRequestOutcome(c, outcome)
						writeConversationTargetError(c, "responses", fallbackErr)
						return
					}
					writeConversationFallbackHeaders(c, h, primary.model.ID, outcome, fallbackModel)
					h.streamResponses(c, selected, stream, outcome, chatReq, req.Metadata)
					return
				} else {
					writeNativeConversationError(c, primary, "responses", err)
				}
				return
			}
			if err := h.nativeResponses(c, adapter, primary, rawBody); err == nil {
				return
			} else if shouldRetryWithFallback(apiErrorFrom(err)) && len(fallbacks) > 0 {
				response, outcome, fallbackModel, fallbackErr := h.completeFallbackConversation(c, primary, fallbacks, chatReq, "responses")
				if fallbackErr != nil {
					middleware.SetRequestOutcome(c, outcome)
					writeConversationTargetError(c, "responses", fallbackErr)
					return
				}
				writeConversationFallbackHeaders(c, h, primary.model.ID, outcome, fallbackModel)
				middleware.SetRequestOutcome(c, outcome)
				c.JSON(http.StatusOK, renderResponsesResponse(response, req.Metadata))
				return
			} else {
				writeNativeConversationError(c, primary, "responses", err)
			}
			return
		}
	}
	if req.Stream {
		stream, selected, outcome, fallbackModel, err := h.openConversationStream(c, chatReq, "responses")
		if err != nil {
			middleware.SetRequestOutcome(c, outcome)
			writeConversationTargetError(c, "responses", err)
			return
		}
		if fallbackModel != "" {
			c.Header("X-Polaris-Fallback", fallbackModel)
			h.metrics.IncFailover(chatReq.Model, fallbackModel)
			c.Header("X-Polaris-Resolved-Model", outcome.Model)
			c.Header("X-Polaris-Resolved-Provider", outcome.Provider)
		}
		h.streamResponses(c, selected, stream, outcome, chatReq, req.Metadata)
		return
	}

	response, outcome, fallbackModel, err := h.executeConversation(c, chatReq, "responses")
	if err != nil {
		middleware.SetRequestOutcome(c, outcome)
		writeConversationTargetError(c, "responses", err)
		return
	}
	if fallbackModel != "" {
		c.Header("X-Polaris-Fallback", fallbackModel)
		h.metrics.IncFailover(chatReq.Model, fallbackModel)
		c.Header("X-Polaris-Resolved-Model", outcome.Model)
		c.Header("X-Polaris-Resolved-Provider", outcome.Provider)
	}
	middleware.SetRequestOutcome(c, outcome)
	c.JSON(http.StatusOK, renderResponsesResponse(response, req.Metadata))
}

func (h *ChatHandler) nativeResponses(c *gin.Context, adapter modality.NativeResponsesAdapter, target chatTarget, rawBody json.RawMessage) error {
	response, err := adapter.CreateResponse(c.Request.Context(), rawBody, target.model.ID)
	outcome := middleware.RequestOutcome{
		Model:           target.model.ID,
		Provider:        target.model.Provider,
		Modality:        modality.ModalityChat,
		InterfaceFamily: "responses",
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

func (h *ChatHandler) streamNativeResponses(c *gin.Context, adapter modality.NativeResponsesAdapter, target chatTarget, rawBody json.RawMessage) error {
	stream, err := adapter.StreamResponse(c.Request.Context(), rawBody, target.model.ID)
	outcome := middleware.RequestOutcome{
		Model:           target.model.ID,
		Provider:        target.model.Provider,
		Modality:        modality.ModalityChat,
		InterfaceFamily: "responses",
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

func normalizeResponsesRequest(req *responsesRequest) (*modality.ChatRequest, error) {
	if strings.TrimSpace(req.Model) == "" {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	messages, err := normalizeResponsesInput(req.Input)
	if err != nil {
		return nil, err
	}
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		messages = append([]modality.ChatMessage{{
			Role:    "system",
			Content: modality.NewTextContent(instructions),
		}}, messages...)
	}
	chatReq := &modality.ChatRequest{
		Model:       req.Model,
		Routing:     req.Routing,
		Messages:    messages,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxOutputTokens,
		Tools:       append([]modality.ToolDefinition(nil), req.Tools...),
		ToolChoice:  append(json.RawMessage(nil), req.ToolChoice...),
		Metadata:    cloneStringMap(req.Metadata),
	}
	if req.Text != nil {
		chatReq.ResponseFormat = req.Text.Format
	}
	return chatReq, nil
}

func normalizeResponsesInput(raw json.RawMessage) ([]modality.ChatMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_input", "input", "Field 'input' is required.")
	}
	switch trimmed[0] {
	case '"':
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_input", "input", "Field 'input' must be a string or message object.")
		}
		return []modality.ChatMessage{{
			Role:    "user",
			Content: modality.NewTextContent(text),
		}}, nil
	case '{':
		var message modality.ChatMessage
		if err := json.Unmarshal(trimmed, &message); err != nil {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_input", "input", "Field 'input' must be a string or message object.")
		}
		return []modality.ChatMessage{message}, nil
	case '[':
		var messages []modality.ChatMessage
		if err := json.Unmarshal(trimmed, &messages); err != nil {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_input", "input", "Field 'input' array must contain message objects.")
		}
		return messages, nil
	default:
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_input", "input", "Field 'input' must be a string or message object.")
	}
}

func renderResponsesResponse(response *modality.ChatResponse, metadata map[string]string) responsesAPIResponse {
	response.Usage = normalizeUsage(response.Usage)
	rendered := responsesAPIResponse{
		ID:        response.ID,
		Object:    "response",
		CreatedAt: response.Created,
		Status:    "completed",
		Model:     response.Model,
		Output:    []responsesOutputItem{},
		Usage: responsesUsage{
			InputTokens:  response.Usage.PromptTokens,
			OutputTokens: response.Usage.CompletionTokens,
			TotalTokens:  response.Usage.TotalTokens,
			Source:       string(response.Usage.Source),
		},
		Metadata: cloneStringMap(metadata),
	}
	if len(response.Choices) == 0 {
		return rendered
	}

	message := response.Choices[0].Message
	messageItem := responsesOutputItem{
		ID:      firstNonEmptyString("msg_"+response.ID, response.ID),
		Type:    "message",
		Role:    "assistant",
		Content: renderResponsesContent(message.Content),
	}
	if len(messageItem.Content) > 0 {
		rendered.Output = append(rendered.Output, messageItem)
	}
	for _, toolCall := range message.ToolCalls {
		rendered.Output = append(rendered.Output, responsesOutputItem{
			ID:        firstNonEmptyString(toolCall.ID, "call"),
			Type:      "function_call",
			Name:      toolCall.Function.Name,
			Arguments: toolCall.Function.Arguments,
			CallID:    toolCall.ID,
		})
	}
	rendered.OutputText = extractMessageText(message.Content)
	return rendered
}

func renderResponsesContent(content modality.MessageContent) []responsesContentItem {
	if content.Text != nil {
		if strings.TrimSpace(*content.Text) == "" {
			return nil
		}
		return []responsesContentItem{{Type: "output_text", Text: *content.Text}}
	}
	items := make([]responsesContentItem, 0, len(content.Parts))
	for _, part := range content.Parts {
		if part.Type != "text" || strings.TrimSpace(part.Text) == "" {
			continue
		}
		items = append(items, responsesContentItem{Type: "output_text", Text: part.Text})
	}
	return items
}

func extractMessageText(content modality.MessageContent) string {
	if content.Text != nil {
		return *content.Text
	}
	texts := make([]string, 0, len(content.Parts))
	for _, part := range content.Parts {
		if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
