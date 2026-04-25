package handler

import (
	"net/http"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/gateway/telemetry"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
)

func (h *ChatHandler) prepareConversation(c *gin.Context, req *modality.ChatRequest) (chatTarget, []chatTarget, error) {
	requiredCapabilities, err := requiredCapabilities(req)
	if err != nil {
		return chatTarget{}, nil, err
	}

	auth := middleware.GetAuthContext(c)
	registry := h.registry(c)
	if registry == nil {
		return chatTarget{}, nil, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable.")
	}

	primary, err := h.resolveChatTarget(c.Request.Context(), registry, auth, req.Model, req.Routing, requiredCapabilities)
	if err != nil {
		return chatTarget{}, nil, err
	}
	fallbacks := h.resolveFallbackTargets(c.Request.Context(), registry, auth, primary.model.ID, requiredCapabilities)
	return primary, fallbacks, nil
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
	var lastOutcome middleware.RequestOutcome
	for index, target := range targets {
		attemptReq := *req
		attemptReq.Model = target.model.ID

		start := time.Now()
		attemptCtx, attemptSpan := telemetry.StartInternalSpan(c.Request.Context(), "fallback.attempt",
			attribute.Int("polaris.fallback_attempt", index+1),
			attribute.String("polaris.provider", target.model.Provider),
			attribute.String("polaris.model", target.model.ID),
			attribute.String("polaris.fallback_from", primary.model.ID),
		)
		stream, err := target.adapter.Stream(attemptCtx, &attemptReq)
		if err != nil {
			telemetry.RecordSpanError(attemptSpan, err)
		}
		attemptSpan.End()
		providerLatencyMs := int(time.Since(start).Milliseconds())
		if err != nil {
			apiErr := apiErrorFrom(err)
			h.metrics.IncProviderError(target.model.Provider, apiErr.Type)
			lastOutcome = middleware.RequestOutcome{
				Model:             target.model.ID,
				Provider:          target.model.Provider,
				Modality:          modality.ModalityChat,
				InterfaceFamily:   interfaceFamily,
				StatusCode:        apiErr.Status,
				ErrorType:         apiErr.Type,
				ProviderLatencyMs: providerLatencyMs,
			}
			if index < len(targets)-1 && shouldRetryWithFallback(apiErr) {
				continue
			}
			return nil, chatTarget{}, lastOutcome, "", apiErr
		}

		outcome := middleware.RequestOutcome{
			Model:             target.model.ID,
			Provider:          target.model.Provider,
			Modality:          modality.ModalityChat,
			InterfaceFamily:   interfaceFamily,
			StatusCode:        http.StatusOK,
			ProviderLatencyMs: providerLatencyMs,
		}
		fallbackModel := ""
		if index > 0 {
			fallbackModel = target.model.ID
			outcome.FallbackModel = fallbackModel
			telemetry.AnnotateCurrentSpan(c.Request.Context(),
				attribute.String("polaris.fallback_from", primary.model.ID),
				attribute.String("polaris.fallback_to", fallbackModel),
			)
		}
		return stream, target, outcome, fallbackModel, nil
	}

	return nil, chatTarget{}, lastOutcome, "", httputil.NewError(http.StatusBadGateway, "provider_error", "provider_unavailable", "model", "No available provider could serve this request.")
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
