package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/apierror"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/metrics"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/guardrails"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/reliability"
	"github.com/JiaCheng2004/Polaris/internal/routing"
	"github.com/JiaCheng2004/Polaris/internal/store"
	cachepkg "github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/gin-gonic/gin"
)

type ChatHandler struct {
	runtime     *gwruntime.Holder
	metrics     *metrics.Recorder
	cache       cachepkg.Cache
	store       store.Store
	reliability *reliability.Manager
	router      *routing.Router
	guardrails  *guardrails.Engine
}

type chatTarget struct {
	adapter    modality.ChatAdapter
	model      provider.Model
	resolution provider.Resolution
}

func NewChatHandler(runtime *gwruntime.Holder, recorder *metrics.Recorder, cache cachepkg.Cache, appStore store.Store, reliabilityManager *reliability.Manager) *ChatHandler {
	engine := guardrails.NewEngine(recorder)
	engine.SetJudge(&registryJudge{runtime: runtime})
	return &ChatHandler{runtime: runtime, metrics: recorder, cache: cache, store: appStore, reliability: reliabilityManager, router: routing.NewRouter(), guardrails: engine}
}

func (h *ChatHandler) Complete(c *gin.Context) {
	var req modality.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if err := validateChatRequest(&req); err != nil {
		httputil.WriteError(c, err)
		return
	}

	// Request-phase guardrails run after validate, before routing/cache.
	if h.checkRequestGuardrails(c, &req) {
		return
	}

	primary, fallbacks, err := h.prepareConversation(c, &req)
	if err != nil {
		writeChatTargetError(c, err)
		return
	}
	applyResolvedRoutingHeaders(c, primary.resolution)
	cacheCtl := newResponseCache(c, h.runtime, h.cache)
	if cacheCtl == nil {
		c.Header(cacheHeader, "bypass")
	}

	if req.Stream {
		if cacheCtl != nil {
			cacheCtl.markBypass(c)
		}
		stream, selected, outcome, fallbackModel, err := h.openConversationStream(c, &req, "chat_completions")
		if err != nil {
			middleware.SetRequestOutcome(c, outcome)
			writeChatTargetError(c, err)
			return
		}
		if fallbackModel != "" {
			c.Header("X-Polaris-Fallback", fallbackModel)
			h.metrics.IncFailover(primary.model.ID, fallbackModel)
			c.Header("X-Polaris-Resolved-Model", outcome.Model)
			c.Header("X-Polaris-Resolved-Provider", outcome.Provider)
		}
		h.streamChatCompletions(c, selected, stream, outcome)
		return
	}

	candidate := semanticChatCandidate{}
	if cacheCtl != nil {
		candidate = cacheCtl.prepareSemanticChat(primary.model, &req)
		if candidate.Enabled {
			if cacheCtl.trySemanticChat(c, primary.model, modality.ModalityChat, candidate) {
				return
			}
		} else {
			cacheCtl.markBypass(c)
		}
	}

	response, outcome, fallbackModel, err := h.completeWithFailover(c, primary, fallbacks, &req)
	if err != nil {
		middleware.SetRequestOutcome(c, outcome)
		httputil.WriteError(c, err)
		return
	}

	if fallbackModel != "" {
		c.Header("X-Polaris-Fallback", fallbackModel)
		h.metrics.IncFailover(primary.model.ID, fallbackModel)
		c.Header("X-Polaris-Resolved-Model", outcome.Model)
		c.Header("X-Polaris-Resolved-Provider", outcome.Provider)
	}
	// Response-phase guardrails run before caching/writing (the cache stores the
	// post-redaction text).
	if h.applyResponseGuardrails(c, response, outcome.Model) {
		return
	}
	middleware.SetRequestOutcome(c, outcome)
	if cacheCtl != nil && candidate.Enabled && fallbackModel == "" {
		cacheCtl.storeSemanticChat(c, candidate, http.StatusOK, response)
	}
	c.JSON(http.StatusOK, response)
}

func (h *ChatHandler) streamChatCompletions(c *gin.Context, selected chatTarget, stream <-chan modality.ChatChunk, outcome middleware.RequestOutcome) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)
	c.Writer.Flush()

	releaseStream := h.metrics.StartStream(selected.model.ID, selected.model.Provider)
	defer releaseStream()
	polarisStream := strings.Contains(c.GetHeader("Accept"), "application/x-polaris-stream+json")

	// Per-choice response-phase redactors (nil when guardrails are disabled).
	var redactors []*guardrails.StreamRedactor
	ensureRedactor := func(i int) *guardrails.StreamRedactor {
		for len(redactors) <= i {
			redactors = append(redactors, h.newResponseRedactor(c, selected.model.ID))
		}
		return redactors[i]
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

		if chunk.Model == "" {
			chunk.Model = selected.model.ID
		}
		if chunk.Usage != nil {
			normalizedUsage := normalizeUsage(*chunk.Usage)
			chunk.Usage = &normalizedUsage
			outcome.PromptTokens = chunk.Usage.PromptTokens
			outcome.CompletionTokens = chunk.Usage.CompletionTokens
			outcome.TotalTokens = chunk.Usage.TotalTokens
			outcome.TokenSource = providerUsageSource(*chunk.Usage)
		}
		for i := range chunk.Choices {
			r := ensureRedactor(i)
			if r == nil || !r.Active() {
				continue
			}
			emit, blocked := r.Process(chunk.Choices[i].Delta.Content)
			if blocked {
				writeStreamGuardrailBlocked(c, &outcome)
				return
			}
			chunk.Choices[i].Delta.Content = emit
		}
		var frame any = chunk
		if polarisStream {
			frame = streamEventsFromChunk(chunk)
		}
		if err := writeSSEData(c, frame); err != nil {
			outcome.ErrorType = "provider_error"
			middleware.SetRequestOutcome(c, outcome)
			return
		}
	}

	// Flush any held-back tail from each redactor as a final content frame.
	for i, r := range redactors {
		if r == nil || !r.Active() {
			continue
		}
		emit, blocked := r.Flush()
		if blocked {
			writeStreamGuardrailBlocked(c, &outcome)
			return
		}
		if emit != "" {
			tail := modality.ChatChunk{Model: selected.model.ID, Choices: []modality.ChatChunkChoice{{Index: i, Delta: modality.ChatDelta{Content: emit}}}}
			var frame any = tail
			if polarisStream {
				frame = streamEventsFromChunk(tail)
			}
			_ = writeSSEData(c, frame)
		}
	}

	middleware.SetRequestOutcome(c, outcome)
	_ = writeSSEDone(c)
}

func streamEventsFromChunk(chunk modality.ChatChunk) []modality.StreamEvent {
	var events []modality.StreamEvent
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			events = append(events, modality.StreamEvent{Kind: modality.StreamEventTextDelta, Text: choice.Delta.Content})
		}
		if len(choice.Delta.ToolCalls) > 0 {
			events = append(events, modality.StreamEvent{Kind: modality.StreamEventToolCallDelta, ToolCalls: choice.Delta.ToolCalls})
		}
		if choice.FinishReason != nil {
			events = append(events, modality.StreamEvent{Kind: modality.StreamEventDone})
		}
	}
	if chunk.Usage != nil {
		events = append(events, modality.StreamEvent{Kind: modality.StreamEventUsage, Usage: chunk.Usage})
	}
	if len(events) == 0 {
		events = append(events, modality.StreamEvent{Kind: modality.StreamEventTextDelta})
	}
	return events
}

func shouldRetryWithFallback(apiErr *httputil.APIError) bool {
	return apierror.Retryable(apiErr)
}

func writeChatTargetError(c *gin.Context, err error) {
	var apiErr *httputil.APIError
	if errors.As(err, &apiErr) {
		httputil.WriteError(c, apiErr)
		return
	}
	switch {
	case errors.Is(err, provider.ErrUnknownAlias):
		httputil.WriteError(c, httputil.NewError(http.StatusNotFound, "model_not_found", "unknown_alias", "model", "Model alias is not defined."))
	case errors.Is(err, provider.ErrUnknownModel):
		httputil.WriteError(c, httputil.NewError(http.StatusNotFound, "model_not_found", "unknown_model", "model", "Requested model is not registered."))
	case errors.Is(err, provider.ErrRouteNotResolved):
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "routing_no_match", "model", "Routing selector did not match any enabled model."))
	case errors.Is(err, provider.ErrModalityMismatch):
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "modality_mismatch", "model", "Requested model does not support the chat endpoint."))
	case errors.Is(err, provider.ErrCapabilityMissing):
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "capability_missing", "", "Requested model does not support the required capability."))
	case errors.Is(err, provider.ErrAdapterMissing):
		httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "adapter_unavailable", "model", "Requested model is configured but not available in this runtime build."))
	default:
		httputil.WriteError(c, err)
	}
}

func apiErrorFrom(err error) *httputil.APIError {
	var apiErr *httputil.APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return httputil.NewError(http.StatusInternalServerError, "internal_error", "internal_error", "", "An internal error occurred.")
}
