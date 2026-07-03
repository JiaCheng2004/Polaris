package anthropiccompat

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/apierror"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

type NativeMessagesAdapter struct {
	client      *Client
	model       string
	modelMapper ModelMapper
}

func NewNativeMessagesAdapter(client *Client, model string) *NativeMessagesAdapter {
	return NewNativeMessagesAdapterWithModelMapper(client, model, nil)
}

func NewNativeMessagesAdapterWithModelMapper(client *Client, model string, modelMapper ModelMapper) *NativeMessagesAdapter {
	return &NativeMessagesAdapter{client: client, model: model, modelMapper: modelMapper}
}

func (a *NativeMessagesAdapter) CreateMessage(ctx context.Context, raw json.RawMessage, canonicalModel string) (*modality.NativeJSONResponse, error) {
	return createMessage(ctx, a.client, raw, canonicalModel, a.wireModelName(canonicalModel))
}

func (a *NativeMessagesAdapter) StreamMessage(ctx context.Context, raw json.RawMessage, canonicalModel string) (<-chan modality.NativeJSONStreamEvent, error) {
	return streamMessage(ctx, a.client, raw, canonicalModel, a.wireModelName(canonicalModel))
}

func (a *ChatAdapter) CreateMessage(ctx context.Context, raw json.RawMessage, canonicalModel string) (*modality.NativeJSONResponse, error) {
	return createMessage(ctx, a.client, raw, canonicalModel, a.wireModelName(canonicalModel))
}

func (a *ChatAdapter) StreamMessage(ctx context.Context, raw json.RawMessage, canonicalModel string) (<-chan modality.NativeJSONStreamEvent, error) {
	return streamMessage(ctx, a.client, raw, canonicalModel, a.wireModelName(canonicalModel))
}

func (a *NativeMessagesAdapter) wireModelName(requestModel string) string {
	model := providerModelName(requestModel, a.model)
	if a.modelMapper != nil {
		return a.modelMapper(model)
	}
	return model
}

func createMessage(ctx context.Context, client *Client, raw json.RawMessage, canonicalModel string, wireModel string) (*modality.NativeJSONResponse, error) {
	payload, err := rewriteAnthropicRequestModel(raw, wireModel)
	if err != nil {
		return nil, err
	}

	var response json.RawMessage
	if _, err := client.JSON(ctx, "/v1/messages", payload, &response); err != nil {
		return nil, err
	}

	rewritten, usage, err := rewriteAnthropicMessagePayload(response, canonicalModel)
	if err != nil {
		return nil, err
	}
	return &modality.NativeJSONResponse{
		Payload: rewritten,
		Model:   canonicalModel,
		Usage:   usage,
	}, nil
}

func streamMessage(ctx context.Context, client *Client, raw json.RawMessage, canonicalModel string, wireModel string) (<-chan modality.NativeJSONStreamEvent, error) {
	payload, err := rewriteAnthropicRequestModel(raw, wireModel)
	if err != nil {
		return nil, err
	}

	resp, err := client.Stream(ctx, "/v1/messages", payload)
	if err != nil {
		return nil, err
	}

	stream := make(chan modality.NativeJSONStreamEvent)
	go func() {
		defer close(stream)
		defer func() {
			_ = resp.Body.Close()
		}()
		if err := decodeAnthropicNativeStream(resp.Body, canonicalModel, stream); err != nil {
			stream <- modality.NativeJSONStreamEvent{Err: err}
		}
	}()

	return stream, nil
}

func decodeAnthropicNativeStream(r io.Reader, canonicalModel string, dst chan<- modality.NativeJSONStreamEvent) error {
	reader := bufio.NewReader(r)
	var (
		eventType string
		dataLines []string
	)

	flush := func() error {
		if eventType == "" && len(dataLines) == 0 {
			return nil
		}

		payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
		currentEvent := eventType
		eventType = ""
		dataLines = nil

		if currentEvent == "ping" || payload == "" {
			return nil
		}
		if currentEvent == "error" {
			var envelope struct {
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			_ = json.Unmarshal([]byte(payload), &envelope)
			message := envelope.Error.Message
			if message == "" {
				message = "Anthropic-compatible messages streaming request failed."
			}
			return apierror.NewError(http.StatusBadGateway, "provider_error", "provider_stream_error", "", message)
		}

		rewritten, usage, err := rewriteAnthropicMessagePayload([]byte(payload), canonicalModel)
		if err != nil {
			return err
		}
		dst <- modality.NativeJSONStreamEvent{
			Event:   currentEvent,
			Payload: rewritten,
			Model:   canonicalModel,
			Usage:   usage,
		}
		return nil
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			if err == io.EOF {
				return flush()
			}
			return fmt.Errorf("read anthropic-compatible messages stream: %w", err)
		}

		trimmed := strings.TrimRight(line, "\r\n")
		switch {
		case trimmed == "":
			if err := flush(); err != nil {
				return err
			}
		case strings.HasPrefix(trimmed, "event:"):
			eventType = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
		case strings.HasPrefix(trimmed, "data:"):
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
		}

		if err == io.EOF {
			return flush()
		}
	}
}

func rewriteAnthropicRequestModel(raw json.RawMessage, model string) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, apierror.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON.")
	}
	payload["model"] = model
	return payload, nil
}

func rewriteAnthropicMessagePayload(raw json.RawMessage, canonicalModel string) (json.RawMessage, *modality.Usage, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, fmt.Errorf("decode anthropic-compatible message payload: %w", err)
	}

	var usage *modality.Usage
	if message, ok := payload["message"].(map[string]any); ok {
		message["model"] = canonicalModel
		usage = anthropicUsageFromAny(message["usage"])
	} else {
		payload["model"] = canonicalModel
		usage = anthropicUsageFromAny(payload["usage"])
	}
	if usage == nil {
		usage = anthropicUsageFromAny(payload["usage"])
	}

	rewritten, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("encode anthropic-compatible message payload: %w", err)
	}
	return rewritten, usage, nil
}

func anthropicUsageFromAny(value any) *modality.Usage {
	object, ok := value.(map[string]any)
	if !ok || len(object) == 0 {
		return nil
	}

	input := intFromAny(object["input_tokens"])
	output := intFromAny(object["output_tokens"])
	total := intFromAny(object["total_tokens"])
	if total == 0 {
		total = input + output
	}
	cacheRead := intFromAny(object["cache_read_input_tokens"])
	cacheWrite5m := intFromAny(object["cache_creation_5m_input_tokens"])
	cacheWrite1h := intFromAny(object["cache_creation_1h_input_tokens"])
	if cacheWrite5m == 0 {
		cacheWrite5m = intFromAny(object["cache_creation_input_tokens"])
	}
	if creation, ok := object["cache_creation"].(map[string]any); ok {
		cacheWrite5m += intFromAny(creation["ephemeral_5m_input_tokens"])
		cacheWrite1h += intFromAny(creation["ephemeral_1h_input_tokens"])
	}
	if input == 0 && output == 0 && total == 0 && cacheRead == 0 && cacheWrite5m == 0 && cacheWrite1h == 0 {
		return nil
	}

	return &modality.Usage{
		PromptTokens:       input,
		CompletionTokens:   output,
		TotalTokens:        total,
		CachedInputTokens:  cacheRead,
		CacheWrite5mTokens: cacheWrite5m,
		CacheWrite1hTokens: cacheWrite1h,
		Source:             modality.TokenCountSourceProviderReported,
	}
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	default:
		return 0
	}
}
