package openaicompat

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/apierror"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/chattools"
)

type RequestTranslator func(req *modality.ChatRequest, stream bool, providerModel string) any

type ChatAdapter struct {
	client     *Client
	model      string
	translator RequestTranslator
}

func NewChatAdapter(client *Client, model string, translator RequestTranslator) *ChatAdapter {
	return &ChatAdapter{
		client:     client,
		model:      model,
		translator: translator,
	}
}

func (a *ChatAdapter) Complete(ctx context.Context, req *modality.ChatRequest) (*modality.ChatResponse, error) {
	payload := a.translateRequest(req, false)

	var response modality.ChatResponse
	if err := a.client.JSON(ctx, "/chat/completions", payload, &response); err != nil {
		return nil, err
	}

	NormalizeChatResponse(&response, req.Model, a.model)
	return &response, nil
}

func (a *ChatAdapter) Stream(ctx context.Context, req *modality.ChatRequest) (<-chan modality.ChatChunk, error) {
	payload := a.translateRequest(req, true)

	resp, err := a.client.Stream(ctx, "/chat/completions", payload)
	if err != nil {
		return nil, err
	}

	stream := make(chan modality.ChatChunk)
	go func() {
		defer close(stream)
		defer func() {
			_ = resp.Body.Close()
		}()

		if err := DecodeStream(a.client.ProviderName(), resp.Body, req.Model, a.model, stream); err != nil {
			stream <- modality.ChatChunk{Err: err}
		}
	}()

	return stream, nil
}

func (a *ChatAdapter) translateRequest(req *modality.ChatRequest, stream bool) any {
	providerModel := ProviderModelName(req.Model, a.model)
	if a.translator != nil {
		return a.translator(req, stream, providerModel)
	}
	payload := struct {
		Model           string                   `json:"model"`
		Messages        []modality.ChatMessage   `json:"messages"`
		Temperature     *float64                 `json:"temperature,omitempty"`
		TopP            *float64                 `json:"top_p,omitempty"`
		MaxTokens       int                      `json:"max_tokens,omitempty"`
		Stream          bool                     `json:"stream,omitempty"`
		Tools           []map[string]any         `json:"tools,omitempty"`
		ToolChoice      json.RawMessage          `json:"tool_choice,omitempty"`
		ResponseFormat  *modality.ResponseFormat `json:"response_format,omitempty"`
		Stop            []string                 `json:"stop,omitempty"`
		ReasoningEffort string                   `json:"reasoning_effort,omitempty"`
	}{
		Model:           providerModel,
		Messages:        append([]modality.ChatMessage(nil), req.Messages...),
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		MaxTokens:       req.MaxTokens,
		Stream:          stream,
		Tools:           chattools.OpenAITools(req.Tools),
		ToolChoice:      req.ToolChoice,
		ResponseFormat:  req.ResponseFormat,
		Stop:            append([]string(nil), req.Stop...),
		ReasoningEffort: modality.ReasoningEffort(req.Reasoning),
	}
	return payload
}

func DecodeStream(providerName string, r io.Reader, canonicalModel string, fallbackModel string, dst chan<- modality.ChatChunk) error {
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
			Message string `json:"message"`
		}
		if err := json.Unmarshal([]byte(payload), &envelope); err == nil && (envelope.Error != nil || strings.TrimSpace(envelope.Message) != "") {
			message := strings.TrimSpace(envelope.Message)
			if envelope.Error != nil && strings.TrimSpace(envelope.Error.Message) != "" {
				message = envelope.Error.Message
			}
			if strings.TrimSpace(message) == "" {
				message = providerName + " streaming request failed."
			}
			return false, apierror.NewError(502, "provider_error", "provider_stream_error", "", message)
		}

		var chunk modality.ChatChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return false, fmt.Errorf("decode %s stream chunk: %w", strings.ToLower(providerName), err)
		}
		NormalizeChatChunk(&chunk, canonicalModel, fallbackModel)
		dst <- chunk
		return false, nil
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			if err == io.EOF {
				_, flushErr := flush()
				return flushErr
			}
			return fmt.Errorf("read %s stream: %w", strings.ToLower(providerName), err)
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

func NormalizeChatResponse(response *modality.ChatResponse, canonicalModel string, fallbackModel string) {
	if response.Object == "" {
		response.Object = "chat.completion"
	}
	if response.Created == 0 {
		response.Created = time.Now().Unix()
	}
	if response.Model == "" || !strings.Contains(response.Model, "/") {
		response.Model = FirstNonEmpty(canonicalModel, fallbackModel)
	}
}

func NormalizeChatChunk(chunk *modality.ChatChunk, canonicalModel string, fallbackModel string) {
	if chunk.Object == "" {
		chunk.Object = "chat.completion.chunk"
	}
	if chunk.Created == 0 {
		chunk.Created = time.Now().Unix()
	}
	if chunk.Model == "" || !strings.Contains(chunk.Model, "/") {
		chunk.Model = FirstNonEmpty(canonicalModel, fallbackModel)
	}
}

func ProviderModelName(requestModel string, fallbackModel string) string {
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

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
