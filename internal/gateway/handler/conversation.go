package handler

import (
	"errors"
	"net/http"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/gin-gonic/gin"
)

func (h *ChatHandler) prepareConversation(c *gin.Context, req *modality.ChatRequest) (chatTarget, []chatTarget, error) {
	allowDerivedFileUnderstanding := false
	if snapshot := middleware.RuntimeSnapshot(c, h.runtime); snapshot != nil && snapshot.Config != nil {
		allowDerivedFileUnderstanding = fileUnderstandingPolicy(req, snapshot.Config.Files).Enabled
	}
	requiredCapabilities, err := requiredCapabilities(req, allowDerivedFileUnderstanding)
	if err != nil {
		return chatTarget{}, nil, err
	}

	auth := middleware.GetAuthContext(c)
	registry := h.registry(c)
	if registry == nil {
		return chatTarget{}, nil, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable.")
	}

	primary, err := h.resolveChatTarget(c, registry, auth, req.Model, req.Routing, requiredCapabilities)
	if err != nil {
		if errors.Is(err, provider.ErrCapabilityMissing) && requestHasFileParts(req) && !allowDerivedFileUnderstanding {
			return chatTarget{}, nil, fileUnderstandingRequiredError("")
		}
		return chatTarget{}, nil, err
	}
	fallbacks := h.resolveFallbackTargets(c, registry, auth, primary.model.ID, requiredCapabilities)
	// Apply the routing strategy (P3). Static default → unchanged order.
	targets := h.routeTargets(c, append([]chatTarget{primary}, fallbacks...), modality.ModalityChat)
	return targets[0], targets[1:], nil
}

func (h *ChatHandler) executeConversation(c *gin.Context, req *modality.ChatRequest, interfaceFamily string) (*modality.ChatResponse, middleware.RequestOutcome, string, error) {
	primary, fallbacks, err := h.prepareConversation(c, req)
	if err != nil {
		return nil, middleware.RequestOutcome{}, "", err
	}

	response, outcome, fallbackModel, err := h.completeWithFailover(c, primary, fallbacks, req)
	outcome.InterfaceFamily = interfaceFamily
	if fallbackModel != "" {
		outcome.FallbackModel = fallbackModel
	}
	return response, outcome, fallbackModel, err
}

func (h *ChatHandler) openConversationStream(c *gin.Context, req *modality.ChatRequest, interfaceFamily string) (<-chan modality.ChatChunk, chatTarget, middleware.RequestOutcome, string, error) {
	primary, fallbacks, err := h.prepareConversation(c, req)
	if err != nil {
		return nil, chatTarget{}, middleware.RequestOutcome{}, "", err
	}

	targets := append([]chatTarget{primary}, fallbacks...)
	return runFailover(h, c, targets, primary, req, interfaceFamily, 1, true, noAvailableProviderError(), invokeStream, nil)
}

func (h *ChatHandler) completeFallbackConversation(c *gin.Context, primary chatTarget, fallbacks []chatTarget, req *modality.ChatRequest, interfaceFamily string) (*modality.ChatResponse, middleware.RequestOutcome, string, error) {
	response, _, outcome, fallbackModel, err := runFailover(h, c, fallbacks, primary, req, interfaceFamily, 2, false, noConfiguredFallbackError(), invokeComplete, onCompleteSuccess)
	return response, outcome, fallbackModel, err
}

func (h *ChatHandler) openFallbackConversationStream(c *gin.Context, primary chatTarget, fallbacks []chatTarget, req *modality.ChatRequest, interfaceFamily string) (<-chan modality.ChatChunk, chatTarget, middleware.RequestOutcome, string, error) {
	return runFailover(h, c, fallbacks, primary, req, interfaceFamily, 2, false, noConfiguredFallbackError(), invokeStream, nil)
}

func writeConversationFallbackHeaders(c *gin.Context, h *ChatHandler, originalModel string, outcome middleware.RequestOutcome, fallbackModel string) {
	if fallbackModel == "" {
		return
	}
	c.Header("X-Polaris-Fallback", fallbackModel)
	if h != nil && h.metrics != nil {
		h.metrics.IncFailover(originalModel, fallbackModel)
	}
	c.Header("X-Polaris-Resolved-Model", outcome.Model)
	c.Header("X-Polaris-Resolved-Provider", outcome.Provider)
}

func writeNativeConversationError(c *gin.Context, target chatTarget, interfaceFamily string, err error) {
	apiErr := apiErrorFrom(err)
	middleware.SetRequestOutcome(c, middleware.RequestOutcome{
		Model:           target.model.ID,
		Provider:        target.model.Provider,
		Modality:        modality.ModalityChat,
		InterfaceFamily: interfaceFamily,
		StatusCode:      apiErr.Status,
		ErrorType:       apiErr.Type,
	})
	writeConversationTargetError(c, interfaceFamily, err)
}

func writeConversationTargetError(c *gin.Context, endpointName string, err error) {
	writeModalityTargetError(c, err, endpointName)
}

func effectiveMaxOutputTokens(model provider.Model, reqMaxTokens int, fallback int) int {
	if reqMaxTokens > 0 {
		return reqMaxTokens
	}
	if model.MaxOutputTokens > 0 {
		return model.MaxOutputTokens
	}
	return fallback
}
