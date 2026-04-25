package handler

import (
	"bytes"
	"math"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gin-gonic/gin"
)

type TokensHandler struct {
	chat *ChatHandler
}

func NewTokensHandler(chat *ChatHandler) *TokensHandler {
	return &TokensHandler{chat: chat}
}

func (h *TokensHandler) Count(c *gin.Context) {
	var req modality.TokenCountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required."))
		return
	}
	if req.RequestedInterface != "" && req.RequestedInterface != "chat" && req.RequestedInterface != "responses" && req.RequestedInterface != "messages" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_requested_interface", "requested_interface", "Field 'requested_interface' must be chat, responses, or messages."))
		return
	}

	messages, notes, err := normalizeTokenCountMessages(req)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if len(messages) == 0 {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_input", "messages", "Field 'messages' or 'input' is required."))
		return
	}

	registry := h.chat.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable."))
		return
	}

	chatReq := &modality.ChatRequest{
		Model:     req.Model,
		Routing:   req.Routing,
		Messages:  messages,
		MaxTokens: req.MaxOutputTokens,
	}
	target, _, err := h.chat.prepareConversation(c, chatReq)
	if err != nil {
		writeConversationTargetError(c, "tokens/count", err)
		return
	}
	applyResolvedRoutingHeaders(c, target.resolution)

	outputEstimate := effectiveMaxOutputTokens(target.model, req.MaxOutputTokens, 1024)
	if counter, ok := target.adapter.(modality.ConversationTokenCounter); ok {
		result, err := counter.CountTokens(c.Request.Context(), chatReq)
		if err == nil && result != nil {
			notes = append(notes, result.Notes...)
			c.JSON(http.StatusOK, modality.TokenCountResponse{
				Model:                target.model.ID,
				InputTokens:          result.InputTokens,
				OutputTokensEstimate: outputEstimate,
				Source:               result.Source,
				Notes:                notes,
			})
			return
		}
	}

	inputTokens, tokenNotes := estimateConversationTokens(messages)
	notes = append(notes, tokenNotes...)
	source := modality.TokenCountSourceEstimated
	if inputTokens == 0 && outputEstimate == 0 {
		source = modality.TokenCountSourceUnavailable
	}

	c.JSON(http.StatusOK, modality.TokenCountResponse{
		Model:                target.model.ID,
		InputTokens:          inputTokens,
		OutputTokensEstimate: outputEstimate,
		Source:               source,
		Notes:                notes,
	})
}

func normalizeTokenCountMessages(req modality.TokenCountRequest) ([]modality.ChatMessage, []string, error) {
	if len(req.Messages) > 0 {
		return append([]modality.ChatMessage(nil), req.Messages...), nil, nil
	}
	if len(bytes.TrimSpace(req.Input)) == 0 || bytes.Equal(bytes.TrimSpace(req.Input), []byte("null")) {
		return nil, nil, nil
	}
	messages, err := normalizeResponsesInput(req.Input)
	if err != nil {
		return nil, nil, err
	}
	notes := []string{"input was normalized into Polaris conversation messages before token counting"}
	return messages, notes, nil
}

func estimateConversationTokens(messages []modality.ChatMessage) (int, []string) {
	total := 0
	notes := []string{"provider token counts are unavailable for this endpoint; Polaris returned an estimate"}
	multimodal := false

	for _, message := range messages {
		total += 4
		total += estimateStringTokens(message.Role)
		total += estimateStringTokens(message.Name)
		total += estimateStringTokens(message.ToolCallID)
		if message.Content.Text != nil {
			total += estimateStringTokens(*message.Content.Text)
		}
		for _, part := range message.Content.Parts {
			switch part.Type {
			case "text":
				total += estimateStringTokens(part.Text)
			case "image_url":
				multimodal = true
				total += estimateImageTokens(part)
			case "input_audio":
				multimodal = true
				total += estimateAudioTokens(part)
			}
		}
		for _, toolCall := range message.ToolCalls {
			total += 8
			total += estimateStringTokens(toolCall.ID)
			total += estimateStringTokens(toolCall.Function.Name)
			total += estimateStringTokens(toolCall.Function.Arguments)
		}
	}
	if multimodal {
		notes = append(notes, "multimodal content uses coarse token estimation heuristics")
	}
	return total, notes
}

func estimateStringTokens(value string) int {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	wordTokens := len(strings.Fields(trimmed))
	runeTokens := int(math.Ceil(float64(utf8.RuneCountInString(trimmed)) / 3.0))
	if runeTokens < 1 {
		runeTokens = 1
	}
	if wordTokens > runeTokens {
		return wordTokens
	}
	return runeTokens
}

func estimateImageTokens(part modality.ContentPart) int {
	if part.ImageURL == nil {
		return 0
	}
	url := strings.TrimSpace(part.ImageURL.URL)
	if url == "" {
		return 0
	}
	if strings.HasPrefix(url, "data:") {
		return clampInt(len(url)/256, 64, 2048)
	}
	return 256
}

func estimateAudioTokens(part modality.ContentPart) int {
	if part.InputAudio == nil {
		return 0
	}
	if part.InputAudio.Data == "" {
		return 0
	}
	return clampInt(len(part.InputAudio.Data)/128, 64, 4096)
}

func clampInt(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
