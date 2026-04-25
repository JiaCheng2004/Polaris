package client

import (
	"bytes"
	"encoding/json"
	"io"
	"time"
)

type Usage struct {
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	Source           string `json:"source,omitempty"`
}

type RoutingOptions struct {
	Providers           []string `json:"providers,omitempty"`
	ExcludeProviders    []string `json:"exclude_providers,omitempty"`
	Capabilities        []string `json:"capabilities,omitempty"`
	Statuses            []string `json:"statuses,omitempty"`
	VerificationClasses []string `json:"verification_classes,omitempty"`
	Prefer              []string `json:"prefer,omitempty"`
	CostTier            string   `json:"cost_tier,omitempty"`
	LatencyTier         string   `json:"latency_tier,omitempty"`
}

type ChatCompletionRequest struct {
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
	return MessageContent{Parts: append([]ContentPart(nil), parts...)}
}

func (content MessageContent) MarshalJSON() ([]byte, error) {
	if content.Text != nil {
		return json.Marshal(*content.Text)
	}
	return json.Marshal(content.Parts)
}

func (content *MessageContent) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	switch {
	case bytes.Equal(trimmed, []byte("null")):
		content.Text = nil
		content.Parts = nil
		return nil
	case len(trimmed) > 0 && trimmed[0] == '"':
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return err
		}
		content.Text = &text
		content.Parts = nil
		return nil
	case len(trimmed) > 0 && trimmed[0] == '[':
		var parts []ContentPart
		if err := json.Unmarshal(trimmed, &parts); err != nil {
			return err
		}
		content.Text = nil
		content.Parts = parts
		return nil
	default:
		return json.Unmarshal(trimmed, &content.Parts)
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

type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   Usage                  `json:"usage"`
}

type ChatCompletionChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type ChatCompletionChunk struct {
	ID      string                      `json:"id"`
	Object  string                      `json:"object"`
	Created int64                       `json:"created"`
	Model   string                      `json:"model"`
	Choices []ChatCompletionChunkChoice `json:"choices"`
	Usage   *Usage                      `json:"usage,omitempty"`
}

type ChatCompletionChunkChoice struct {
	Index        int       `json:"index"`
	Delta        ChatDelta `json:"delta"`
	FinishReason *string   `json:"finish_reason"`
}

type ChatDelta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type ResponsesRequest struct {
	Model           string            `json:"model"`
	Routing         *RoutingOptions   `json:"routing,omitempty"`
	Input           json.RawMessage   `json:"input"`
	Instructions    string            `json:"instructions,omitempty"`
	Temperature     *float64          `json:"temperature,omitempty"`
	TopP            *float64          `json:"top_p,omitempty"`
	MaxOutputTokens int               `json:"max_output_tokens,omitempty"`
	Stream          bool              `json:"stream,omitempty"`
	Tools           []ToolDefinition  `json:"tools,omitempty"`
	ToolChoice      json.RawMessage   `json:"tool_choice,omitempty"`
	Text            *ResponsesText    `json:"text,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type ResponsesText struct {
	Format *ResponseFormat `json:"format,omitempty"`
}

type ResponsesResponse struct {
	ID         string                `json:"id"`
	Object     string                `json:"object"`
	CreatedAt  int64                 `json:"created_at"`
	Status     string                `json:"status"`
	Model      string                `json:"model"`
	Output     []ResponsesOutputItem `json:"output"`
	OutputText string                `json:"output_text,omitempty"`
	Usage      ResponsesUsage        `json:"usage"`
	Metadata   map[string]string     `json:"metadata,omitempty"`
}

type ResponsesOutputItem struct {
	ID        string                 `json:"id,omitempty"`
	Type      string                 `json:"type"`
	Role      string                 `json:"role,omitempty"`
	Content   []ResponsesContentItem `json:"content,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Arguments string                 `json:"arguments,omitempty"`
	CallID    string                 `json:"call_id,omitempty"`
}

type ResponsesContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type ResponsesUsage struct {
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	TotalTokens  int    `json:"total_tokens"`
	Source       string `json:"source,omitempty"`
}

type ResponsesStreamEvent struct {
	Type         string               `json:"type"`
	Response     *ResponsesResponse   `json:"response,omitempty"`
	Item         *ResponsesOutputItem `json:"item,omitempty"`
	ItemID       string               `json:"item_id,omitempty"`
	OutputIndex  int                  `json:"output_index,omitempty"`
	ContentIndex int                  `json:"content_index,omitempty"`
	Delta        string               `json:"delta,omitempty"`
	Text         string               `json:"text,omitempty"`
}

type MessagesRequest struct {
	Model         string                   `json:"model"`
	Routing       *RoutingOptions          `json:"routing,omitempty"`
	System        string                   `json:"system,omitempty"`
	Messages      []MessagesInputMessage   `json:"messages"`
	MaxTokens     int                      `json:"max_tokens,omitempty"`
	Temperature   *float64                 `json:"temperature,omitempty"`
	TopP          *float64                 `json:"top_p,omitempty"`
	Stream        bool                     `json:"stream,omitempty"`
	StopSequences []string                 `json:"stop_sequences,omitempty"`
	Tools         []MessagesToolDefinition `json:"tools,omitempty"`
	ToolChoice    json.RawMessage          `json:"tool_choice,omitempty"`
	Metadata      map[string]string        `json:"metadata,omitempty"`
}

type MessagesToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type MessagesInputMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type MessagesContentSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type MessagesContentBlock struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text,omitempty"`
	Source    *MessagesContentSource `json:"source,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     json.RawMessage        `json:"input,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	Content   json.RawMessage        `json:"content,omitempty"`
}

type MessagesResponse struct {
	ID           string                `json:"id"`
	Type         string                `json:"type"`
	Role         string                `json:"role"`
	Content      []MessagesOutputBlock `json:"content"`
	Model        string                `json:"model"`
	StopReason   string                `json:"stop_reason,omitempty"`
	StopSequence *string               `json:"stop_sequence,omitempty"`
	Usage        MessagesUsage         `json:"usage"`
}

type MessagesOutputBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type MessagesUsage struct {
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	Source       string `json:"source,omitempty"`
}

type MessagesStreamEvent struct {
	Type         string               `json:"type"`
	Message      *MessagesResponse    `json:"message,omitempty"`
	Index        int                  `json:"index,omitempty"`
	ContentBlock *MessagesOutputBlock `json:"content_block,omitempty"`
	Delta        json.RawMessage      `json:"delta,omitempty"`
	Usage        *MessagesUsage       `json:"usage,omitempty"`
}

type TokenCountRequest struct {
	Model              string          `json:"model"`
	Routing            *RoutingOptions `json:"routing,omitempty"`
	Messages           []ChatMessage   `json:"messages,omitempty"`
	Input              json.RawMessage `json:"input,omitempty"`
	ToolContext        json.RawMessage `json:"tool_context,omitempty"`
	RequestedInterface string          `json:"requested_interface,omitempty"`
	MaxOutputTokens    int             `json:"max_output_tokens,omitempty"`
}

type TokenCountResponse struct {
	Model                string   `json:"model"`
	InputTokens          int      `json:"input_tokens"`
	OutputTokensEstimate int      `json:"output_tokens_estimate"`
	Source               string   `json:"source"`
	Notes                []string `json:"notes,omitempty"`
}

type TranslationRequest struct {
	Model          string            `json:"model"`
	Routing        *RoutingOptions   `json:"routing,omitempty"`
	Input          TranslationInput  `json:"input"`
	TargetLanguage string            `json:"target_language"`
	SourceLanguage string            `json:"source_language,omitempty"`
	Glossary       map[string]string `json:"glossary,omitempty"`
}

type TranslationInput struct {
	Single *string
	Many   []string
}

func NewSingleTranslationInput(value string) TranslationInput {
	return TranslationInput{Single: &value}
}

func NewMultiTranslationInput(values ...string) TranslationInput {
	return TranslationInput{Many: append([]string(nil), values...)}
}

func (input TranslationInput) Values() []string {
	if input.Single != nil {
		return []string{*input.Single}
	}
	return append([]string(nil), input.Many...)
}

func (input TranslationInput) MarshalJSON() ([]byte, error) {
	if input.Single != nil {
		return json.Marshal(*input.Single)
	}
	return json.Marshal(input.Many)
}

func (input *TranslationInput) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	switch {
	case bytes.Equal(trimmed, []byte("null")):
		input.Single = nil
		input.Many = nil
		return nil
	case len(trimmed) > 0 && trimmed[0] == '"':
		var value string
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return err
		}
		input.Single = &value
		input.Many = nil
		return nil
	case len(trimmed) > 0 && trimmed[0] == '[':
		var values []string
		if err := json.Unmarshal(trimmed, &values); err != nil {
			return err
		}
		input.Single = nil
		input.Many = values
		return nil
	default:
		return json.Unmarshal(trimmed, &input.Many)
	}
}

type TranslationResponse struct {
	Object       string              `json:"object"`
	Model        string              `json:"model"`
	Translations []TranslationResult `json:"translations"`
	Usage        Usage               `json:"usage"`
}

type TranslationResult struct {
	Index                  int    `json:"index"`
	Text                   string `json:"text"`
	DetectedSourceLanguage string `json:"detected_source_language,omitempty"`
}

type EmbeddingRequest struct {
	Model          string          `json:"model"`
	Routing        *RoutingOptions `json:"routing,omitempty"`
	Input          EmbeddingInput  `json:"input"`
	Dimensions     *int            `json:"dimensions,omitempty"`
	EncodingFormat string          `json:"encoding_format,omitempty"`
	User           string          `json:"user,omitempty"`
}

type EmbeddingInput struct {
	Single *string
	Many   []string
}

func NewSingleEmbeddingInput(value string) EmbeddingInput {
	return EmbeddingInput{Single: &value}
}

func NewMultiEmbeddingInput(values ...string) EmbeddingInput {
	return EmbeddingInput{Many: append([]string(nil), values...)}
}

func (input EmbeddingInput) Values() []string {
	if input.Single != nil {
		return []string{*input.Single}
	}
	return append([]string(nil), input.Many...)
}

func (input EmbeddingInput) MarshalJSON() ([]byte, error) {
	if input.Single != nil {
		return json.Marshal(*input.Single)
	}
	return json.Marshal(input.Many)
}

func (input *EmbeddingInput) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	switch {
	case bytes.Equal(trimmed, []byte("null")):
		input.Single = nil
		input.Many = nil
		return nil
	case len(trimmed) > 0 && trimmed[0] == '"':
		var value string
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return err
		}
		input.Single = &value
		input.Many = nil
		return nil
	case len(trimmed) > 0 && trimmed[0] == '[':
		var values []string
		if err := json.Unmarshal(trimmed, &values); err != nil {
			return err
		}
		input.Single = nil
		input.Many = values
		return nil
	default:
		return json.Unmarshal(trimmed, &input.Many)
	}
}

type EmbeddingResponse struct {
	Object string          `json:"object"`
	Data   []EmbeddingData `json:"data"`
	Model  string          `json:"model"`
	Usage  EmbeddingUsage  `json:"usage"`
}

type EmbeddingData struct {
	Object    string          `json:"object"`
	Index     int             `json:"index"`
	Embedding EmbeddingValues `json:"embedding"`
}

type EmbeddingValues struct {
	Float32 []float32
	Base64  string
}

func (values EmbeddingValues) MarshalJSON() ([]byte, error) {
	if values.Base64 != "" {
		return json.Marshal(values.Base64)
	}
	if values.Float32 == nil {
		return json.Marshal([]float32{})
	}
	return json.Marshal(values.Float32)
}

func (values *EmbeddingValues) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	switch {
	case bytes.Equal(trimmed, []byte("null")):
		values.Float32 = nil
		values.Base64 = ""
		return nil
	case len(trimmed) > 0 && trimmed[0] == '"':
		var value string
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return err
		}
		values.Base64 = value
		values.Float32 = nil
		return nil
	default:
		var items []float32
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return err
		}
		values.Float32 = items
		values.Base64 = ""
		return nil
	}
}

type EmbeddingUsage struct {
	PromptTokens int    `json:"prompt_tokens"`
	TotalTokens  int    `json:"total_tokens"`
	Source       string `json:"source,omitempty"`
}

type ImageGenerationRequest struct {
	Model           string          `json:"model"`
	Routing         *RoutingOptions `json:"routing,omitempty"`
	Prompt          string          `json:"prompt"`
	N               int             `json:"n,omitempty"`
	Size            string          `json:"size,omitempty"`
	Quality         string          `json:"quality,omitempty"`
	Style           string          `json:"style,omitempty"`
	ResponseFormat  string          `json:"response_format,omitempty"`
	ReferenceImages []string        `json:"reference_images,omitempty"`
}

type ImageEditRequest struct {
	Model            string
	Routing          *RoutingOptions
	Prompt           string
	Image            []byte
	ImageFilename    string
	ImageContentType string
	Mask             []byte
	MaskFilename     string
	MaskContentType  string
	N                int
	Size             string
	ResponseFormat   string
}

type ImageResponse struct {
	Created int64         `json:"created"`
	Data    []ImageResult `json:"data"`
}

type ImageResult struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

type VideoGenerationRequest struct {
	Model           string          `json:"model"`
	Routing         *RoutingOptions `json:"routing,omitempty"`
	Prompt          string          `json:"prompt"`
	Duration        int             `json:"duration,omitempty"`
	AspectRatio     string          `json:"aspect_ratio,omitempty"`
	Resolution      string          `json:"resolution,omitempty"`
	FirstFrame      string          `json:"first_frame,omitempty"`
	ReferenceImages []string        `json:"reference_images,omitempty"`
	WithAudio       bool            `json:"with_audio,omitempty"`

	LastFrame       string   `json:"last_frame,omitempty"`
	ReferenceVideos []string `json:"reference_videos,omitempty"`
	Audio           string   `json:"audio,omitempty"`
}

type VideoJob struct {
	JobID         string `json:"job_id"`
	Status        string `json:"status"`
	EstimatedTime int    `json:"estimated_time,omitempty"`
	Model         string `json:"model,omitempty"`
}

type VideoStatus struct {
	JobID       string       `json:"job_id"`
	Status      string       `json:"status"`
	Progress    float64      `json:"progress,omitempty"`
	Result      *VideoResult `json:"result,omitempty"`
	Error       *VideoError  `json:"error,omitempty"`
	CreatedAt   int64        `json:"created_at,omitempty"`
	CompletedAt int64        `json:"completed_at,omitempty"`
	ExpiresAt   int64        `json:"expires_at,omitempty"`
}

type VideoResult struct {
	VideoURL    string `json:"video_url,omitempty"`
	AudioURL    string `json:"audio_url,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Duration    int    `json:"duration,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

type VideoError struct {
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type SpeechRequest struct {
	Model          string          `json:"model"`
	Routing        *RoutingOptions `json:"routing,omitempty"`
	Input          string          `json:"input"`
	Voice          string          `json:"voice"`
	ResponseFormat string          `json:"response_format,omitempty"`
	Speed          *float64        `json:"speed,omitempty"`
}

type Audio struct {
	Data        []byte
	ContentType string
}

type VideoAsset struct {
	Data        []byte
	ContentType string
}

type MusicGenerationRequest struct {
	Mode            string          `json:"mode,omitempty"`
	Model           string          `json:"model"`
	Routing         *RoutingOptions `json:"routing,omitempty"`
	Prompt          string          `json:"prompt,omitempty"`
	Lyrics          string          `json:"lyrics,omitempty"`
	Plan            json.RawMessage `json:"plan,omitempty"`
	DurationMS      int             `json:"duration_ms,omitempty"`
	Instrumental    bool            `json:"instrumental,omitempty"`
	Seed            *int            `json:"seed,omitempty"`
	OutputFormat    string          `json:"output_format,omitempty"`
	SampleRateHz    int             `json:"sample_rate_hz,omitempty"`
	Bitrate         int             `json:"bitrate,omitempty"`
	StoreForEditing bool            `json:"store_for_editing,omitempty"`
	SignWithC2PA    bool            `json:"sign_with_c2pa,omitempty"`
}

type MusicEditRequest struct {
	Mode            string          `json:"mode,omitempty"`
	Model           string          `json:"model"`
	Routing         *RoutingOptions `json:"routing,omitempty"`
	Operation       string          `json:"operation"`
	Prompt          string          `json:"prompt,omitempty"`
	Lyrics          string          `json:"lyrics,omitempty"`
	Plan            json.RawMessage `json:"plan,omitempty"`
	SourceJobID     string          `json:"source_job_id,omitempty"`
	SourceAudio     string          `json:"source_audio,omitempty"`
	File            []byte          `json:"-"`
	Filename        string          `json:"-"`
	ContentType     string          `json:"-"`
	DurationMS      int             `json:"duration_ms,omitempty"`
	Instrumental    bool            `json:"instrumental,omitempty"`
	Seed            *int            `json:"seed,omitempty"`
	OutputFormat    string          `json:"output_format,omitempty"`
	SampleRateHz    int             `json:"sample_rate_hz,omitempty"`
	Bitrate         int             `json:"bitrate,omitempty"`
	StoreForEditing bool            `json:"store_for_editing,omitempty"`
	SignWithC2PA    bool            `json:"sign_with_c2pa,omitempty"`
}

type MusicStemRequest struct {
	Mode         string          `json:"mode,omitempty"`
	Model        string          `json:"model"`
	Routing      *RoutingOptions `json:"routing,omitempty"`
	SourceJobID  string          `json:"source_job_id,omitempty"`
	SourceAudio  string          `json:"source_audio,omitempty"`
	File         []byte          `json:"-"`
	Filename     string          `json:"-"`
	ContentType  string          `json:"-"`
	StemVariant  string          `json:"stem_variant,omitempty"`
	OutputFormat string          `json:"output_format,omitempty"`
	SignWithC2PA bool            `json:"sign_with_c2pa,omitempty"`
}

type MusicLyricsRequest struct {
	Model   string          `json:"model"`
	Routing *RoutingOptions `json:"routing,omitempty"`
	Mode    string          `json:"mode,omitempty"`
	Prompt  string          `json:"prompt,omitempty"`
	Lyrics  string          `json:"lyrics,omitempty"`
	Title   string          `json:"title,omitempty"`
}

type MusicPlanRequest struct {
	Model      string          `json:"model"`
	Routing    *RoutingOptions `json:"routing,omitempty"`
	Prompt     string          `json:"prompt"`
	DurationMS int             `json:"duration_ms,omitempty"`
	SourcePlan json.RawMessage `json:"source_plan,omitempty"`
}

type MusicJob struct {
	JobID     string `json:"job_id"`
	Status    string `json:"status"`
	Model     string `json:"model,omitempty"`
	Operation string `json:"operation,omitempty"`
}

type MusicStatus struct {
	JobID       string       `json:"job_id"`
	Status      string       `json:"status"`
	Model       string       `json:"model,omitempty"`
	Operation   string       `json:"operation,omitempty"`
	Progress    float64      `json:"progress,omitempty"`
	Result      *MusicResult `json:"result,omitempty"`
	Error       *MusicError  `json:"error,omitempty"`
	CreatedAt   int64        `json:"created_at,omitempty"`
	CompletedAt int64        `json:"completed_at,omitempty"`
	ExpiresAt   int64        `json:"expires_at,omitempty"`
}

type MusicResult struct {
	SongID       string          `json:"song_id,omitempty"`
	DownloadURL  string          `json:"download_url,omitempty"`
	ContentType  string          `json:"content_type,omitempty"`
	Filename     string          `json:"filename,omitempty"`
	DurationMS   int             `json:"duration_ms,omitempty"`
	SampleRateHz int             `json:"sample_rate_hz,omitempty"`
	Bitrate      int             `json:"bitrate,omitempty"`
	SizeBytes    int             `json:"size_bytes,omitempty"`
	Lyrics       string          `json:"lyrics,omitempty"`
	Plan         json.RawMessage `json:"plan,omitempty"`
}

type MusicError struct {
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type MusicLyricsResponse struct {
	Title     string `json:"title,omitempty"`
	StyleTags string `json:"style_tags,omitempty"`
	Lyrics    string `json:"lyrics,omitempty"`
}

type MusicPlanResponse struct {
	Plan json.RawMessage `json:"plan"`
}

type MusicAsset struct {
	Data        []byte
	ContentType string
}

type MusicStream struct {
	Body        io.ReadCloser
	ContentType string
}

type MusicOperationResponse struct {
	Job   *MusicJob
	Asset *MusicAsset
}

type AudioSessionRequest struct {
	Model             string              `json:"model"`
	Routing           *RoutingOptions     `json:"routing,omitempty"`
	Voice             string              `json:"voice,omitempty"`
	Instructions      string              `json:"instructions,omitempty"`
	InputAudioFormat  string              `json:"input_audio_format,omitempty"`
	OutputAudioFormat string              `json:"output_audio_format,omitempty"`
	SampleRateHz      int                 `json:"sample_rate_hz,omitempty"`
	TurnDetection     *AudioTurnDetection `json:"turn_detection,omitempty"`
}

type AudioTurnDetection struct {
	Mode            string `json:"mode"`
	SilenceMS       int    `json:"silence_ms,omitempty"`
	PrefixPaddingMS int    `json:"prefix_padding_ms,omitempty"`
}

type AudioSession struct {
	ID           string `json:"id"`
	Object       string `json:"object"`
	Model        string `json:"model"`
	ExpiresAt    int64  `json:"expires_at"`
	WebSocketURL string `json:"websocket_url"`
	ClientSecret string `json:"client_secret"`
}

type AudioEventResponse struct {
	Voice        string `json:"voice,omitempty"`
	Instructions string `json:"instructions,omitempty"`
}

type AudioClientEvent struct {
	Type       string               `json:"type"`
	EventID    string               `json:"event_id,omitempty"`
	Session    *AudioSessionRequest `json:"session,omitempty"`
	Audio      string               `json:"audio,omitempty"`
	Text       string               `json:"text,omitempty"`
	ResponseID string               `json:"response_id,omitempty"`
	Response   *AudioEventResponse  `json:"response,omitempty"`
}

type AudioServerEvent struct {
	Type       string         `json:"type"`
	EventID    string         `json:"event_id,omitempty"`
	Session    *AudioSession  `json:"session,omitempty"`
	ResponseID string         `json:"response_id,omitempty"`
	Audio      string         `json:"audio,omitempty"`
	Text       string         `json:"text,omitempty"`
	Transcript string         `json:"transcript,omitempty"`
	Usage      *AudioUsage    `json:"usage,omitempty"`
	Error      *AudioWSSError `json:"error,omitempty"`
}

type AudioUsage struct {
	InputAudioSeconds  float64 `json:"input_audio_seconds,omitempty"`
	OutputAudioSeconds float64 `json:"output_audio_seconds,omitempty"`
	InputTextTokens    int     `json:"input_text_tokens,omitempty"`
	OutputTextTokens   int     `json:"output_text_tokens,omitempty"`
	TotalTokens        int     `json:"total_tokens,omitempty"`
	Source             string  `json:"source,omitempty"`
}

type AudioWSSError struct {
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Param   string `json:"param,omitempty"`
}

type StreamingTranscriptionSessionRequest struct {
	Model            string          `json:"model"`
	Routing          *RoutingOptions `json:"routing,omitempty"`
	InputAudioFormat string          `json:"input_audio_format,omitempty"`
	SampleRateHz     int             `json:"sample_rate_hz,omitempty"`
	Language         string          `json:"language,omitempty"`
	InterimResults   *bool           `json:"interim_results,omitempty"`
	ReturnUtterances *bool           `json:"return_utterances,omitempty"`
}

type StreamingTranscriptionSession struct {
	ID           string `json:"id"`
	Object       string `json:"object"`
	Model        string `json:"model"`
	ExpiresAt    int64  `json:"expires_at"`
	WebSocketURL string `json:"websocket_url"`
	ClientSecret string `json:"client_secret"`
}

type StreamingTranscriptionClientEvent struct {
	Type    string                                `json:"type"`
	EventID string                                `json:"event_id,omitempty"`
	Session *StreamingTranscriptionSessionRequest `json:"session,omitempty"`
	Audio   string                                `json:"audio,omitempty"`
}

type StreamingTranscriptionEvent struct {
	Type       string                         `json:"type"`
	EventID    string                         `json:"event_id,omitempty"`
	Session    *StreamingTranscriptionSession `json:"session,omitempty"`
	Text       string                         `json:"text,omitempty"`
	Segment    *TranscriptSegment             `json:"segment,omitempty"`
	Transcript *TranscriptionResponse         `json:"transcript,omitempty"`
	Error      *AudioWSSError                 `json:"error,omitempty"`
}

type InterpretingGlossaryEntry struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type InterpretingSessionRequest struct {
	Model              string                      `json:"model"`
	Routing            *RoutingOptions             `json:"routing,omitempty"`
	Mode               string                      `json:"mode,omitempty"`
	SourceLanguage     string                      `json:"source_language,omitempty"`
	TargetLanguage     string                      `json:"target_language,omitempty"`
	Voice              string                      `json:"voice,omitempty"`
	InputAudioFormat   string                      `json:"input_audio_format,omitempty"`
	OutputAudioFormat  string                      `json:"output_audio_format,omitempty"`
	InputSampleRateHz  int                         `json:"input_sample_rate_hz,omitempty"`
	OutputSampleRateHz int                         `json:"output_sample_rate_hz,omitempty"`
	Denoise            *bool                       `json:"denoise,omitempty"`
	Glossary           []InterpretingGlossaryEntry `json:"glossary,omitempty"`
}

type InterpretingSession struct {
	ID           string `json:"id"`
	Object       string `json:"object"`
	Model        string `json:"model"`
	ExpiresAt    int64  `json:"expires_at"`
	WebSocketURL string `json:"websocket_url"`
	ClientSecret string `json:"client_secret"`
}

type InterpretingClientEvent struct {
	Type    string                      `json:"type"`
	EventID string                      `json:"event_id,omitempty"`
	Session *InterpretingSessionRequest `json:"session,omitempty"`
	Audio   string                      `json:"audio,omitempty"`
}

type InterpretingUsage struct {
	InputAudioSeconds  float64 `json:"input_audio_seconds,omitempty"`
	OutputAudioSeconds float64 `json:"output_audio_seconds,omitempty"`
	TotalTokens        int     `json:"total_tokens,omitempty"`
	Source             string  `json:"source,omitempty"`
}

type InterpretingEvent struct {
	Type       string               `json:"type"`
	EventID    string               `json:"event_id,omitempty"`
	Session    *InterpretingSession `json:"session,omitempty"`
	ResponseID string               `json:"response_id,omitempty"`
	Text       string               `json:"text,omitempty"`
	Audio      string               `json:"audio,omitempty"`
	Segment    *TranscriptSegment   `json:"segment,omitempty"`
	Usage      *InterpretingUsage   `json:"usage,omitempty"`
	Error      *AudioWSSError       `json:"error,omitempty"`
}

type TranscriptionRequest struct {
	Model          string
	Routing        *RoutingOptions
	File           []byte
	Filename       string
	ContentType    string
	Language       string
	ResponseFormat string
	Temperature    *float64
}

type TranscriptionResponse struct {
	Text        string              `json:"text,omitempty"`
	Language    string              `json:"language,omitempty"`
	Duration    float64             `json:"duration,omitempty"`
	Segments    []TranscriptSegment `json:"segments,omitempty"`
	Raw         []byte              `json:"-"`
	ContentType string              `json:"-"`
	Format      string              `json:"-"`
}

type TranscriptSegment struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

type Model struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Kind              string   `json:"kind,omitempty"`
	Provider          string   `json:"provider,omitempty"`
	ProviderVariant   string   `json:"provider_variant,omitempty"`
	DisplayName       string   `json:"display_name,omitempty"`
	FamilyID          string   `json:"family_id,omitempty"`
	FamilyDisplayName string   `json:"family_display_name,omitempty"`
	Status            string   `json:"status,omitempty"`
	VerificationClass string   `json:"verification_class,omitempty"`
	CostTier          string   `json:"cost_tier,omitempty"`
	LatencyTier       string   `json:"latency_tier,omitempty"`
	DocURL            string   `json:"doc_url,omitempty"`
	LastVerified      string   `json:"last_verified,omitempty"`
	Modality          string   `json:"modality"`
	Capabilities      []string `json:"capabilities,omitempty"`
	ContextWindow     int      `json:"context_window,omitempty"`
	MaxOutputTokens   int      `json:"max_output_tokens,omitempty"`
	MaxDuration       int      `json:"max_duration,omitempty"`
	AllowedDurations  []int    `json:"allowed_durations,omitempty"`
	AspectRatios      []string `json:"aspect_ratios,omitempty"`
	Resolutions       []string `json:"resolutions,omitempty"`
	Cancelable        bool     `json:"cancelable,omitempty"`
	Voices            []string `json:"voices,omitempty"`
	Formats           []string `json:"formats,omitempty"`
	OutputFormats     []string `json:"output_formats,omitempty"`
	MinDurationMs     int      `json:"min_duration_ms,omitempty"`
	MaxDurationMs     int      `json:"max_duration_ms,omitempty"`
	SampleRatesHz     []int    `json:"sample_rates_hz,omitempty"`
	Dimensions        int      `json:"dimensions,omitempty"`
	SessionTTL        int64    `json:"session_ttl,omitempty"`
	ResolvesTo        string   `json:"resolves_to,omitempty"`
}

type ModelList struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

type VoiceListRequest struct {
	Provider        string
	Model           string
	Scope           string
	Type            string
	State           string
	Limit           int
	IncludeArchived bool
}

type VoiceList struct {
	Object   string      `json:"object"`
	Scope    string      `json:"scope"`
	Provider string      `json:"provider,omitempty"`
	Data     []VoiceItem `json:"data"`
}

type VoiceItem struct {
	ID          string         `json:"id"`
	Provider    string         `json:"provider,omitempty"`
	Type        string         `json:"type,omitempty"`
	Name        string         `json:"name,omitempty"`
	Gender      string         `json:"gender,omitempty"`
	Age         string         `json:"age,omitempty"`
	State       string         `json:"state,omitempty"`
	Models      []string       `json:"models,omitempty"`
	Categories  []string       `json:"categories,omitempty"`
	Emotions    []VoiceStyle   `json:"emotions,omitempty"`
	PreviewURL  string         `json:"preview_url,omitempty"`
	PreviewText string         `json:"preview_text,omitempty"`
	Error       string         `json:"error,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type VoiceStyle struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	PreviewURL  string `json:"preview_url,omitempty"`
	PreviewText string `json:"preview_text,omitempty"`
}

type VoiceCloneRequest struct {
	Model                  string          `json:"model"`
	Routing                *RoutingOptions `json:"routing,omitempty"`
	VoiceID                string          `json:"voice_id,omitempty"`
	Audio                  string          `json:"audio"`
	AudioFormat            string          `json:"audio_format,omitempty"`
	Language               string          `json:"language,omitempty"`
	PromptText             string          `json:"prompt_text,omitempty"`
	PreviewText            string          `json:"preview_text,omitempty"`
	Denoise                *bool           `json:"denoise,omitempty"`
	CheckPromptTextQuality *bool           `json:"check_prompt_text_quality,omitempty"`
	CheckAudioQuality      *bool           `json:"check_audio_quality,omitempty"`
	EnableSourceSeparation *bool           `json:"enable_source_separation,omitempty"`
	DenoiseModel           string          `json:"denoise_model,omitempty"`
}

type VoiceDesignRequest struct {
	Model          string          `json:"model"`
	Routing        *RoutingOptions `json:"routing,omitempty"`
	VoiceID        string          `json:"voice_id,omitempty"`
	Text           string          `json:"text"`
	PromptText     string          `json:"prompt_text,omitempty"`
	PromptImageURL string          `json:"prompt_image_url,omitempty"`
}

type VoiceActivationRequest struct {
	Model    string `json:"model"`
	Provider string `json:"provider,omitempty"`
}

type AudioNoteRequest struct {
	Model              string          `json:"model"`
	Routing            *RoutingOptions `json:"routing,omitempty"`
	SourceURL          string          `json:"source_url"`
	FileType           string          `json:"file_type,omitempty"`
	Language           string          `json:"language,omitempty"`
	IncludeSummary     bool            `json:"include_summary,omitempty"`
	IncludeChapters    bool            `json:"include_chapters,omitempty"`
	IncludeActionItems bool            `json:"include_action_items,omitempty"`
	IncludeQAPairs     bool            `json:"include_qa_pairs,omitempty"`
	TargetLanguage     string          `json:"target_language,omitempty"`
}

type AudioNoteJob struct {
	ID     string           `json:"id"`
	Object string           `json:"object"`
	Model  string           `json:"model"`
	Status string           `json:"status"`
	Result *AudioNoteResult `json:"result,omitempty"`
	Error  *AudioWSSError   `json:"error,omitempty"`
}

type AudioNoteResult struct {
	Transcript  string                `json:"transcript,omitempty"`
	Summary     string                `json:"summary,omitempty"`
	Chapters    []AudioNoteChapter    `json:"chapters,omitempty"`
	ActionItems []AudioNoteActionItem `json:"action_items,omitempty"`
	QAPairs     []AudioNoteQAPair     `json:"qa_pairs,omitempty"`
	Translation string                `json:"translation,omitempty"`
	Metadata    map[string]any        `json:"metadata,omitempty"`
}

type AudioNoteChapter struct {
	Title string  `json:"title,omitempty"`
	Start float64 `json:"start,omitempty"`
	End   float64 `json:"end,omitempty"`
	Text  string  `json:"text,omitempty"`
}

type AudioNoteActionItem struct {
	Content   string   `json:"content,omitempty"`
	Executor  []string `json:"executor,omitempty"`
	Due       []string `json:"due,omitempty"`
	StartTime float64  `json:"start_time,omitempty"`
}

type AudioNoteQAPair struct {
	Question string `json:"question,omitempty"`
	Answer   string `json:"answer,omitempty"`
}

type PodcastRequest struct {
	Model        string           `json:"model"`
	Routing      *RoutingOptions  `json:"routing,omitempty"`
	Segments     []PodcastSegment `json:"segments"`
	OutputFormat string           `json:"output_format,omitempty"`
	SampleRateHz int              `json:"sample_rate_hz,omitempty"`
	UseHeadMusic *bool            `json:"use_head_music,omitempty"`
}

type PodcastSegment struct {
	Speaker string `json:"speaker"`
	Voice   string `json:"voice,omitempty"`
	Text    string `json:"text"`
}

type PodcastJob struct {
	ID     string `json:"id"`
	Object string `json:"object"`
	Model  string `json:"model"`
	Status string `json:"status"`
}

type PodcastStatus struct {
	ID     string         `json:"id"`
	Object string         `json:"object"`
	Model  string         `json:"model"`
	Status string         `json:"status"`
	Result *PodcastResult `json:"result,omitempty"`
	Error  *AudioWSSError `json:"error,omitempty"`
}

type PodcastResult struct {
	ContentType string         `json:"content_type,omitempty"`
	Usage       Usage          `json:"usage,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type PodcastAsset struct {
	Data        []byte
	ContentType string
}

type UsageParams struct {
	From     *time.Time
	To       *time.Time
	Model    string
	Modality string
	GroupBy  string
}

type DailyUsage struct {
	Date     string  `json:"date"`
	Requests int64   `json:"requests"`
	Tokens   int64   `json:"tokens"`
	CostUSD  float64 `json:"cost_usd"`
}

type ModelUsage struct {
	Model    string  `json:"model"`
	Requests int64   `json:"requests"`
	Tokens   int64   `json:"tokens"`
	CostUSD  float64 `json:"cost_usd"`
}

type UsageReport struct {
	From          string       `json:"from"`
	To            string       `json:"to"`
	TotalRequests int64        `json:"total_requests"`
	TotalTokens   int64        `json:"total_tokens"`
	TotalCostUSD  float64      `json:"total_cost_usd"`
	ByDay         []DailyUsage `json:"by_day,omitempty"`
	ByModel       []ModelUsage `json:"by_model,omitempty"`
}

type CreateKeyRequest struct {
	Name          string   `json:"name"`
	OwnerID       string   `json:"owner_id,omitempty"`
	RateLimit     string   `json:"rate_limit,omitempty"`
	AllowedModels []string `json:"allowed_models,omitempty"`
	IsAdmin       bool     `json:"is_admin,omitempty"`
	ExpiresAt     string   `json:"expires_at,omitempty"`
}

type ListKeysParams struct {
	OwnerID        string
	IncludeRevoked *bool
}

type APIKey struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Key           string     `json:"key,omitempty"`
	KeyPrefix     string     `json:"key_prefix"`
	OwnerID       string     `json:"owner_id,omitempty"`
	RateLimit     string     `json:"rate_limit,omitempty"`
	AllowedModels []string   `json:"allowed_models"`
	IsAdmin       bool       `json:"is_admin"`
	CreatedAt     time.Time  `json:"created_at"`
	LastUsedAt    *time.Time `json:"last_used_at"`
	ExpiresAt     *time.Time `json:"expires_at"`
	IsRevoked     bool       `json:"is_revoked,omitempty"`
}

type APIKeyList struct {
	Object string   `json:"object"`
	Data   []APIKey `json:"data"`
}

type Project struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
}

type ProjectList struct {
	Object string    `json:"object"`
	Data   []Project `json:"data"`
}

type CreateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type CreateVirtualKeyRequest struct {
	ProjectID         string   `json:"project_id"`
	Name              string   `json:"name"`
	RateLimit         string   `json:"rate_limit,omitempty"`
	AllowedModels     []string `json:"allowed_models,omitempty"`
	AllowedModalities []string `json:"allowed_modalities,omitempty"`
	AllowedToolsets   []string `json:"allowed_toolsets,omitempty"`
	AllowedMCP        []string `json:"allowed_mcp_bindings,omitempty"`
	IsAdmin           bool     `json:"is_admin,omitempty"`
	ExpiresAt         string   `json:"expires_at,omitempty"`
}

type ListVirtualKeysParams struct {
	ProjectID      string
	IncludeRevoked *bool
}

type VirtualKey struct {
	ID                string     `json:"id"`
	ProjectID         string     `json:"project_id,omitempty"`
	Name              string     `json:"name"`
	Key               string     `json:"key,omitempty"`
	KeyPrefix         string     `json:"key_prefix"`
	RateLimit         string     `json:"rate_limit,omitempty"`
	AllowedModels     []string   `json:"allowed_models"`
	AllowedModalities []string   `json:"allowed_modalities,omitempty"`
	AllowedToolsets   []string   `json:"allowed_toolsets,omitempty"`
	AllowedMCP        []string   `json:"allowed_mcp_bindings,omitempty"`
	IsAdmin           bool       `json:"is_admin"`
	CreatedAt         time.Time  `json:"created_at"`
	LastUsedAt        *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	IsRevoked         bool       `json:"is_revoked,omitempty"`
}

type VirtualKeyList struct {
	Object string       `json:"object"`
	Data   []VirtualKey `json:"data"`
}

type CreatePolicyRequest struct {
	ProjectID         string   `json:"project_id"`
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	AllowedModels     []string `json:"allowed_models,omitempty"`
	AllowedModalities []string `json:"allowed_modalities,omitempty"`
	AllowedToolsets   []string `json:"allowed_toolsets,omitempty"`
	AllowedMCP        []string `json:"allowed_mcp_bindings,omitempty"`
}

type Policy struct {
	ID                string    `json:"id"`
	ProjectID         string    `json:"project_id"`
	Name              string    `json:"name"`
	Description       string    `json:"description,omitempty"`
	AllowedModels     []string  `json:"allowed_models,omitempty"`
	AllowedModalities []string  `json:"allowed_modalities,omitempty"`
	AllowedToolsets   []string  `json:"allowed_toolsets,omitempty"`
	AllowedMCP        []string  `json:"allowed_mcp_bindings,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type PolicyList struct {
	Object string   `json:"object"`
	Data   []Policy `json:"data"`
}

type CreateBudgetRequest struct {
	ProjectID     string  `json:"project_id"`
	Name          string  `json:"name"`
	Mode          string  `json:"mode"`
	LimitUSD      float64 `json:"limit_usd,omitempty"`
	LimitRequests int64   `json:"limit_requests,omitempty"`
	Window        string  `json:"window,omitempty"`
}

type Budget struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"project_id"`
	Name          string    `json:"name"`
	Mode          string    `json:"mode"`
	LimitUSD      float64   `json:"limit_usd"`
	LimitRequests int64     `json:"limit_requests"`
	Window        string    `json:"window"`
	CreatedAt     time.Time `json:"created_at"`
}

type BudgetList struct {
	Object string   `json:"object"`
	Data   []Budget `json:"data"`
}

type CreateToolRequest struct {
	Name           string          `json:"name"`
	Description    string          `json:"description,omitempty"`
	Implementation string          `json:"implementation"`
	InputSchema    json.RawMessage `json:"input_schema,omitempty"`
	Enabled        *bool           `json:"enabled,omitempty"`
}

type ToolDefinitionResponse struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description,omitempty"`
	Implementation string    `json:"implementation"`
	InputSchema    string    `json:"input_schema"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
}

type ToolDefinitionList struct {
	Object string                   `json:"object"`
	Data   []ToolDefinitionResponse `json:"data"`
}

type CreateToolsetRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	ToolIDs     []string `json:"tool_ids"`
}

type Toolset struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	ToolIDs     []string  `json:"tool_ids"`
	CreatedAt   time.Time `json:"created_at"`
}

type ToolsetList struct {
	Object string    `json:"object"`
	Data   []Toolset `json:"data"`
}

type CreateMCPBindingRequest struct {
	Name        string            `json:"name"`
	Kind        string            `json:"kind"`
	UpstreamURL string            `json:"upstream_url,omitempty"`
	ToolsetID   string            `json:"toolset_id,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Enabled     *bool             `json:"enabled,omitempty"`
}

type MCPBinding struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Kind        string    `json:"kind"`
	UpstreamURL string    `json:"upstream_url,omitempty"`
	ToolsetID   string    `json:"toolset_id,omitempty"`
	HeadersJSON string    `json:"headers_json"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

type MCPBindingList struct {
	Object string       `json:"object"`
	Data   []MCPBinding `json:"data"`
}
