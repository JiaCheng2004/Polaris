package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/gin-gonic/gin"
)

type ChatHandler struct {
	registry *provider.Registry
}

func NewChatHandler(registry *provider.Registry) *ChatHandler {
	return &ChatHandler{registry: registry}
}

func (h *ChatHandler) Complete(c *gin.Context) {
	var req modality.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if err := validateChatRequest(&req); err != nil {
		httputil.WriteError(c, err)
		return
	}

	requiredCapabilities, err := requiredCapabilities(&req)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}

	adapter, model, err := h.registry.GetChatAdapter(req.Model)
	if err != nil {
		writeRegistryError(c, err)
		return
	}

	auth := middleware.GetAuthContext(c)
	if !middleware.ModelAllowed(auth.AllowedModels, model.ID) {
		httputil.WriteError(c, httputil.NewError(http.StatusForbidden, "permission_error", "model_not_allowed", "model", "API key is not permitted to use this model."))
		return
	}

	if _, err := h.registry.RequireModel(model.ID, modality.ModalityChat, requiredCapabilities...); err != nil {
		writeRegistryError(c, err)
		return
	}

	req.Model = model.ID
	start := time.Now()

	if req.Stream {
		h.stream(c, adapter, model, &req, start)
		return
	}

	response, err := adapter.Complete(c.Request.Context(), &req)
	providerLatencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		apiErr := apiErrorFrom(err)
		middleware.SetRequestOutcome(c, middleware.RequestOutcome{
			Model:             model.ID,
			Provider:          model.Provider,
			Modality:          modality.ModalityChat,
			StatusCode:        apiErr.Status,
			ErrorType:         apiErr.Type,
			ProviderLatencyMs: providerLatencyMs,
		})
		httputil.WriteError(c, apiErr)
		return
	}

	response.Model = model.ID
	middleware.SetRequestOutcome(c, middleware.RequestOutcome{
		Model:             model.ID,
		Provider:          model.Provider,
		Modality:          modality.ModalityChat,
		StatusCode:        http.StatusOK,
		ProviderLatencyMs: providerLatencyMs,
		PromptTokens:      response.Usage.PromptTokens,
		CompletionTokens:  response.Usage.CompletionTokens,
		TotalTokens:       response.Usage.TotalTokens,
	})
	c.JSON(http.StatusOK, response)
}

func (h *ChatHandler) stream(c *gin.Context, adapter modality.ChatAdapter, model provider.Model, req *modality.ChatRequest, start time.Time) {
	stream, err := adapter.Stream(c.Request.Context(), req)
	providerLatencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		apiErr := apiErrorFrom(err)
		middleware.SetRequestOutcome(c, middleware.RequestOutcome{
			Model:             model.ID,
			Provider:          model.Provider,
			Modality:          modality.ModalityChat,
			StatusCode:        apiErr.Status,
			ErrorType:         apiErr.Type,
			ProviderLatencyMs: providerLatencyMs,
		})
		httputil.WriteError(c, apiErr)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)
	c.Writer.Flush()

	outcome := middleware.RequestOutcome{
		Model:             model.ID,
		Provider:          model.Provider,
		Modality:          modality.ModalityChat,
		StatusCode:        http.StatusOK,
		ProviderLatencyMs: providerLatencyMs,
	}

	for chunk := range stream {
		if chunk.Err != nil {
			apiErr := apiErrorFrom(chunk.Err)
			outcome.ErrorType = apiErr.Type
			middleware.SetRequestOutcome(c, outcome)
			if err := writeSSEData(c, httputil.ErrorEnvelope{
				Error: httputil.ErrorBody{
					Message: apiErr.Message,
					Type:    apiErr.Type,
					Code:    apiErr.Code,
					Param:   apiErr.Param,
				},
			}); err == nil {
				_ = writeSSEDone(c)
			}
			return
		}

		if chunk.Model == "" {
			chunk.Model = model.ID
		}
		if chunk.Usage != nil {
			outcome.PromptTokens = chunk.Usage.PromptTokens
			outcome.CompletionTokens = chunk.Usage.CompletionTokens
			outcome.TotalTokens = chunk.Usage.TotalTokens
		}
		if err := writeSSEData(c, chunk); err != nil {
			outcome.ErrorType = "provider_error"
			middleware.SetRequestOutcome(c, outcome)
			return
		}
	}

	middleware.SetRequestOutcome(c, outcome)
	_ = writeSSEDone(c)
}

func validateChatRequest(req *modality.ChatRequest) error {
	if strings.TrimSpace(req.Model) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	if len(req.Messages) == 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_messages", "messages", "Field 'messages' must contain at least one message.")
	}
	if req.Temperature != nil && (*req.Temperature < 0 || *req.Temperature > 2) {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_temperature", "temperature", "Field 'temperature' must be between 0 and 2.")
	}
	if req.TopP != nil && (*req.TopP < 0 || *req.TopP > 1) {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_top_p", "top_p", "Field 'top_p' must be between 0 and 1.")
	}
	if len(req.Stop) > 4 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "too_many_stop_sequences", "stop", "Field 'stop' may contain at most 4 sequences.")
	}
	for _, message := range req.Messages {
		switch message.Role {
		case "system", "user", "assistant", "tool":
		default:
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_role", "messages.role", "Messages must use the roles system, user, assistant, or tool.")
		}
		if message.Role == "tool" && strings.TrimSpace(message.ToolCallID) == "" {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_tool_call_id", "messages.tool_call_id", "Tool messages must include tool_call_id.")
		}
		if err := validateMessageContent(message.Content, message.Role); err != nil {
			return err
		}
	}
	for _, tool := range req.Tools {
		if tool.Type != "function" || strings.TrimSpace(tool.Function.Name) == "" {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_tool", "tools", "Tools must be function definitions with a name.")
		}
	}
	if req.ResponseFormat != nil {
		switch req.ResponseFormat.Type {
		case "json_object":
		case "json_schema":
			if req.ResponseFormat.JSONSchema == nil {
				return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_json_schema", "response_format", "json_schema response formats must include json_schema.")
			}
		default:
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_response_format", "response_format", "Unsupported response_format type.")
		}
	}
	return nil
}

func validateMessageContent(content modality.MessageContent, role string) error {
	if content.Text != nil {
		return nil
	}
	if len(content.Parts) == 0 && role != "assistant" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "empty_content", "messages.content", "Message content cannot be empty.")
	}
	for _, part := range content.Parts {
		switch part.Type {
		case "text":
		case "image_url":
			if part.ImageURL == nil || strings.TrimSpace(part.ImageURL.URL) == "" {
				return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image", "messages.content.image_url", "Image content must include image_url.url.")
			}
		case "input_audio":
			if part.InputAudio == nil || part.InputAudio.Data == "" || part.InputAudio.Format == "" {
				return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_audio", "messages.content.input_audio", "Audio content must include input_audio.data and input_audio.format.")
			}
		default:
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_content_part", "messages.content.type", "Unsupported content part type.")
		}
	}
	return nil
}

func requiredCapabilities(req *modality.ChatRequest) ([]modality.Capability, error) {
	required := []modality.Capability{}
	add := func(capability modality.Capability) {
		for _, existing := range required {
			if existing == capability {
				return
			}
		}
		required = append(required, capability)
	}

	if req.Stream {
		add(modality.CapabilityStreaming)
	}
	if len(req.Tools) > 0 || len(req.ToolChoice) > 0 {
		add(modality.CapabilityFunctionCalling)
	}
	if req.ResponseFormat != nil {
		add(modality.CapabilityJSONMode)
	}
	for _, message := range req.Messages {
		for _, part := range message.Content.Parts {
			switch part.Type {
			case "image_url":
				add(modality.CapabilityVision)
			case "input_audio":
				add(modality.CapabilityAudioInput)
			}
		}
	}

	return required, nil
}

func writeRegistryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, provider.ErrUnknownAlias):
		httputil.WriteError(c, httputil.NewError(http.StatusNotFound, "model_not_found", "unknown_alias", "model", "Model alias is not defined."))
	case errors.Is(err, provider.ErrUnknownModel):
		httputil.WriteError(c, httputil.NewError(http.StatusNotFound, "model_not_found", "unknown_model", "model", "Requested model is not registered."))
	case errors.Is(err, provider.ErrModalityMismatch):
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "modality_mismatch", "model", "Requested model does not support the chat endpoint."))
	case errors.Is(err, provider.ErrCapabilityMissing):
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "capability_missing", "", "Requested model does not support the required capability."))
	case errors.Is(err, provider.ErrAdapterMissing):
		httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "adapter_unavailable", "model", "Requested model is configured but not available in this runtime build."))
	default:
		httputil.WriteError(c, err)
	}
}

func apiErrorFrom(err error) *httputil.APIError {
	var apiErr *httputil.APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return httputil.NewError(http.StatusInternalServerError, "internal_error", "internal_error", "", "An internal error occurred.")
}

func writeSSEData(c *gin.Context, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := c.Writer.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := c.Writer.Write(payload); err != nil {
		return err
	}
	if _, err := c.Writer.Write([]byte("\n\n")); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

func writeSSEDone(c *gin.Context) error {
	if _, err := c.Writer.Write([]byte("data: [DONE]\n\n")); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}
