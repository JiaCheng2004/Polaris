package openai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func (a *ChatAdapter) CreateResponse(ctx context.Context, raw json.RawMessage, canonicalModel string) (*modality.NativeJSONResponse, error) {
	payload, err := rewriteOpenAIRequestModel(raw, providerModelName(canonicalModel, a.model))
	if err != nil {
		return nil, err
	}

	var response json.RawMessage
	if err := a.client.JSON(ctx, "/responses", payload, &response); err != nil {
		return nil, err
	}

	rewritten, usage, err := rewriteOpenAIResponsePayload(response, canonicalModel)
	if err != nil {
		return nil, err
	}
	return &modality.NativeJSONResponse{
		Payload: rewritten,
		Model:   canonicalModel,
		Usage:   usage,
	}, nil
}

func (a *ChatAdapter) StreamResponse(ctx context.Context, raw json.RawMessage, canonicalModel string) (<-chan modality.NativeJSONStreamEvent, error) {
	payload, err := rewriteOpenAIRequestModel(raw, providerModelName(canonicalModel, a.model))
	if err != nil {
		return nil, err
	}

	resp, err := a.client.Stream(ctx, "/responses", payload)
	if err != nil {
		return nil, err
	}

	stream := make(chan modality.NativeJSONStreamEvent)
	go func() {
		defer close(stream)
		defer func() {
			_ = resp.Body.Close()
		}()
		if err := decodeOpenAINativeStream(resp.Body, canonicalModel, stream); err != nil {
			stream <- modality.NativeJSONStreamEvent{Err: err}
		}
	}()

	return stream, nil
}

func decodeOpenAINativeStream(r io.Reader, canonicalModel string, dst chan<- modality.NativeJSONStreamEvent) error {
	reader := bufio.NewReader(r)
	var dataLines []string

	flush := func() (bool, error) {
		if len(dataLines) == 0 {
			return false, nil
		}
		payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = nil
		if payload == "" {
			return false, nil
		}
		if payload == "[DONE]" {
			return true, nil
		}

		var envelope struct {
			Error *struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
				Param   string `json:"param"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &envelope); err == nil && envelope.Error != nil {
			message := envelope.Error.Message
			if strings.TrimSpace(message) == "" {
				message = "OpenAI responses streaming request failed."
			}
			return false, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_stream_error", "", message)
		}

		rewritten, usage, err := rewriteOpenAIResponsePayload([]byte(payload), canonicalModel)
		if err != nil {
			return false, err
		}
		dst <- modality.NativeJSONStreamEvent{
			Payload: rewritten,
			Model:   canonicalModel,
			Usage:   usage,
		}
		return false, nil
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			if err == io.EOF {
				_, flushErr := flush()
				return flushErr
			}
			return fmt.Errorf("read openai responses stream: %w", err)
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

func rewriteOpenAIRequestModel(raw json.RawMessage, model string) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON.")
	}
	payload["model"] = model
	return payload, nil
}

func rewriteOpenAIResponsePayload(raw json.RawMessage, canonicalModel string) (json.RawMessage, *modality.Usage, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, fmt.Errorf("decode openai responses payload: %w", err)
	}

	var usage *modality.Usage
	if response, ok := payload["response"].(map[string]any); ok {
		response["model"] = canonicalModel
		usage = nativeUsageFromMap(response["usage"])
	} else {
		payload["model"] = canonicalModel
		usage = nativeUsageFromMap(payload["usage"])
	}

	rewritten, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("encode openai responses payload: %w", err)
	}
	return rewritten, usage, nil
}

func nativeUsageFromMap(value any) *modality.Usage {
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
	if input == 0 && output == 0 && total == 0 {
		return nil
	}

	return &modality.Usage{
		PromptTokens:     input,
		CompletionTokens: output,
		TotalTokens:      total,
		Source:           modality.TokenCountSourceProviderReported,
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
