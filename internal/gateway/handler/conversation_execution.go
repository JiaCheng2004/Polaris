package handler

import (
	"net/http"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/gateway/telemetry"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	retrypkg "github.com/JiaCheng2004/Polaris/internal/provider/common/retry"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
)

func (h *ChatHandler) completeWithFailover(c *gin.Context, primary chatTarget, fallbacks []chatTarget, req *modality.ChatRequest) (*modality.ChatResponse, middleware.RequestOutcome, string, error) {
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
		response, err := target.adapter.Complete(attemptCtx, &attemptReq)
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
				StatusCode:        apiErr.Status,
				ErrorType:         apiErr.Type,
				ProviderLatencyMs: providerLatencyMs,
			}
			if index < len(targets)-1 && retrypkg.ShouldRetryAPIError(apiErr) {
				continue
			}
			return nil, lastOutcome, "", apiErr
		}

		response.Model = target.model.ID
		response.Usage = normalizeUsage(response.Usage)
		outcome := middleware.RequestOutcome{
			Model:             target.model.ID,
			Provider:          target.model.Provider,
			Modality:          modality.ModalityChat,
			StatusCode:        http.StatusOK,
			ProviderLatencyMs: providerLatencyMs,
			PromptTokens:      response.Usage.PromptTokens,
			CompletionTokens:  response.Usage.CompletionTokens,
			TotalTokens:       response.Usage.TotalTokens,
			TokenSource:       providerUsageSource(response.Usage),
		}

		fallbackModel := ""
		if index > 0 {
			fallbackModel = target.model.ID
			telemetry.AnnotateCurrentSpan(c.Request.Context(),
				attribute.String("polaris.fallback_from", primary.model.ID),
				attribute.String("polaris.fallback_to", fallbackModel),
			)
		}
		return response, outcome, fallbackModel, nil
	}

	return nil, lastOutcome, "", httputil.NewError(http.StatusBadGateway, "provider_error", "provider_unavailable", "model", "No available provider could serve this request.")
}

func (h *ChatHandler) resolveChatTarget(c *gin.Context, registry *provider.Registry, auth middleware.AuthContext, name string, routing *modality.RoutingOptions, requiredCapabilities []modality.Capability) (chatTarget, error) {
	ctx := c.Request.Context()
	_, span := telemetry.StartInternalSpan(ctx, "policy.resolve_chat_target",
		attribute.String("polaris.requested_model", name),
		attribute.String("polaris.modality", string(modality.ModalityChat)),
	)
	defer span.End()

	if err := validateRoutingOptions(routing); err != nil {
		telemetry.RecordSpanError(span, err)
		return chatTarget{}, err
	}
	resolution, err := registry.RequireResolvedModel(name, modality.ModalityChat, routing, requiredCapabilities...)
	if err != nil {
		telemetry.RecordSpanError(span, err)
		return chatTarget{}, err
	}
	model := resolution.Model
	adapter, _, err := registry.GetChatAdapter(model.ID)
	if err != nil {
		telemetry.RecordSpanError(span, err)
		return chatTarget{}, err
	}
	if !middleware.ScopeAllowed(auth.AllowedModels, auth.PolicyModels, model.ID) {
		err := httputil.NewError(http.StatusForbidden, "permission_error", "model_not_allowed", "model", "API key is not permitted to use this model.")
		telemetry.RecordSpanError(span, err)
		return chatTarget{}, err
	}
	if !middleware.ModalityScopeAllowed(auth.AllowedModalities, auth.PolicyModalities, modality.ModalityChat) {
		err := httputil.NewError(http.StatusForbidden, "permission_error", "modality_not_allowed", "model", "API key is not permitted to use this modality.")
		telemetry.RecordSpanError(span, err)
		return chatTarget{}, err
	}
	if err := enforcePricingPolicy(c, model.ID); err != nil {
		telemetry.RecordSpanError(span, err)
		return chatTarget{}, err
	}
	span.SetAttributes(
		attribute.String("polaris.model", model.ID),
		attribute.String("polaris.provider", model.Provider),
	)
	return chatTarget{adapter: adapter, model: model, resolution: resolution}, nil
}

func (h *ChatHandler) resolveFallbackTargets(c *gin.Context, registry *provider.Registry, auth middleware.AuthContext, primaryModelID string, requiredCapabilities []modality.Capability) []chatTarget {
	var targets []chatTarget
	for _, candidate := range registry.GetFallbacks(primaryModelID) {
		target, err := h.resolveChatTarget(c, registry, auth, candidate, nil, requiredCapabilities)
		if err != nil {
			continue
		}
		targets = append(targets, target)
	}
	return targets
}

func (h *ChatHandler) registry(c *gin.Context) *provider.Registry {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil {
		return nil
	}
	return snapshot.Registry
}
