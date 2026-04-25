package google

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
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
	payload, err := a.translateRequest(req)
	if err != nil {
		return nil, err
	}

	var response generateContentResponse
	if err := a.client.JSON(ctx, a.generatePath(req.Model), payload, &response); err != nil {
		return nil, err
	}

	translated, err := a.translateResponse(response, req.Model)
	if err != nil {
		return nil, err
	}
	return translated, nil
}

func (a *ChatAdapter) Stream(ctx context.Context, req *modality.ChatRequest) (<-chan modality.ChatChunk, error) {
	payload, err := a.translateRequest(req)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.Stream(ctx, a.streamPath(req.Model), payload)
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

func (a *ChatAdapter) CountTokens(ctx context.Context, req *modality.ChatRequest) (*modality.TokenCountResult, error) {
	payload, err := a.translateRequest(req)
	if err != nil {
		return nil, err
	}

	var response googleCountTokensResponse
	if err := a.client.JSON(ctx, a.countTokensPath(req.Model), googleCountTokensRequest{GenerateContentRequest: payload}, &response); err != nil {
		return nil, err
	}

	return &modality.TokenCountResult{
		InputTokens: response.TotalTokens,
		Source:      modality.TokenCountSourceProviderReported,
		Notes: []string{
			"input tokens were returned by Gemini's native countTokens endpoint",
			"output_tokens_estimate remains a Polaris estimate derived from max_output_tokens limits",
		},
	}, nil
}

type generateContentRequest struct {
	SystemInstruction *googleContent          `json:"system_instruction,omitempty"`
	Contents          []googleContent         `json:"contents"`
	Tools             []googleTool            `json:"tools,omitempty"`
	ToolConfig        *googleToolConfig       `json:"toolConfig,omitempty"`
	GenerationConfig  *googleGenerationConfig `json:"generationConfig,omitempty"`
}

type googleContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []googlePart `json:"parts"`
}

type googlePart struct {
	Text             string                  `json:"text,omitempty"`
	InlineData       *googleBlob             `json:"inlineData,omitempty"`
	FileData         *googleBlob             `json:"fileData,omitempty"`
	FunctionCall     *googleFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *googleFunctionResponse `json:"functionResponse,omitempty"`
}

type googleBlob struct {
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"`
	FileURI  string `json:"fileUri,omitempty"`
}

type googleFunctionCall struct {
	ID   string         `json:"id,omitempty"`
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type googleFunctionResponse struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response,omitempty"`
}

type googleTool struct {
	FunctionDeclarations []googleFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

type googleFunctionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type googleToolConfig struct {
	FunctionCallingConfig googleFunctionCallingConfig `json:"functionCallingConfig"`
}

type googleFunctionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"`
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

type googleGenerationConfig struct {
	Temperature      *float64       `json:"temperature,omitempty"`
	TopP             *float64       `json:"topP,omitempty"`
	MaxOutputTokens  int            `json:"maxOutputTokens,omitempty"`
	StopSequences    []string       `json:"stopSequences,omitempty"`
	ResponseMimeType string         `json:"responseMimeType,omitempty"`
	ResponseSchema   map[string]any `json:"responseSchema,omitempty"`
}

type generateContentResponse struct {
	Candidates    []googleCandidate   `json:"candidates"`
	UsageMetadata googleUsageMetadata `json:"usageMetadata"`
	ResponseID    string              `json:"responseId"`
	ModelVersion  string              `json:"modelVersion"`
}

type googleCandidate struct {
	Content      googleContent `json:"content"`
	FinishReason string        `json:"finishReason"`
	Index        int           `json:"index"`
}

type googleUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type googleCountTokensRequest struct {
	GenerateContentRequest generateContentRequest `json:"generateContentRequest,omitempty"`
}

type googleCountTokensResponse struct {
	TotalTokens int `json:"totalTokens"`
}

func (a *ChatAdapter) translateRequest(req *modality.ChatRequest) (generateContentRequest, error) {
	payload := generateContentRequest{}
	toolNamesByID := map[string]string{}

	for _, message := range req.Messages {
		for _, toolCall := range message.ToolCalls {
			if toolCall.ID != "" && toolCall.Function.Name != "" {
				toolNamesByID[toolCall.ID] = toolCall.Function.Name
			}
		}

		switch message.Role {
		case "system":
			text, err := contentToText(message.Content)
			if err != nil {
				return generateContentRequest{}, err
			}
			if strings.TrimSpace(text) != "" {
				payload.SystemInstruction = &googleContent{
					Parts: []googlePart{{Text: text}},
				}
			}
		case "user":
			parts, err := translateContentParts(message.Content)
			if err != nil {
				return generateContentRequest{}, err
			}
			payload.Contents = append(payload.Contents, googleContent{Role: "user", Parts: parts})
		case "assistant":
			parts, err := translateContentParts(message.Content)
			if err != nil {
				return generateContentRequest{}, err
			}
			for _, toolCall := range message.ToolCalls {
				args := map[string]any{}
				if strings.TrimSpace(toolCall.Function.Arguments) != "" {
					if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
						args["_raw_arguments"] = toolCall.Function.Arguments
					}
				}
				parts = append(parts, googlePart{
					FunctionCall: &googleFunctionCall{
						ID:   toolCall.ID,
						Name: toolCall.Function.Name,
						Args: args,
					},
				})
			}
			payload.Contents = append(payload.Contents, googleContent{Role: "model", Parts: parts})
		case "tool":
			name := firstNonEmpty(message.Name, toolNamesByID[message.ToolCallID])
			if name == "" {
				return generateContentRequest{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_tool_name", "messages.name", "Tool messages for Google models must include a tool name or follow a matching assistant tool call.")
			}
			text, err := contentToText(message.Content)
			if err != nil {
				return generateContentRequest{}, err
			}
			payload.Contents = append(payload.Contents, googleContent{
				Role: "user",
				Parts: []googlePart{
					{
						FunctionResponse: &googleFunctionResponse{
							ID:   message.ToolCallID,
							Name: name,
							Response: map[string]any{
								"result": text,
							},
						},
					},
				},
			})
		default:
			return generateContentRequest{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_role", "messages.role", "Unsupported message role.")
		}
	}

	if len(req.Tools) > 0 {
		declarations := make([]googleFunctionDeclaration, 0, len(req.Tools))
		for _, tool := range req.Tools {
			declarations = append(declarations, googleFunctionDeclaration{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			})
		}
		payload.Tools = []googleTool{{FunctionDeclarations: declarations}}
		if len(req.ToolChoice) > 0 {
			toolConfig, err := translateToolChoice(req.ToolChoice)
			if err != nil {
				return generateContentRequest{}, err
			}
			payload.ToolConfig = toolConfig
		}
	}

	var generationConfig *googleGenerationConfig
	if req.Temperature != nil || req.TopP != nil || req.MaxTokens > 0 || len(req.Stop) > 0 || req.ResponseFormat != nil {
		generationConfig = &googleGenerationConfig{
			Temperature:     req.Temperature,
			TopP:            req.TopP,
			MaxOutputTokens: req.MaxTokens,
		}
		if len(req.Stop) > 0 {
			generationConfig.StopSequences = append([]string(nil), req.Stop...)
		}
		if req.ResponseFormat != nil {
			switch req.ResponseFormat.Type {
			case "json_object":
				generationConfig.ResponseMimeType = "application/json"
			case "json_schema":
				generationConfig.ResponseMimeType = "application/json"
				if req.ResponseFormat.JSONSchema == nil || len(req.ResponseFormat.JSONSchema.Schema) == 0 {
					return generateContentRequest{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_json_schema", "response_format", "json_schema response formats must include json_schema.schema.")
				}
				var schema map[string]any
				if err := json.Unmarshal(req.ResponseFormat.JSONSchema.Schema, &schema); err != nil {
					return generateContentRequest{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json_schema", "response_format", "json_schema.schema must be valid JSON.")
				}
				generationConfig.ResponseSchema = schema
			}
		}
	}
	payload.GenerationConfig = generationConfig

	return payload, nil
}

func translateToolChoice(raw json.RawMessage) (*googleToolConfig, error) {
	var stringChoice string
	if err := json.Unmarshal(raw, &stringChoice); err == nil {
		switch stringChoice {
		case "auto":
			return &googleToolConfig{FunctionCallingConfig: googleFunctionCallingConfig{Mode: "AUTO"}}, nil
		case "required":
			return &googleToolConfig{FunctionCallingConfig: googleFunctionCallingConfig{Mode: "ANY"}}, nil
		case "none":
			return &googleToolConfig{FunctionCallingConfig: googleFunctionCallingConfig{Mode: "NONE"}}, nil
		default:
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_tool_choice", "tool_choice", "Unsupported tool_choice value.")
		}
	}

	var objectChoice struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &objectChoice); err != nil {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_tool_choice", "tool_choice", "tool_choice must be a string or function selector object.")
	}
	if objectChoice.Type != "function" || strings.TrimSpace(objectChoice.Function.Name) == "" {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_tool_choice", "tool_choice", "Function tool_choice must include function.name.")
	}
	return &googleToolConfig{
		FunctionCallingConfig: googleFunctionCallingConfig{
			Mode:                 "ANY",
			AllowedFunctionNames: []string{objectChoice.Function.Name},
		},
	}, nil
}

func translateContentParts(content modality.MessageContent) ([]googlePart, error) {
	if content.Text != nil {
		if *content.Text == "" {
			return []googlePart{}, nil
		}
		return []googlePart{{Text: *content.Text}}, nil
	}

	parts := make([]googlePart, 0, len(content.Parts))
	for _, part := range content.Parts {
		switch part.Type {
		case "text":
			parts = append(parts, googlePart{Text: part.Text})
		case "image_url":
			if part.ImageURL == nil || strings.TrimSpace(part.ImageURL.URL) == "" {
				return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image", "messages.content.image_url", "Image content must include image_url.url.")
			}
			translated, err := translateImagePart(part.ImageURL.URL)
			if err != nil {
				return nil, err
			}
			parts = append(parts, translated)
		case "input_audio":
			if part.InputAudio == nil || strings.TrimSpace(part.InputAudio.Data) == "" || strings.TrimSpace(part.InputAudio.Format) == "" {
				return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_audio", "messages.content.input_audio", "Audio content must include input_audio.data and input_audio.format.")
			}
			parts = append(parts, googlePart{
				InlineData: &googleBlob{
					MimeType: audioMimeType(part.InputAudio.Format),
					Data:     part.InputAudio.Data,
				},
			})
		default:
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_content_part", "messages.content.type", "Unsupported content part type.")
		}
	}
	return parts, nil
}

func translateImagePart(raw string) (googlePart, error) {
	if strings.HasPrefix(raw, "data:") {
		header, data, ok := strings.Cut(strings.TrimPrefix(raw, "data:"), ",")
		if !ok {
			return googlePart{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image_data_uri", "messages.content.image_url.url", "Invalid image data URI.")
		}
		mediaType := "image/png"
		if value, _, _ := strings.Cut(header, ";"); value != "" {
			mediaType = value
		}
		if !strings.Contains(header, ";base64") {
			data = base64.StdEncoding.EncodeToString([]byte(data))
		}
		return googlePart{
			InlineData: &googleBlob{
				MimeType: mediaType,
				Data:     data,
			},
		}, nil
	}

	return googlePart{
		FileData: &googleBlob{
			MimeType: guessMimeType(raw),
			FileURI:  raw,
		},
	}, nil
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

func (a *ChatAdapter) translateResponse(response generateContentResponse, canonicalModel string) (*modality.ChatResponse, error) {
	candidate := firstCandidate(response.Candidates)
	textParts, toolCalls := translateCandidateParts(candidate.Content.Parts)
	message := modality.ChatMessage{Role: "assistant", ToolCalls: toolCalls}
	if len(textParts) > 0 {
		message.Content = modality.NewTextContent(strings.Join(textParts, ""))
	}

	usage := modality.Usage{
		PromptTokens:     response.UsageMetadata.PromptTokenCount,
		CompletionTokens: response.UsageMetadata.CandidatesTokenCount,
		TotalTokens:      response.UsageMetadata.TotalTokenCount,
	}

	return &modality.ChatResponse{
		ID:      responseID(response.ResponseID, canonicalModel),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   firstNonEmpty(canonicalModel, a.model),
		Choices: []modality.ChatChoice{
			{
				Index:        candidate.Index,
				Message:      message,
				FinishReason: normalizeFinishReason(candidate.FinishReason),
			},
		},
		Usage: usage,
	}, nil
}

func (a *ChatAdapter) decodeStream(body io.Reader, canonicalModel string, dst chan<- modality.ChatChunk) error {
	reader := bufio.NewReader(body)
	var dataLines []string
	roleEmitted := false

	flush := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = nil
		if payload == "" {
			return nil
		}

		var response generateContentResponse
		if err := json.Unmarshal([]byte(payload), &response); err != nil {
			return fmt.Errorf("decode google stream chunk: %w", err)
		}

		candidate := firstCandidate(response.Candidates)
		chunkID := responseID(response.ResponseID, canonicalModel)
		created := time.Now().Unix()
		model := firstNonEmpty(canonicalModel, a.model)

		if !roleEmitted {
			dst <- modality.ChatChunk{
				ID:      chunkID,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   model,
				Choices: []modality.ChatChunkChoice{{Index: candidate.Index, Delta: modality.ChatDelta{Role: "assistant"}}},
			}
			roleEmitted = true
		}

		textParts, toolCalls := translateCandidateParts(candidate.Content.Parts)
		for _, text := range textParts {
			if text == "" {
				continue
			}
			dst <- modality.ChatChunk{
				ID:      chunkID,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   model,
				Choices: []modality.ChatChunkChoice{{Index: candidate.Index, Delta: modality.ChatDelta{Content: text}}},
			}
		}
		if len(toolCalls) > 0 {
			dst <- modality.ChatChunk{
				ID:      chunkID,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   model,
				Choices: []modality.ChatChunkChoice{{Index: candidate.Index, Delta: modality.ChatDelta{ToolCalls: toolCalls}}},
			}
		}

		if finishReason := normalizeFinishReason(candidate.FinishReason); finishReason != "" || response.UsageMetadata.TotalTokenCount > 0 {
			finish := finishReason
			if finish == "" {
				finish = "stop"
			}
			dst <- modality.ChatChunk{
				ID:      chunkID,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   model,
				Choices: []modality.ChatChunkChoice{{Index: candidate.Index, Delta: modality.ChatDelta{}, FinishReason: &finish}},
				Usage: &modality.Usage{
					PromptTokens:     response.UsageMetadata.PromptTokenCount,
					CompletionTokens: response.UsageMetadata.CandidatesTokenCount,
					TotalTokens:      response.UsageMetadata.TotalTokenCount,
				},
			}
		}

		return nil
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			if err == io.EOF {
				return flush()
			}
			return fmt.Errorf("read google stream: %w", err)
		}

		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			if flushErr := flush(); flushErr != nil {
				return flushErr
			}
		} else if strings.HasPrefix(trimmed, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
		}

		if err == io.EOF {
			return flush()
		}
	}
}

func translateCandidateParts(parts []googlePart) ([]string, []modality.ToolCall) {
	var textParts []string
	var toolCalls []modality.ToolCall
	for index, part := range parts {
		if strings.TrimSpace(part.Text) != "" {
			textParts = append(textParts, part.Text)
		}
		if part.FunctionCall != nil {
			arguments := "{}"
			if len(part.FunctionCall.Args) > 0 {
				if raw, err := json.Marshal(part.FunctionCall.Args); err == nil {
					arguments = string(raw)
				}
			}
			toolCalls = append(toolCalls, modality.ToolCall{
				ID:   firstNonEmpty(part.FunctionCall.ID, fmt.Sprintf("call_%d", index)),
				Type: "function",
				Function: modality.ToolCallFunction{
					Name:      part.FunctionCall.Name,
					Arguments: arguments,
				},
			})
		}
	}
	return textParts, toolCalls
}

func (a *ChatAdapter) generatePath(requestModel string) string {
	return "/v1beta/models/" + providerModelName(requestModel, a.model) + ":generateContent"
}

func (a *ChatAdapter) streamPath(requestModel string) string {
	return "/v1beta/models/" + providerModelName(requestModel, a.model) + ":streamGenerateContent?alt=sse"
}

func (a *ChatAdapter) countTokensPath(requestModel string) string {
	return "/v1beta/models/" + providerModelName(requestModel, a.model) + ":countTokens"
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

func normalizeFinishReason(reason string) string {
	switch strings.ToUpper(strings.TrimSpace(reason)) {
	case "":
		return ""
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT":
		return "content_filter"
	default:
		return strings.ToLower(reason)
	}
}

func firstCandidate(candidates []googleCandidate) googleCandidate {
	if len(candidates) == 0 {
		return googleCandidate{}
	}
	return candidates[0]
}

func responseID(responseID string, canonicalModel string) string {
	if responseID != "" {
		return responseID
	}
	return fmt.Sprintf("google-%d-%s", time.Now().UnixNano(), strings.ReplaceAll(firstNonEmpty(canonicalModel, "chat"), "/", "-"))
}

func guessMimeType(raw string) string {
	if parsed := mime.TypeByExtension(path.Ext(raw)); parsed != "" {
		return parsed
	}
	return "application/octet-stream"
}

func audioMimeType(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "wav":
		return "audio/wav"
	case "mp3":
		return "audio/mpeg"
	case "ogg":
		return "audio/ogg"
	case "flac":
		return "audio/flac"
	default:
		return "application/octet-stream"
	}
}
