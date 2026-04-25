package handler

import (
	"net/http"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

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
