package anthropic

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
	client           *Client
	model            string
	defaultMaxTokens int
}

func NewChatAdapter(client *Client, model string, maxTokens int) *ChatAdapter {
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	return &ChatAdapter{
		client:           client,
		model:            model,
		defaultMaxTokens: maxTokens,
	}
}

func (a *ChatAdapter) Complete(ctx context.Context, req *modality.ChatRequest) (*modality.ChatResponse, error) {
	payload, err := a.translateRequest(req, false)
	if err != nil {
		return nil, err
	}

	var response anthropicMessagesResponse
	if err := a.client.JSON(ctx, "/v1/messages", payload, &response); err != nil {
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

	resp, err := a.client.Stream(ctx, "/v1/messages", payload)
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
	payload, err := a.translateRequest(req, false)
	if err != nil {
		return nil, err
	}

	countPayload := anthropicCountTokensRequest{
		Model:      payload.Model,
		Messages:   payload.Messages,
		System:     payload.System,
		Tools:      payload.Tools,
		ToolChoice: payload.ToolChoice,
	}

	var response anthropicCountTokensResponse
	if err := a.client.JSON(ctx, "/v1/messages/count_tokens", countPayload, &response); err != nil {
		return nil, err
	}

	return &modality.TokenCountResult{
		InputTokens: response.InputTokens,
		Source:      modality.TokenCountSourceProviderReported,
		Notes: []string{
			"input tokens were returned by Anthropic's native token counting endpoint",
			"output_tokens_estimate remains a Polaris estimate derived from max_tokens limits",
		},
	}, nil
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
	Metadata      map[string]string  `json:"metadata,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type      string                `json:"type"`
	Text      string                `json:"text,omitempty"`
	Source    *anthropicImageSource `json:"source,omitempty"`
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
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type anthropicMessagesResponse struct {
	ID         string                  `json:"id"`
	Model      string                  `json:"model"`
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthropicCountTokensRequest struct {
	Model      string             `json:"model"`
	Messages   []anthropicMessage `json:"messages"`
	System     string             `json:"system,omitempty"`
	Tools      []anthropicTool    `json:"tools,omitempty"`
	ToolChoice map[string]any     `json:"tool_choice,omitempty"`
}

type anthropicCountTokensResponse struct {
	InputTokens int `json:"input_tokens"`
}

func (a *ChatAdapter) translateRequest(req *modality.ChatRequest, stream bool) (anthropicMessagesRequest, error) {
	payload := anthropicMessagesRequest{
		Model:         providerModelName(req.Model, a.model),
		MaxTokens:     req.MaxTokens,
		Temperature:   req.Temperature,
		TopP:          req.TopP,
		Stream:        stream,
		StopSequences: append([]string(nil), req.Stop...),
	}
	if payload.MaxTokens <= 0 {
		payload.MaxTokens = a.defaultMaxTokens
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
		payload.Tools = append(payload.Tools, anthropicTool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: tool.Function.Parameters,
		})
	}
	if len(payload.Tools) > 0 && len(req.ToolChoice) > 0 {
		toolChoice, err := translateToolChoice(req.ToolChoice)
		if err != nil {
			return anthropicMessagesRequest{}, err
		}
		payload.ToolChoice = toolChoice
	}

	return payload, nil
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
		return anthropicMessage{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_role", "messages.role", "Unsupported message role.")
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
				return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image", "messages.content.image_url", "Image content must include image_url.")
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
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_audio_input", "messages.content.input_audio", "Anthropic chat does not support audio input in this build.")
		default:
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_content_part", "messages.content.type", "Unsupported content part type.")
		}
	}
	return blocks, nil
}

func translateImageSource(raw string) (*anthropicImageSource, error) {
	if strings.HasPrefix(raw, "data:") {
		header, data, ok := strings.Cut(strings.TrimPrefix(raw, "data:"), ",")
		if !ok {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image_data_uri", "messages.content.image_url.url", "Invalid image data URI.")
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
			return "", httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_system_content", "messages.content", "System and tool messages must use text content.")
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
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_tool_choice", "tool_choice", "Invalid tool_choice payload.")
	}
	if objectChoice.Type != "function" || objectChoice.Function.Name == "" {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_tool_choice", "tool_choice", "Unsupported tool_choice payload.")
	}
	return map[string]any{
		"type": "tool",
		"name": objectChoice.Function.Name,
	}, nil
}

func (a *ChatAdapter) translateResponse(response anthropicMessagesResponse, canonicalModel string) (*modality.ChatResponse, error) {
	message, err := responseContentToMessage(response.Content)
	if err != nil {
		return nil, err
	}

	usage := modality.Usage{
		PromptTokens:     response.Usage.InputTokens,
		CompletionTokens: response.Usage.OutputTokens,
		TotalTokens:      response.Usage.InputTokens + response.Usage.OutputTokens,
	}

	return &modality.ChatResponse{
		ID:      response.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   canonicalModel,
		Choices: []modality.ChatChoice{
			{
				Index:        0,
				Message:      message,
				FinishReason: mapStopReason(response.StopReason),
			},
		},
		Usage: usage,
	}, nil
}

func responseContentToMessage(content []anthropicContentBlock) (modality.ChatMessage, error) {
	var textParts []string
	var toolCalls []modality.ToolCall

	for _, block := range content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				textParts = append(textParts, block.Text)
			}
		case "tool_use":
			arguments := "{}"
			if len(block.Input) > 0 {
				raw, err := json.Marshal(block.Input)
				if err != nil {
					return modality.ChatMessage{}, fmt.Errorf("marshal anthropic tool input: %w", err)
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
	return message, nil
}

func (a *ChatAdapter) decodeStream(r io.Reader, canonicalModel string, dst chan<- modality.ChatChunk) error {
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
				message = "Anthropic streaming request failed."
			}
			return false, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_stream_error", "", message)
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
			return fmt.Errorf("read anthropic stream: %w", err)
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
				ID    string `json:"id"`
				Usage struct {
					InputTokens int `json:"input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return nil, false, fmt.Errorf("decode anthropic message_start: %w", err)
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
			return nil, false, fmt.Errorf("decode anthropic content_block_start: %w", err)
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
					return nil, false, fmt.Errorf("marshal anthropic tool input: %w", err)
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
			return nil, false, fmt.Errorf("decode anthropic content_block_delta: %w", err)
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
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return nil, false, fmt.Errorf("decode anthropic message_delta: %w", err)
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
