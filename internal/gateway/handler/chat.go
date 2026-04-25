package handler

import (
	"errors"
	"net/http"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/metrics"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	retrypkg "github.com/JiaCheng2004/Polaris/internal/provider/common/retry"
	cachepkg "github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/gin-gonic/gin"
)

type ChatHandler struct {
	runtime *gwruntime.Holder
	metrics *metrics.Recorder
	cache   cachepkg.Cache
}

type chatTarget struct {
	adapter    modality.ChatAdapter
	model      provider.Model
	resolution provider.Resolution
}

func NewChatHandler(runtime *gwruntime.Holder, recorder *metrics.Recorder, cache cachepkg.Cache) *ChatHandler {
	return &ChatHandler{runtime: runtime, metrics: recorder, cache: cache}
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
		if err := writeSSEData(c, chunk); err != nil {
			outcome.ErrorType = "provider_error"
			middleware.SetRequestOutcome(c, outcome)
			return
		}
	}

	middleware.SetRequestOutcome(c, outcome)
	_ = writeSSEDone(c)
}

func shouldRetryWithFallback(apiErr *httputil.APIError) bool {
	return retrypkg.ShouldRetryAPIError(apiErr)
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
