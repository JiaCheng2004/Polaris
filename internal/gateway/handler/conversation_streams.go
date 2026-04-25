package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gin-gonic/gin"
)

type responsesStreamEvent struct {
	Type         string                `json:"type"`
	Response     *responsesAPIResponse `json:"response,omitempty"`
	Item         *responsesOutputItem  `json:"item,omitempty"`
	ItemID       string                `json:"item_id,omitempty"`
	OutputIndex  int                   `json:"output_index,omitempty"`
	ContentIndex int                   `json:"content_index,omitempty"`
	Delta        string                `json:"delta,omitempty"`
	Text         string                `json:"text,omitempty"`
}

type messagesStreamEvent struct {
	Type         string               `json:"type"`
	Message      *messagesAPIResponse `json:"message,omitempty"`
	Index        int                  `json:"index,omitempty"`
	ContentBlock *messagesOutputBlock `json:"content_block,omitempty"`
	Delta        any                  `json:"delta,omitempty"`
	Usage        *messagesUsage       `json:"usage,omitempty"`
}

func (h *ChatHandler) streamResponses(c *gin.Context, selected chatTarget, stream <-chan modality.ChatChunk, outcome middleware.RequestOutcome, req *modality.ChatRequest, metadata map[string]string) {
	releaseStream := h.metrics.StartStream(selected.model.ID, selected.model.Provider)
	defer releaseStream()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)
	c.Writer.Flush()

	state := responsesAPIResponse{
		ID:        "resp_" + firstNonEmptyString(middleware.GetRequestID(c), selected.model.ID),
		Object:    "response",
		CreatedAt: time.Now().Unix(),
		Status:    "in_progress",
		Model:     selected.model.ID,
		Output: []responsesOutputItem{{
			ID:      "msg_" + firstNonEmptyString(middleware.GetRequestID(c), selected.model.ID),
			Type:    "message",
			Role:    "assistant",
			Content: []responsesContentItem{},
		}},
		Usage:    responsesUsage{Source: string(modality.TokenCountSourceUnavailable)},
		Metadata: cloneStringMap(metadata),
	}
	if err := writeSSEData(c, responsesStreamEvent{Type: "response.created", Response: &state}); err != nil {
		middleware.SetRequestOutcome(c, outcome)
		return
	}

	for chunk := range stream {
		if chunk.Err != nil {
			apiErr := apiErrorFrom(chunk.Err)
			h.metrics.IncProviderError(selected.model.Provider, apiErr.Type)
			outcome.ErrorType = apiErr.Type
			middleware.SetRequestOutcome(c, outcome)
			if err := writeSSEData(c, httputil.ErrorEnvelope{
				Error: httputil.ErrorBody{
					Message: apiErr.Message,
					Type:    apiErr.Type,
					Code:    apiErr.Code,
					Param:   apiErr.Param,
				},
			}); err == nil {
				_ = writeSSEDone(c)
			}
			return
		}
		if chunk.Model != "" {
			state.Model = chunk.Model
			outcome.Model = chunk.Model
		}
		if chunk.Usage != nil {
			normalizedUsage := normalizeUsage(*chunk.Usage)
			chunk.Usage = &normalizedUsage
			outcome.PromptTokens = chunk.Usage.PromptTokens
			outcome.CompletionTokens = chunk.Usage.CompletionTokens
			outcome.TotalTokens = chunk.Usage.TotalTokens
			outcome.TokenSource = providerUsageSource(*chunk.Usage)
			state.Usage = responsesUsage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
				TotalTokens:  chunk.Usage.TotalTokens,
				Source:       string(chunk.Usage.Source),
			}
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		if delta := choice.Delta.Content; strings.TrimSpace(delta) != "" {
			messageItem := &state.Output[0]
			if len(messageItem.Content) == 0 {
				messageItem.Content = []responsesContentItem{{Type: "output_text"}}
			}
			messageItem.Content[0].Text += delta
			state.OutputText += delta
			if err := writeSSEData(c, responsesStreamEvent{
				Type:         "response.output_text.delta",
				ItemID:       messageItem.ID,
				OutputIndex:  0,
				ContentIndex: 0,
				Delta:        delta,
			}); err != nil {
				middleware.SetRequestOutcome(c, outcome)
				return
			}
		}
		for _, toolCall := range choice.Delta.ToolCalls {
			item := responsesOutputItem{
				ID:        toolCall.ID,
				Type:      "function_call",
				Name:      toolCall.Function.Name,
				Arguments: toolCall.Function.Arguments,
				CallID:    toolCall.ID,
			}
			state.Output = append(state.Output, item)
			if err := writeSSEData(c, responsesStreamEvent{
				Type:        "response.output_item.added",
				Item:        &item,
				OutputIndex: len(state.Output) - 1,
			}); err != nil {
				middleware.SetRequestOutcome(c, outcome)
				return
			}
		}
	}

	if len(state.Output) > 0 && len(state.Output[0].Content) > 0 {
		_ = writeSSEData(c, responsesStreamEvent{
			Type:         "response.output_text.done",
			ItemID:       state.Output[0].ID,
			OutputIndex:  0,
			ContentIndex: 0,
			Text:         state.OutputText,
		})
	}
	state.Status = "completed"
	middleware.SetRequestOutcome(c, outcome)
	_ = writeSSEData(c, responsesStreamEvent{Type: "response.completed", Response: &state})
	_ = writeSSEDone(c)
}

func (h *ChatHandler) streamMessages(c *gin.Context, selected chatTarget, stream <-chan modality.ChatChunk, outcome middleware.RequestOutcome) {
	releaseStream := h.metrics.StartStream(selected.model.ID, selected.model.Provider)
	defer releaseStream()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)
	c.Writer.Flush()

	state := messagesAPIResponse{
		ID:      "msg_" + firstNonEmptyString(middleware.GetRequestID(c), selected.model.ID),
		Type:    "message",
		Role:    "assistant",
		Model:   selected.model.ID,
		Content: []messagesOutputBlock{},
		Usage:   messagesUsage{Source: string(modality.TokenCountSourceUnavailable)},
	}
	if err := writeSSEData(c, messagesStreamEvent{Type: "message_start", Message: &state}); err != nil {
		middleware.SetRequestOutcome(c, outcome)
		return
	}

	textIndex := -1
	toolIndexes := map[string]int{}
	toolInputs := map[string]string{}

	for chunk := range stream {
		if chunk.Err != nil {
			apiErr := apiErrorFrom(chunk.Err)
			h.metrics.IncProviderError(selected.model.Provider, apiErr.Type)
			outcome.ErrorType = apiErr.Type
			middleware.SetRequestOutcome(c, outcome)
			if err := writeSSEData(c, httputil.ErrorEnvelope{
				Error: httputil.ErrorBody{
					Message: apiErr.Message,
					Type:    apiErr.Type,
					Code:    apiErr.Code,
					Param:   apiErr.Param,
				},
			}); err == nil {
				_ = writeSSEDone(c)
			}
			return
		}
		if chunk.Model != "" {
			state.Model = chunk.Model
			outcome.Model = chunk.Model
		}
		if chunk.Usage != nil {
			normalizedUsage := normalizeUsage(*chunk.Usage)
			chunk.Usage = &normalizedUsage
			outcome.PromptTokens = chunk.Usage.PromptTokens
			outcome.CompletionTokens = chunk.Usage.CompletionTokens
			outcome.TotalTokens = chunk.Usage.TotalTokens
			outcome.TokenSource = providerUsageSource(*chunk.Usage)
			state.Usage = messagesUsage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
				Source:       string(chunk.Usage.Source),
			}
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		if delta := choice.Delta.Content; strings.TrimSpace(delta) != "" {
			if textIndex < 0 {
				textIndex = len(state.Content)
				block := messagesOutputBlock{Type: "text", Text: ""}
				state.Content = append(state.Content, block)
				if err := writeSSEData(c, messagesStreamEvent{
					Type:         "content_block_start",
					Index:        textIndex,
					ContentBlock: &block,
				}); err != nil {
					middleware.SetRequestOutcome(c, outcome)
					return
				}
			}
			state.Content[textIndex].Text += delta
			if err := writeSSEData(c, messagesStreamEvent{
				Type:  "content_block_delta",
				Index: textIndex,
				Delta: gin.H{"type": "text_delta", "text": delta},
			}); err != nil {
				middleware.SetRequestOutcome(c, outcome)
				return
			}
		}
		for _, toolCall := range choice.Delta.ToolCalls {
			index, ok := toolIndexes[toolCall.ID]
			if !ok {
				index = len(state.Content)
				block := messagesOutputBlock{
					Type:  "tool_use",
					ID:    toolCall.ID,
					Name:  toolCall.Function.Name,
					Input: jsonRawObject(),
				}
				state.Content = append(state.Content, block)
				toolIndexes[toolCall.ID] = index
				if err := writeSSEData(c, messagesStreamEvent{
					Type:         "content_block_start",
					Index:        index,
					ContentBlock: &block,
				}); err != nil {
					middleware.SetRequestOutcome(c, outcome)
					return
				}
			}
			if strings.TrimSpace(toolCall.Function.Arguments) != "" {
				toolInputs[toolCall.ID] += toolCall.Function.Arguments
				state.Content[index].Input = rawToolInput(toolInputs[toolCall.ID])
				if err := writeSSEData(c, messagesStreamEvent{
					Type:  "content_block_delta",
					Index: index,
					Delta: gin.H{"type": "input_json_delta", "partial_json": toolCall.Function.Arguments},
				}); err != nil {
					middleware.SetRequestOutcome(c, outcome)
					return
				}
			}
		}

		if choice.FinishReason != nil {
			if textIndex >= 0 {
				_ = writeSSEData(c, messagesStreamEvent{Type: "content_block_stop", Index: textIndex})
			}
			for _, index := range toolIndexes {
				_ = writeSSEData(c, messagesStreamEvent{Type: "content_block_stop", Index: index})
			}
			_ = writeSSEData(c, messagesStreamEvent{
				Type: "message_delta",
				Delta: gin.H{
					"stop_reason":   anthropicStopReason(*choice.FinishReason),
					"stop_sequence": nil,
				},
				Usage: &state.Usage,
			})
		}
	}

	middleware.SetRequestOutcome(c, outcome)
	_ = writeSSEData(c, messagesStreamEvent{Type: "message_stop"})
	_ = writeSSEDone(c)
}

func jsonRawObject() []byte {
	return []byte(`{}`)
}
