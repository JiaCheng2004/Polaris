package handler

import (
	"net/http"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/gin-gonic/gin"
)

type TranslationHandler struct {
	runtime *gwruntime.Holder
}

func NewTranslationHandler(runtime *gwruntime.Holder) *TranslationHandler {
	return &TranslationHandler{runtime: runtime}
}

func (h *TranslationHandler) Translate(c *gin.Context) {
	var req modality.TranslationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if err := validateTranslationRequest(&req); err != nil {
		httputil.WriteError(c, err)
		return
	}

	registry := h.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable."))
		return
	}

	auth := middleware.GetAuthContext(c)
	resolved, err := resolveEndpointModel(c, registry, auth, req.Model, req.Routing, modality.ModalityTranslation)
	if err != nil {
		writeModalityTargetError(c, err, "translations")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model
	adapter, _, err := registry.GetTranslationAdapter(model.ID)
	if err != nil {
		writeModalityTargetError(c, err, "translations")
		return
	}

	req.Model = model.ID
	response, err := adapter.Translate(c.Request.Context(), &req)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	response.Model = model.ID
	response.Usage = normalizeUsage(response.Usage)
	middleware.SetRequestOutcome(c, middleware.RequestOutcome{
		Model:            model.ID,
		Provider:         model.Provider,
		Modality:         modality.ModalityTranslation,
		InterfaceFamily:  "translations",
		StatusCode:       http.StatusOK,
		PromptTokens:     response.Usage.PromptTokens,
		CompletionTokens: response.Usage.CompletionTokens,
		TotalTokens:      response.Usage.TotalTokens,
		TokenSource:      response.Usage.Source,
	})
	c.JSON(http.StatusOK, response)
}

func (h *TranslationHandler) registry(c *gin.Context) *provider.Registry {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil {
		return nil
	}
	return snapshot.Registry
}

func validateTranslationRequest(req *modality.TranslationRequest) error {
	if strings.TrimSpace(req.Model) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	if req.Input.Empty() {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_input", "input", "Field 'input' is required.")
	}
	values := req.Input.Values()
	if len(values) > 16 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "too_many_inputs", "input", "Field 'input' must contain at most 16 items.")
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_input", "input", "Field 'input' must not contain empty strings.")
		}
	}
	if strings.TrimSpace(req.TargetLanguage) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_target_language", "target_language", "Field 'target_language' is required.")
	}
	return nil
}
