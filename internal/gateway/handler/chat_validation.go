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
		switch tool.Type {
		case "function":
			if strings.TrimSpace(tool.Function.Name) == "" {
				return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_tool", "tools", "Function tools must include function.name.")
			}
		case "hosted":
			if tool.Hosted == nil || strings.TrimSpace(tool.Hosted.Name) == "" {
				return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_hosted_tool", "tools", "Hosted tools must include hosted.name.")
			}
		default:
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_tool", "tools", "Tools must be function or hosted definitions.")
		}
	}
	if err := validateReasoningOptions(req.Reasoning); err != nil {
		return err
	}
	if err := validatePolarisChatOptions(req.Polaris); err != nil {
		return err
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
	if req.ResponseFormat != nil && req.ResponseFormat.Type == "json_schema" && req.ResponseFormat.JSONSchema != nil && req.ResponseFormat.JSONSchema.Strict && requestEnablesCitations(req) {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "citations_incompatible_with_strict_json", "response_format", "Citations are not compatible with strict JSON schema output.")
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
		case "file":
			if !validFilePart(part.File) {
				return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_file", "messages.content.file", "File content must include file_id, url, or data.")
			}
		case "document":
			if !validFilePart(part.Document) {
				return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_document", "messages.content.document", "Document content must include file_id, url, or data.")
			}
		default:
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_content_part", "messages.content.type", "Unsupported content part type.")
		}
	}
	return nil
}

func requiredCapabilities(req *modality.ChatRequest, allowDerivedFileUnderstanding bool) ([]modality.Capability, error) {
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
		if requestHasHostedTools(req) {
			addHostedToolCapabilities(req, add)
		}
		if requestHasFunctionTools(req) || len(req.ToolChoice) > 0 {
			add(modality.CapabilityFunctionCalling)
		}
	}
	if req.Reasoning != nil {
		add(modality.CapabilityReasoning)
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
			case "file":
				if !allowDerivedFileUnderstanding {
					addFilePartCapabilities(part.File, add)
				}
			case "document":
				if !allowDerivedFileUnderstanding {
					addFilePartCapabilities(part.Document, add)
				}
			}
		}
	}

	return required, nil
}

func validateReasoningOptions(options *modality.ReasoningOptions) error {
	if options == nil {
		return nil
	}
	switch strings.TrimSpace(options.Effort) {
	case "", "minimal", "low", "medium", "high":
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_reasoning_effort", "reasoning.effort", "Reasoning effort must be minimal, low, medium, or high.")
	}
	if options.BudgetTokens < 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_reasoning_budget", "reasoning.budget_tokens", "Reasoning budget_tokens must not be negative.")
	}
	switch strings.TrimSpace(options.IncludeSummary) {
	case "", "none", "auto", "concise", "detailed":
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_reasoning_summary", "reasoning.include_summary", "Reasoning include_summary must be none, auto, concise, or detailed.")
	}
	return nil
}

func validatePolarisChatOptions(options *modality.PolarisChatOptions) error {
	if options == nil || options.FileUnderstanding == nil {
		return nil
	}
	switch strings.TrimSpace(options.FileUnderstanding.Mode) {
	case "", "disabled", "derived_context", "auto_fallback":
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_file_understanding_mode", "polaris.file_understanding.mode", "File understanding mode must be disabled, derived_context, or auto_fallback.")
	}
	switch strings.TrimSpace(options.FileUnderstanding.Profile) {
	case "", "fast", "balanced", "quality":
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_file_understanding_profile", "polaris.file_understanding.profile", "File understanding profile must be fast, balanced, or quality.")
	}
	return nil
}

func requestHasFunctionTools(req *modality.ChatRequest) bool {
	for _, tool := range req.Tools {
		if tool.Type == "" || tool.Type == "function" {
			return true
		}
	}
	return false
}

func requestHasHostedTools(req *modality.ChatRequest) bool {
	for _, tool := range req.Tools {
		if tool.Type == "hosted" {
			return true
		}
	}
	return false
}

func addHostedToolCapabilities(req *modality.ChatRequest, add func(modality.Capability)) {
	for _, tool := range req.Tools {
		if tool.Type != "hosted" || tool.Hosted == nil {
			continue
		}
		switch strings.TrimSpace(tool.Hosted.Name) {
		case "web_search":
			add(modality.CapabilityHostedToolWebSearch)
		case "code_interpreter":
			add(modality.CapabilityHostedToolCodeInterpreter)
		case "computer_use":
			add(modality.CapabilityHostedToolComputerUse)
		case "file_search":
			add(modality.CapabilityHostedToolFileSearch)
		case "image_generation":
			add(modality.CapabilityHostedToolImageGeneration)
		case "mcp":
			add(modality.CapabilityHostedToolMCP)
		case "url_context":
			add(modality.CapabilityHostedToolURLContext)
		}
	}
}

func requestEnablesCitations(req *modality.ChatRequest) bool {
	for _, message := range req.Messages {
		for _, part := range message.Content.Parts {
			if part.Type == "file" && part.File != nil && part.File.Citations != nil && *part.File.Citations {
				return true
			}
			if part.Type == "document" && part.Document != nil && part.Document.Citations != nil && *part.Document.Citations {
				return true
			}
		}
	}
	return false
}

func validFilePart(part *modality.FilePart) bool {
	if part == nil {
		return false
	}
	return strings.TrimSpace(part.FileID) != "" ||
		strings.TrimSpace(part.URL) != "" ||
		strings.TrimSpace(part.Data) != ""
}

func addFilePartCapabilities(part *modality.FilePart, add func(modality.Capability)) {
	if part == nil {
		return
	}
	mimeType := strings.ToLower(strings.TrimSpace(part.MimeType))
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		add(modality.CapabilityVision)
	case strings.HasPrefix(mimeType, "audio/"):
		add(modality.CapabilityAudioInput)
	case mimeType == "application/pdf":
		add(modality.CapabilityPDFInput)
	case strings.TrimSpace(part.FileID) != "" && mimeType == "":
		add(modality.CapabilityFileReference)
	default:
		add(modality.CapabilityDocumentInput)
	}
}
