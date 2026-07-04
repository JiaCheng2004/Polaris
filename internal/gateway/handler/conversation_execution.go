package handler

import (
	"net/http"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/obs"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
)

func (h *ChatHandler) completeWithFailover(c *gin.Context, primary chatTarget, fallbacks []chatTarget, req *modality.ChatRequest) (*modality.ChatResponse, middleware.RequestOutcome, string, error) {
	targets := append([]chatTarget{primary}, fallbacks...)
	response, _, outcome, fallbackModel, err := runFailover(h, c, targets, primary, req, "", 1, true, noAvailableProviderError(), invokeComplete, onCompleteSuccess)
	return response, outcome, fallbackModel, err
}

func (h *ChatHandler) resolveChatTarget(c *gin.Context, registry *provider.Registry, auth middleware.AuthContext, name string, routing *modality.RoutingOptions, requiredCapabilities []modality.Capability) (chatTarget, error) {
	ctx := c.Request.Context()
	_, span := obs.StartInternalSpan(ctx, "policy.resolve_chat_target",
		attribute.String("polaris.requested_model", name),
		attribute.String("polaris.modality", string(modality.ModalityChat)),
	)
	defer span.End()

	if err := validateRoutingOptions(routing); err != nil {
		obs.RecordSpanError(span, err)
		return chatTarget{}, err
	}
	resolution, err := registry.RequireResolvedModel(name, modality.ModalityChat, routing, requiredCapabilities...)
	if err != nil {
		obs.RecordSpanError(span, err)
		return chatTarget{}, err
	}
	model := resolution.Model
	adapter, _, err := registry.GetChatAdapter(model.ID)
	if err != nil {
		obs.RecordSpanError(span, err)
		return chatTarget{}, err
	}
	if !middleware.ScopeAllowed(auth.AllowedModels, auth.PolicyModels, model.ID) {
		err := httputil.NewError(http.StatusForbidden, "permission_error", "model_not_allowed", "model", "API key is not permitted to use this model.")
		obs.RecordSpanError(span, err)
		return chatTarget{}, err
	}
	if !middleware.ModalityScopeAllowed(auth.AllowedModalities, auth.PolicyModalities, modality.ModalityChat) {
		err := httputil.NewError(http.StatusForbidden, "permission_error", "modality_not_allowed", "model", "API key is not permitted to use this modality.")
		obs.RecordSpanError(span, err)
		return chatTarget{}, err
	}
	if err := enforcePricingPolicy(c, model.ID); err != nil {
		obs.RecordSpanError(span, err)
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
