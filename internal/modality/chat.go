package modality

import (
	"bytes"
	"context"
	"encoding/json"
)

type ChatAdapter interface {
	Complete(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	Stream(ctx context.Context, req *ChatRequest) (<-chan ChatChunk, error)
}

type NativeJSONResponse struct {
	Payload json.RawMessage
	Model   string
	Usage   *Usage
}

type NativeJSONStreamEvent struct {
	Event   string
	Payload json.RawMessage
	Model   string
	Usage   *Usage
	Err     error
}

type NativeResponsesAdapter interface {
	CreateResponse(ctx context.Context, raw json.RawMessage, canonicalModel string) (*NativeJSONResponse, error)
	StreamResponse(ctx context.Context, raw json.RawMessage, canonicalModel string) (<-chan NativeJSONStreamEvent, error)
}

type NativeMessagesAdapter interface {
	CreateMessage(ctx context.Context, raw json.RawMessage, canonicalModel string) (*NativeJSONResponse, error)
	StreamMessage(ctx context.Context, raw json.RawMessage, canonicalModel string) (<-chan NativeJSONStreamEvent, error)
}

type ChatRequest struct {
	Model          string            `json:"model"`
	Routing        *RoutingOptions   `json:"routing,omitempty"`
	Messages       []ChatMessage     `json:"messages"`
	Temperature    *float64          `json:"temperature,omitempty"`
	TopP           *float64          `json:"top_p,omitempty"`
	MaxTokens      int               `json:"max_tokens,omitempty"`
	Stream         bool              `json:"stream,omitempty"`
	Tools          []ToolDefinition  `json:"tools,omitempty"`
	ToolChoice     json.RawMessage   `json:"tool_choice,omitempty"`
	ResponseFormat *ResponseFormat   `json:"response_format,omitempty"`
	Stop           []string          `json:"stop,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type ChatMessage struct {
	Role       string         `json:"role"`
	Content    MessageContent `json:"content"`
	Name       string         `json:"name,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall     `json:"tool_calls,omitempty"`
}

type MessageContent struct {
	Text  *string       `json:"-"`
	Parts []ContentPart `json:"-"`
}

func NewTextContent(text string) MessageContent {
	return MessageContent{Text: &text}
}

func NewPartContent(parts ...ContentPart) MessageContent {
	return MessageContent{Parts: parts}
}

func (c MessageContent) MarshalJSON() ([]byte, error) {
	if c.Text != nil {
		return json.Marshal(*c.Text)
	}
	return json.Marshal(c.Parts)
}

func (c *MessageContent) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	switch {
	case bytes.Equal(trimmed, []byte("null")):
		c.Text = nil
		c.Parts = nil
		return nil
	case len(trimmed) > 0 && trimmed[0] == '"':
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return err
		}
		c.Text = &text
		c.Parts = nil
		return nil
	case len(trimmed) > 0 && trimmed[0] == '[':
		var parts []ContentPart
		if err := json.Unmarshal(trimmed, &parts); err != nil {
			return err
		}
		c.Text = nil
		c.Parts = parts
		return nil
	default:
		return json.Unmarshal(trimmed, &c.Parts)
	}
}

type ContentPart struct {
	Type       string          `json:"type"`
	Text       string          `json:"text,omitempty"`
	ImageURL   *ImageURLPart   `json:"image_url,omitempty"`
	InputAudio *InputAudioPart `json:"input_audio,omitempty"`
}

type ImageURLPart struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type InputAudioPart struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

type ToolDefinition struct {
	Type     string             `json:"type"`
	Function FunctionDefinition `json:"function"`
}

type FunctionDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type ResponseFormat struct {
	Type       string             `json:"type"`
	JSONSchema *JSONSchemaWrapper `json:"json_schema,omitempty"`
}

type JSONSchemaWrapper struct {
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
	Strict      bool            `json:"strict,omitempty"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ChatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   Usage        `json:"usage"`
}

type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type ChatChunk struct {
	ID      string            `json:"id"`
	Object  string            `json:"object"`
	Created int64             `json:"created"`
	Model   string            `json:"model"`
	Choices []ChatChunkChoice `json:"choices"`
	Usage   *Usage            `json:"usage,omitempty"`
	Err     error             `json:"-"`
}

type ChatChunkChoice struct {
	Index        int       `json:"index"`
	Delta        ChatDelta `json:"delta"`
	FinishReason *string   `json:"finish_reason"`
}

type ChatDelta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type Usage struct {
	PromptTokens     int              `json:"prompt_tokens"`
	CompletionTokens int              `json:"completion_tokens"`
	TotalTokens      int              `json:"total_tokens"`
	Source           TokenCountSource `json:"source,omitempty"`
}
