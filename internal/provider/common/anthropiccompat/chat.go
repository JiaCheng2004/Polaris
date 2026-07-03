package anthropiccompat

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

	"github.com/JiaCheng2004/Polaris/internal/apierror"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

type RequestTranslator func(req *modality.ChatRequest, stream bool, providerModel string) any
type ModelMapper func(providerModel string) string

// Options configures a ChatAdapter. The zero value is the plain
// Anthropic-compatible behavior used by the token-plan providers; the native
// Anthropic adapter enables thinking, hosted tools, and per-request betas.
type Options struct {
	DefaultMaxTokens  int
	Translator        RequestTranslator
	ModelMapper       ModelMapper
	EnableThinking    bool
	EnableHostedTools bool
	Betas             func(*modality.ChatRequest) []string
}

type ChatAdapter struct {
	client            *Client
	model             string
	translator        RequestTranslator
	defaultMaxTokens  int
	modelMapper       ModelMapper
	enableThinking    bool
	enableHostedTools bool
	betas             func(*modality.ChatRequest) []string
}

func NewChatAdapter(client *Client, model string, defaultMaxTokens int, translator RequestTranslator) *ChatAdapter {
	return NewChatAdapterWithOptions(client, model, Options{DefaultMaxTokens: defaultMaxTokens, Translator: translator})
}

func NewChatAdapterWithModelMapper(client *Client, model string, defaultMaxTokens int, translator RequestTranslator, modelMapper ModelMapper) *ChatAdapter {
	return NewChatAdapterWithOptions(client, model, Options{DefaultMaxTokens: defaultMaxTokens, Translator: translator, ModelMapper: modelMapper})
}

// NewChatAdapterWithOptions builds a ChatAdapter with the full option set.
func NewChatAdapterWithOptions(client *Client, model string, opts Options) *ChatAdapter {
	if opts.DefaultMaxTokens <= 0 {
		opts.DefaultMaxTokens = 4096
	}
	return &ChatAdapter{
		client:            client,
		model:             model,
		translator:        opts.Translator,
		defaultMaxTokens:  opts.DefaultMaxTokens,
		modelMapper:       opts.ModelMapper,
		enableThinking:    opts.EnableThinking,
		enableHostedTools: opts.EnableHostedTools,
		betas:             opts.Betas,
	}
}

func (a *ChatAdapter) requestBetas(req *modality.ChatRequest) []string {
	if a.betas == nil {
		return nil
	}
	return a.betas(req)
}

// CountTokensPayload builds the request body for Anthropic's count_tokens
// endpoint from a chat request, reusing the standard message/tool translation.
func (a *ChatAdapter) CountTokensPayload(req *modality.ChatRequest) (any, error) {
	full, err := defaultTranslateRequest(req, false, a.wireModelName(req.Model), a.defaultMaxTokens, a.enableThinking, a.enableHostedTools)
	if err != nil {
		return nil, err
	}
	return anthropicCountTokensRequest{
		Model:      full.Model,
		Messages:   full.Messages,
		System:     full.System,
		Tools:      full.Tools,
		ToolChoice: full.ToolChoice,
	}, nil
}

func (a *ChatAdapter) Complete(ctx context.Context, req *modality.ChatRequest) (*modality.ChatResponse, error) {
	payload, err := a.translateRequest(req, false)
	if err != nil {
		return nil, err
	}

	var response anthropicMessagesResponse
	if _, err := a.client.JSONWithBetas(ctx, "/v1/messages", payload, &response, a.requestBetas(req)); err != nil {
		return nil, err
	}

	return a.translateResponse(response, req.Model)
}

func (a *ChatAdapter) Stream(ctx context.Context, req *modality.ChatRequest) (<-chan modality.ChatChunk, error) {
	payload, err := a.translateRequest(req, true)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.StreamWithBetas(ctx, "/v1/messages", payload, a.requestBetas(req))
	if err != nil {
		return nil, err
	}

	stream := make(chan modality.ChatChunk)
	go func() {
		defer close(stream)
		defer func() {
			_ = resp.Body.Close()
		}()

		if err := decodeStream(resp.Body, req.Model, stream); err != nil {
			stream <- modality.ChatChunk{Err: err}
		}
	}()

	return stream, nil
}

type anthropicMessagesRequest struct {
	Model         string             `json:"model"`
	MaxTokens     int                `json:"max_tokens"`
	Messages      []anthropicMessage `json:"messages"`
	System        string             `json:"system,omitempty"`
	Temperature   *float64           `json:"temperature,omitempty"`
	TopP          *float64           `json:"top_p,omitempty"`
	Stream        bool               `json:"stream,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
	Tools         []anthropicTool    `json:"tools,omitempty"`
	ToolChoice    map[string]any     `json:"tool_choice,omitempty"`
	Thinking      *anthropicThinking `json:"thinking,omitempty"`
	Metadata      map[string]string  `json:"metadata,omitempty"`
}

type anthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

type anthropicCountTokensRequest struct {
	Model      string             `json:"model"`
	Messages   []anthropicMessage `json:"messages"`
	System     string             `json:"system,omitempty"`
	Tools      []anthropicTool    `json:"tools,omitempty"`
	ToolChoice map[string]any     `json:"tool_choice,omitempty"`
}

// CountTokensResponse is the parsed body of Anthropic's /v1/messages/count_tokens.
type CountTokensResponse struct {
	InputTokens int `json:"input_tokens"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type      string                `json:"type"`
	Text      string                `json:"text,omitempty"`
	Source    *anthropicImageSource `json:"source,omitempty"`
	Citations json.RawMessage       `json:"citations,omitempty"`
	ID        string                `json:"id,omitempty"`
	Name      string                `json:"name,omitempty"`
	Input     map[string]any        `json:"input,omitempty"`
	ToolUseID string                `json:"tool_use_id,omitempty"`
	Content   any                   `json:"content,omitempty"`
}

type anthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
	FileID    string `json:"file_id,omitempty"`
}

type anthropicCitations struct {
	Enabled bool `json:"enabled"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Type        string          `json:"type,omitempty"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	Config      map[string]any  `json:"-"`
}

// MarshalJSON emits the base tool fields plus any hosted-tool Config keys. For a
// plain function tool (no Type/Config) it produces the standard
// name/description/input_schema object.
func (t anthropicTool) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	for key, value := range t.Config {
		out[key] = value
	}
	if t.Name != "" {
		out["name"] = t.Name
	}
	if t.Type != "" {
		out["type"] = t.Type
	}
	if t.Description != "" {
		out["description"] = t.Description
	}
	if len(t.InputSchema) > 0 {
		out["input_schema"] = t.InputSchema
	}
	return json.Marshal(out)
}

type anthropicMessagesResponse struct {
	ID         string                  `json:"id"`
	Model      string                  `json:"model"`
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
	Usage      anthropicUsage          `json:"usage"`
}

type anthropicUsage struct {
	InputTokens                int `json:"input_tokens"`
	OutputTokens               int `json:"output_tokens"`
	CacheReadInputTokens       int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens   int `json:"cache_creation_input_tokens"`
	CacheCreation5mInputTokens int `json:"cache_creation_5m_input_tokens"`
	CacheCreation1hInputTokens int `json:"cache_creation_1h_input_tokens"`
	CacheCreation              struct {
		Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
		Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
}

func (u anthropicUsage) toModalityUsage() modality.Usage {
	cacheWrite5m := u.CacheCreation5mInputTokens + u.CacheCreation.Ephemeral5mInputTokens
	if cacheWrite5m == 0 {
		cacheWrite5m = u.CacheCreationInputTokens
	}
	cacheWrite1h := u.CacheCreation1hInputTokens + u.CacheCreation.Ephemeral1hInputTokens
	return modality.Usage{
		PromptTokens:       u.InputTokens,
		CompletionTokens:   u.OutputTokens,
		TotalTokens:        u.InputTokens + u.OutputTokens,
		CachedInputTokens:  u.CacheReadInputTokens,
		CacheWrite5mTokens: cacheWrite5m,
		CacheWrite1hTokens: cacheWrite1h,
	}
}

func (a *ChatAdapter) translateRequest(req *modality.ChatRequest, stream bool) (any, error) {
	providerModel := a.wireModelName(req.Model)
	if a.translator != nil {
		return a.translator(req, stream, providerModel), nil
	}
	return defaultTranslateRequest(req, stream, providerModel, a.defaultMaxTokens, a.enableThinking, a.enableHostedTools)
}

func (a *ChatAdapter) wireModelName(requestModel string) string {
	model := providerModelName(requestModel, a.model)
	if a.modelMapper != nil {
		return a.modelMapper(model)
	}
	return model
}

func defaultTranslateRequest(req *modality.ChatRequest, stream bool, providerModel string, defaultMaxTokens int, enableThinking bool, enableHostedTools bool) (anthropicMessagesRequest, error) {
	payload := anthropicMessagesRequest{
		Model:         providerModel,
		MaxTokens:     req.MaxTokens,
		Temperature:   req.Temperature,
		TopP:          req.TopP,
		Stream:        stream,
		StopSequences: append([]string(nil), req.Stop...),
		Metadata:      cloneStringMap(req.Metadata),
	}
	if payload.MaxTokens <= 0 {
		payload.MaxTokens = defaultMaxTokens
	}

	var systemParts []string
	for _, message := range req.Messages {
		if message.Role == "system" {
			text, err := contentToText(message.Content)
			if err != nil {
				return anthropicMessagesRequest{}, err
			}
			if text != "" {
				systemParts = append(systemParts, text)
			}
			continue
		}

		translated, err := translateMessage(message)
		if err != nil {
			return anthropicMessagesRequest{}, err
		}
		if len(translated.Content) == 0 {
			continue
		}
		payload.Messages = append(payload.Messages, translated)
	}
	payload.System = strings.Join(systemParts, "\n\n")

	for _, tool := range req.Tools {
		translatedTool, ok, err := translateAnthropicToolDefinition(tool, enableHostedTools)
		if err != nil {
			return anthropicMessagesRequest{}, err
		}
		if ok {
			payload.Tools = append(payload.Tools, translatedTool)
		}
	}
	if len(payload.Tools) > 0 && len(req.ToolChoice) > 0 {
		toolChoice, err := translateToolChoice(req.ToolChoice)
		if err != nil {
			return anthropicMessagesRequest{}, err
		}
		payload.ToolChoice = toolChoice
	}

	if enableThinking {
		if budget := modality.ReasoningBudgetTokens(req.Reasoning, req.Model); budget > 0 {
			payload.Thinking = &anthropicThinking{Type: "enabled", BudgetTokens: budget}
		}
	}

	return payload, nil
}

func translateAnthropicToolDefinition(tool modality.ToolDefinition, allowHosted bool) (anthropicTool, bool, error) {
	switch strings.TrimSpace(tool.Type) {
	case "", "function":
		if strings.TrimSpace(tool.Function.Name) == "" {
			return anthropicTool{}, false, nil
		}
		return anthropicTool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: tool.Function.Parameters,
		}, true, nil
	case "hosted":
		if !allowHosted || tool.Hosted == nil {
			return anthropicTool{}, false, nil
		}
		config := map[string]any{}
		for key, value := range tool.Hosted.Config {
			config[key] = value
		}
		switch strings.TrimSpace(tool.Hosted.Name) {
		case "web_search":
			return anthropicTool{Type: "web_search_20260209", Config: config}, true, nil
		case "code_interpreter":
			return anthropicTool{Type: "code_execution_20260120", Config: config}, true, nil
		case "computer_use":
			return anthropicTool{Type: "computer_20251124", Config: config}, true, nil
		case "url_context":
			return anthropicTool{Type: "web_fetch_20260209", Config: config}, true, nil
		case "mcp":
			return anthropicTool{Type: "mcp_toolset", Config: config}, true, nil
		default:
			return anthropicTool{}, false, apierror.NewError(http.StatusBadRequest, "capability_not_supported", "hosted_tool_not_supported", "tools", "Anthropic does not support the requested hosted tool.")
		}
	default:
		return anthropicTool{}, false, nil
	}
}

func translateMessage(message modality.ChatMessage) (anthropicMessage, error) {
	switch message.Role {
	case "user":
		blocks, err := translateContent(message.Content)
		if err != nil {
			return anthropicMessage{}, err
		}
		return anthropicMessage{Role: "user", Content: blocks}, nil
	case "assistant":
		blocks, err := translateContent(message.Content)
		if err != nil {
			return anthropicMessage{}, err
		}
		for _, toolCall := range message.ToolCalls {
			input := map[string]any{}
			if strings.TrimSpace(toolCall.Function.Arguments) != "" {
				if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &input); err != nil {
					input["_raw_arguments"] = toolCall.Function.Arguments
				}
			}
			blocks = append(blocks, anthropicContentBlock{
				Type:  "tool_use",
				ID:    toolCall.ID,
				Name:  toolCall.Function.Name,
				Input: input,
			})
		}
		return anthropicMessage{Role: "assistant", Content: blocks}, nil
	case "tool":
		text, err := contentToText(message.Content)
		if err != nil {
			return anthropicMessage{}, err
		}
		return anthropicMessage{
			Role: "user",
			Content: []anthropicContentBlock{
				{
					Type:      "tool_result",
					ToolUseID: message.ToolCallID,
					Content:   text,
				},
			},
		}, nil
	default:
		return anthropicMessage{}, apierror.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_role", "messages.role", "Unsupported message role.")
	}
}

func translateContent(content modality.MessageContent) ([]anthropicContentBlock, error) {
	if content.Text != nil {
		if *content.Text == "" {
			return nil, nil
		}
		return []anthropicContentBlock{{Type: "text", Text: *content.Text}}, nil
	}

	var blocks []anthropicContentBlock
	for _, part := range content.Parts {
		switch part.Type {
		case "text":
			blocks = append(blocks, anthropicContentBlock{Type: "text", Text: part.Text})
		case "image_url":
			if part.ImageURL == nil {
				return nil, apierror.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image", "messages.content.image_url", "Image content must include image_url.")
			}
			source, err := translateImageSource(part.ImageURL.URL)
			if err != nil {
				return nil, err
			}
			blocks = append(blocks, anthropicContentBlock{
				Type:   "image",
				Source: source,
			})
		case "input_audio":
			return nil, apierror.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_audio_input", "messages.content.input_audio", "Anthropic-compatible chat does not support audio input.")
		case "file", "document":
			block, err := translateFileBlock(part)
			if err != nil {
				return nil, err
			}
			blocks = append(blocks, block)
		default:
			return nil, apierror.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_content_part", "messages.content.type", "Unsupported content part type.")
		}
	}
	return blocks, nil
}

func translateFileBlock(part modality.ContentPart) (anthropicContentBlock, error) {
	filePart := part.File
	if part.Type == "document" {
		filePart = part.Document
	}
	if filePart == nil {
		return anthropicContentBlock{}, apierror.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_file", "messages.content.file", "File content must include file details.")
	}
	blockType := "document"
	if strings.HasPrefix(filePart.MimeType, "image/") {
		blockType = "image"
	}
	source, err := translateFileSource(filePart)
	if err != nil {
		return anthropicContentBlock{}, err
	}
	block := anthropicContentBlock{Type: blockType, Source: source}
	if filePart.Citations != nil && blockType == "document" {
		raw, _ := json.Marshal(anthropicCitations{Enabled: *filePart.Citations})
		block.Citations = raw
	}
	return block, nil
}

func translateFileSource(part *modality.FilePart) (*anthropicImageSource, error) {
	switch {
	case strings.TrimSpace(part.FileID) != "":
		return &anthropicImageSource{Type: "file", FileID: strings.TrimSpace(part.FileID)}, nil
	case strings.TrimSpace(part.Data) != "":
		mediaType := firstNonEmpty(part.MimeType, "application/octet-stream")
		return &anthropicImageSource{Type: "base64", MediaType: mediaType, Data: strings.TrimSpace(part.Data)}, nil
	case strings.TrimSpace(part.URL) != "":
		return &anthropicImageSource{Type: "url", URL: strings.TrimSpace(part.URL)}, nil
	default:
		return nil, apierror.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_file", "messages.content.file", "File content must include file_id, url, or data.")
	}
}

func translateImageSource(raw string) (*anthropicImageSource, error) {
	if strings.HasPrefix(raw, "data:") {
		header, data, ok := strings.Cut(strings.TrimPrefix(raw, "data:"), ",")
		if !ok {
			return nil, apierror.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image_data_uri", "messages.content.image_url.url", "Invalid image data URI.")
		}
		mediaType := "image/png"
		if value, _, _ := strings.Cut(header, ";"); value != "" {
			mediaType = value
		}
		if !strings.Contains(header, ";base64") {
			data = base64.StdEncoding.EncodeToString([]byte(data))
		}
		return &anthropicImageSource{
			Type:      "base64",
			MediaType: mediaType,
			Data:      data,
		}, nil
	}

	return &anthropicImageSource{
		Type: "url",
		URL:  raw,
	}, nil
}

func contentToText(content modality.MessageContent) (string, error) {
	if content.Text != nil {
		return *content.Text, nil
	}
	var parts []string
	for _, part := range content.Parts {
		if part.Type != "text" {
			return "", apierror.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_system_content", "messages.content", "System and tool messages must use text content.")
		}
		parts = append(parts, part.Text)
	}
	return strings.Join(parts, "\n"), nil
}

func translateToolChoice(raw json.RawMessage) (map[string]any, error) {
	var stringChoice string
	if err := json.Unmarshal(raw, &stringChoice); err == nil {
		switch stringChoice {
		case "auto":
			return map[string]any{"type": "auto"}, nil
		case "required":
			return map[string]any{"type": "any"}, nil
		case "none":
			return nil, nil
		default:
			return nil, apierror.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_tool_choice", "tool_choice", "Unsupported tool_choice value.")
		}
	}

	var objectChoice struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &objectChoice); err != nil {
		return nil, apierror.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_tool_choice", "tool_choice", "Invalid tool_choice payload.")
	}
	if objectChoice.Type != "function" || objectChoice.Function.Name == "" {
		return nil, apierror.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_tool_choice", "tool_choice", "Unsupported tool_choice payload.")
	}
	return map[string]any{
		"type": "tool",
		"name": objectChoice.Function.Name,
	}, nil
}

func (a *ChatAdapter) translateResponse(response anthropicMessagesResponse, canonicalModel string) (*modality.ChatResponse, error) {
	message, citations, err := responseContentToMessage(response.Content)
	if err != nil {
		return nil, err
	}

	return &modality.ChatResponse{
		ID:      response.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   firstNonEmpty(canonicalModel, a.model),
		Choices: []modality.ChatChoice{
			{
				Index:        0,
				Message:      message,
				FinishReason: mapStopReason(response.StopReason),
				Citations:    citations,
			},
		},
		Usage: response.Usage.toModalityUsage(),
	}, nil
}

func responseContentToMessage(content []anthropicContentBlock) (modality.ChatMessage, []modality.Citation, error) {
	var textParts []string
	var toolCalls []modality.ToolCall
	var citations []modality.Citation

	for _, block := range content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				textParts = append(textParts, block.Text)
			}
			citations = append(citations, anthropicCitationsFromRaw(block.Citations)...)
		case "tool_use":
			arguments := "{}"
			if len(block.Input) > 0 {
				raw, err := json.Marshal(block.Input)
				if err != nil {
					return modality.ChatMessage{}, nil, fmt.Errorf("marshal anthropic-compatible tool input: %w", err)
				}
				arguments = string(raw)
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
	}

	message := modality.ChatMessage{Role: "assistant"}
	if len(textParts) > 0 {
		message.Content = modality.NewTextContent(strings.Join(textParts, ""))
	}
	if len(toolCalls) > 0 {
		message.ToolCalls = toolCalls
	}
	return message, citations, nil
}

func anthropicCitationsFromRaw(raw json.RawMessage) []modality.Citation {
	if len(raw) == 0 {
		return nil
	}
	var parsed []struct {
		Type            string `json:"type"`
		CitedText       string `json:"cited_text"`
		DocumentTitle   string `json:"document_title"`
		DocumentIndex   int    `json:"document_index"`
		StartCharIndex  int    `json:"start_char_index"`
		EndCharIndex    int    `json:"end_char_index"`
		StartPageNumber int    `json:"start_page_number"`
		EndPageNumber   int    `json:"end_page_number"`
		StartBlockIndex int    `json:"start_block_index"`
		EndBlockIndex   int    `json:"end_block_index"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil
	}
	citations := make([]modality.Citation, 0, len(parsed))
	for _, item := range parsed {
		kind := "file"
		switch {
		case item.StartPageNumber > 0 || item.EndPageNumber > 0:
			kind = "page"
		case item.StartCharIndex > 0 || item.EndCharIndex > 0:
			kind = "block"
		}
		citations = append(citations, modality.Citation{
			Kind:      kind,
			Title:     item.DocumentTitle,
			CitedText: item.CitedText,
			Provider:  "anthropic-compatible",
			Locator: &modality.CitationLocator{
				StartChar: item.StartCharIndex,
				EndChar:   item.EndCharIndex,
				PageStart: item.StartPageNumber,
				PageEnd:   item.EndPageNumber,
				BlockIdx:  item.StartBlockIndex,
			},
			RawMeta: raw,
		})
	}
	return citations
}

func decodeStream(r io.Reader, canonicalModel string, dst chan<- modality.ChatChunk) error {
	reader := bufio.NewReader(r)
	var (
		eventType string
		dataLines []string
		state     anthropicStreamState
	)

	flush := func() (bool, error) {
		if eventType == "" && len(dataLines) == 0 {
			return false, nil
		}

		payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
		currentEvent := eventType
		eventType = ""
		dataLines = nil

		if currentEvent == "ping" || payload == "" {
			return false, nil
		}
		if currentEvent == "error" {
			var envelope struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			_ = json.Unmarshal([]byte(payload), &envelope)
			message := envelope.Error.Message
			if message == "" {
				message = "Anthropic-compatible streaming request failed."
			}
			return false, apierror.NewError(http.StatusBadGateway, "provider_error", "provider_stream_error", "", message)
		}

		chunks, done, err := state.consume(currentEvent, payload, canonicalModel)
		if err != nil {
			return false, err
		}
		for _, chunk := range chunks {
			dst <- chunk
		}
		return done, nil
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			if err == io.EOF {
				_, flushErr := flush()
				return flushErr
			}
			return fmt.Errorf("read anthropic-compatible stream: %w", err)
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
		} else if strings.HasPrefix(trimmed, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
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

type anthropicStreamState struct {
	id               string
	promptTokens     int
	completionTokens int
	stopReason       string
	roleSent         bool
	toolBlocks       map[int]anthropicToolBlockState
}

type anthropicToolBlockState struct {
	ID   string
	Name string
}

func (s *anthropicStreamState) consume(eventType string, payload string, model string) ([]modality.ChatChunk, bool, error) {
	if s.toolBlocks == nil {
		s.toolBlocks = map[int]anthropicToolBlockState{}
	}

	switch eventType {
	case "message_start":
		var event struct {
			Message struct {
				ID    string         `json:"id"`
				Usage anthropicUsage `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return nil, false, fmt.Errorf("decode anthropic-compatible message_start: %w", err)
		}
		s.id = event.Message.ID
		s.promptTokens = event.Message.Usage.InputTokens
		return nil, false, nil
	case "content_block_start":
		var event struct {
			Index        int                   `json:"index"`
			ContentBlock anthropicContentBlock `json:"content_block"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return nil, false, fmt.Errorf("decode anthropic-compatible content_block_start: %w", err)
		}

		var chunks []modality.ChatChunk
		if !s.roleSent {
			s.roleSent = true
			chunks = append(chunks, modality.ChatChunk{
				ID:      s.id,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   model,
				Choices: []modality.ChatChunkChoice{{
					Index: 0,
					Delta: modality.ChatDelta{Role: "assistant"},
				}},
			})
		}

		if event.ContentBlock.Type == "tool_use" {
			s.toolBlocks[event.Index] = anthropicToolBlockState{
				ID:   event.ContentBlock.ID,
				Name: event.ContentBlock.Name,
			}

			arguments := ""
			if len(event.ContentBlock.Input) > 0 {
				raw, err := json.Marshal(event.ContentBlock.Input)
				if err != nil {
					return nil, false, fmt.Errorf("marshal anthropic-compatible tool input: %w", err)
				}
				arguments = string(raw)
			}
			chunks = append(chunks, modality.ChatChunk{
				ID:      s.id,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   model,
				Choices: []modality.ChatChunkChoice{{
					Index: 0,
					Delta: modality.ChatDelta{
						ToolCalls: []modality.ToolCall{{
							ID:   event.ContentBlock.ID,
							Type: "function",
							Function: modality.ToolCallFunction{
								Name:      event.ContentBlock.Name,
								Arguments: arguments,
							},
						}},
					},
				}},
			})
		}
		return chunks, false, nil
	case "content_block_delta":
		var event struct {
			Index int `json:"index"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return nil, false, fmt.Errorf("decode anthropic-compatible content_block_delta: %w", err)
		}

		switch event.Delta.Type {
		case "text_delta":
			return []modality.ChatChunk{{
				ID:      s.id,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   model,
				Choices: []modality.ChatChunkChoice{{
					Index: 0,
					Delta: modality.ChatDelta{Content: event.Delta.Text},
				}},
			}}, false, nil
		case "input_json_delta":
			block := s.toolBlocks[event.Index]
			return []modality.ChatChunk{{
				ID:      s.id,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   model,
				Choices: []modality.ChatChunkChoice{{
					Index: 0,
					Delta: modality.ChatDelta{
						ToolCalls: []modality.ToolCall{{
							ID:   block.ID,
							Type: "function",
							Function: modality.ToolCallFunction{
								Name:      block.Name,
								Arguments: event.Delta.PartialJSON,
							},
						}},
					},
				}},
			}}, false, nil
		default:
			return nil, false, nil
		}
	case "message_delta":
		var event struct {
			Delta struct {
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage anthropicUsage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return nil, false, fmt.Errorf("decode anthropic-compatible message_delta: %w", err)
		}
		if event.Delta.StopReason != "" {
			s.stopReason = event.Delta.StopReason
		}
		if event.Usage.OutputTokens > 0 {
			s.completionTokens = event.Usage.OutputTokens
		}
		return nil, false, nil
	case "message_stop":
		finishReason := mapStopReason(s.stopReason)
		return []modality.ChatChunk{{
			ID:      s.id,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   model,
			Choices: []modality.ChatChunkChoice{{
				Index:        0,
				Delta:        modality.ChatDelta{},
				FinishReason: &finishReason,
			}},
			Usage: &modality.Usage{
				PromptTokens:     s.promptTokens,
				CompletionTokens: s.completionTokens,
				TotalTokens:      s.promptTokens + s.completionTokens,
			},
		}}, true, nil
	default:
		return nil, false, nil
	}
}

func mapStopReason(reason string) string {
	switch reason {
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	case "end_turn", "stop_sequence":
		return "stop"
	default:
		return "stop"
	}
}

func providerModelName(requestModel string, fallbackModel string) string {
	if requestModel != "" {
		if idx := strings.IndexByte(requestModel, '/'); idx >= 0 {
			return requestModel[idx+1:]
		}
		return requestModel
	}
	if idx := strings.IndexByte(fallbackModel, '/'); idx >= 0 {
		return fallbackModel[idx+1:]
	}
	return fallbackModel
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
