package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/gateway/telemetry"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
)

func resolveEndpointModel(c *gin.Context, registry *provider.Registry, auth middleware.AuthContext, name string, routing *modality.RoutingOptions, requiredModality modality.Modality, requiredCapabilities ...modality.Capability) (provider.Resolution, error) {
	ctx := c.Request.Context()
	_, span := telemetry.StartInternalSpan(ctx, "policy.resolve_model",
		attribute.String("polaris.requested_model", name),
		attribute.String("polaris.modality", string(requiredModality)),
	)
	defer span.End()

	if err := validateRoutingOptions(routing); err != nil {
		telemetry.RecordSpanError(span, err)
		return provider.Resolution{}, err
	}

	resolution, err := registry.RequireResolvedModel(name, requiredModality, routing, requiredCapabilities...)
	if err != nil {
		telemetry.RecordSpanError(span, err)
		return provider.Resolution{}, err
	}
	model := resolution.Model
	if !middleware.ScopeAllowed(auth.AllowedModels, auth.PolicyModels, model.ID) {
		err := httputil.NewError(http.StatusForbidden, "permission_error", "model_not_allowed", "model", "API key is not permitted to use this model.")
		telemetry.RecordSpanError(span, err)
		return provider.Resolution{}, err
	}
	if !middleware.ModalityScopeAllowed(auth.AllowedModalities, auth.PolicyModalities, requiredModality) {
		err := httputil.NewError(http.StatusForbidden, "permission_error", "modality_not_allowed", "model", "API key is not permitted to use this modality.")
		telemetry.RecordSpanError(span, err)
		return provider.Resolution{}, err
	}
	if err := enforcePricingPolicy(c, model.ID); err != nil {
		telemetry.RecordSpanError(span, err)
		return provider.Resolution{}, err
	}
	span.SetAttributes(
		attribute.String("polaris.model", model.ID),
		attribute.String("polaris.provider", model.Provider),
	)
	return resolution, nil
}

// enforcePricingPolicy is invoked from resolveEndpointModel above and from
// conversation_execution.go (the chat path that does not flow through
// resolveEndpointModel). Every other model-resolving entrypoint routes
// through resolveEndpointModel, so those two call sites cover the gateway.
func enforcePricingPolicy(c *gin.Context, modelID string) error {
	snapshot, ok := middleware.GetRuntimeSnapshot(c)
	if !ok || snapshot == nil || snapshot.Config == nil || !snapshot.Config.Pricing.FailOnMissing {
		return nil
	}
	if snapshot.Pricing == nil {
		return httputil.NewError(http.StatusBadRequest, "model_not_priced", "pricing_unavailable", "model", "Pricing catalog is unavailable.")
	}
	if _, ok := snapshot.Pricing.Lookup(modelID); !ok {
		return httputil.NewError(http.StatusBadRequest, "model_not_priced", "model_not_priced", "model", "Requested model does not have a pricing entry.")
	}
	return nil
}

func writeModalityTargetError(c *gin.Context, err error, endpointName string) {
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
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "modality_mismatch", "model", fmt.Sprintf("Requested model does not support the %s endpoint.", endpointName)))
	case errors.Is(err, provider.ErrCapabilityMissing):
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "capability_missing", "model", fmt.Sprintf("Requested model does not support the required capability for %s.", endpointName)))
	case errors.Is(err, provider.ErrAdapterMissing):
		httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "adapter_unavailable", "model", "Requested model is configured but not available in this runtime build."))
	default:
		httputil.WriteError(c, err)
	}
}

func readMultipartFile(header *multipart.FileHeader) ([]byte, string, error) {
	file, err := header.Open()
	if err != nil {
		return nil, "", err
	}
	defer func() {
		_ = file.Close()
	}()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, "", err
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return data, contentType, nil
}

func audioContentType(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "ogg", "opus":
		return "audio/ogg"
	case "aac":
		return "audio/aac"
	case "flac":
		return "audio/flac"
	case "pcm":
		return "audio/pcm"
	default:
		return "application/octet-stream"
	}
}

func transcriptionContentType(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "json":
		return "application/json"
	case "text":
		return "text/plain; charset=utf-8"
	case "srt":
		return "application/x-subrip; charset=utf-8"
	case "vtt":
		return "text/vtt; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

func fileFormatFromName(filename string) string {
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
}

func containsString(values []string, candidate string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
}

func validateRoutingOptions(routing *modality.RoutingOptions) error {
	if routing == nil || routing.Empty() {
		return nil
	}
	for _, capability := range routing.Capabilities {
		if !capability.Valid() {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_routing_capability", "routing.capabilities", fmt.Sprintf("Routing capability %q is invalid.", capability))
		}
	}
	for _, status := range routing.Statuses {
		switch strings.TrimSpace(status) {
		case "ga", "preview", "experimental":
		default:
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_routing_status", "routing.statuses", fmt.Sprintf("Routing status %q is invalid.", status))
		}
	}
	for _, class := range routing.VerificationClasses {
		switch strings.TrimSpace(class) {
		case "strict", "opt_in", "skipped":
		default:
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_routing_verification_class", "routing.verification_classes", fmt.Sprintf("Routing verification class %q is invalid.", class))
		}
	}
	switch strings.TrimSpace(routing.CostTier) {
	case "", modality.CostTierLow, modality.CostTierBalanced, modality.CostTierPremium:
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_routing_cost_tier", "routing.cost_tier", "Field 'routing.cost_tier' must be low, balanced, or premium.")
	}
	switch strings.TrimSpace(routing.LatencyTier) {
	case "", modality.LatencyTierFast, modality.LatencyTierBalanced, modality.LatencyTierBestQuality:
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_routing_latency_tier", "routing.latency_tier", "Field 'routing.latency_tier' must be fast, balanced, or best_quality.")
	}
	for _, value := range append(append([]string(nil), routing.Providers...), append(append([]string(nil), routing.ExcludeProviders...), routing.Prefer...)...) {
		if strings.TrimSpace(value) == "" {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_routing_value", "routing", "Routing values must not contain empty strings.")
		}
	}
	return nil
}

func parseRoutingFormValue(raw string) (*modality.RoutingOptions, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var routing modality.RoutingOptions
	if err := json.Unmarshal([]byte(raw), &routing); err != nil {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_routing", "routing", "Form field 'routing' must be valid JSON.")
	}
	if routing.Empty() {
		return nil, nil
	}
	if err := validateRoutingOptions(&routing); err != nil {
		return nil, err
	}
	return &routing, nil
}

func applyResolvedRoutingHeaders(c *gin.Context, resolution provider.Resolution) {
	if resolution.ResolvedModel != "" {
		c.Header("X-Polaris-Resolved-Model", resolution.ResolvedModel)
	}
	if resolution.ResolvedProvider != "" {
		c.Header("X-Polaris-Resolved-Provider", resolution.ResolvedProvider)
	}
	if resolution.Mode != "" {
		c.Header("X-Polaris-Routing-Mode", resolution.Mode)
	}
}
