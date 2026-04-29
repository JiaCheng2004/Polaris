package handler

import (
	"net/http"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	cachepkg "github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/gin-gonic/gin"
)

type EmbedHandler struct {
	runtime *gwruntime.Holder
	cache   cachepkg.Cache
}

func NewEmbedHandler(runtime *gwruntime.Holder, cache cachepkg.Cache) *EmbedHandler {
	return &EmbedHandler{runtime: runtime, cache: cache}
}

func (h *EmbedHandler) Create(c *gin.Context) {
	var req modality.EmbedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if err := validateEmbedRequest(&req); err != nil {
		httputil.WriteError(c, err)
		return
	}

	registry := h.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable."))
		return
	}

	auth := middleware.GetAuthContext(c)
	resolved, err := resolveEndpointModel(c, registry, auth, req.Model, req.Routing, modality.ModalityEmbed)
	if err != nil {
		writeModalityTargetError(c, err, "embeddings")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model
	if req.Dimensions != nil && model.Dimensions > 0 && *req.Dimensions > model.Dimensions {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_dimensions", "dimensions", "Requested dimensions exceed the model's declared dimensions."))
		return
	}

	adapter, _, err := registry.GetEmbedAdapter(model.ID)
	if err != nil {
		writeModalityTargetError(c, err, "embeddings")
		return
	}

	cacheCtl := newResponseCache(c, h.runtime, h.cache)
	cacheKey := exactCacheKey("embeddings", model.ID, req)
	if cacheCtl != nil && cacheCtl.tryExact(c, cacheKey, model, modality.ModalityEmbed) {
		return
	}
	if cacheCtl == nil {
		c.Header(cacheHeader, "bypass")
	}

	req.Model = model.ID
	response, err := adapter.Embed(c.Request.Context(), &req)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	response.Model = model.ID
	response.Usage = normalizeEmbedUsage(response.Usage)
	middleware.SetRequestOutcome(c, middleware.RequestOutcome{
		Model:        model.ID,
		Provider:     model.Provider,
		Modality:     modality.ModalityEmbed,
		StatusCode:   http.StatusOK,
		PromptTokens: response.Usage.PromptTokens,
		TotalTokens:  response.Usage.TotalTokens,
		TokenSource:  countsTokenSource(response.Usage.PromptTokens, 0, response.Usage.TotalTokens),
	})
	if cacheCtl != nil {
		cacheCtl.storeJSON(c, cacheKey, http.StatusOK, response)
	}
	c.JSON(http.StatusOK, response)
}

func (h *EmbedHandler) registry(c *gin.Context) *provider.Registry {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil {
		return nil
	}
	return snapshot.Registry
}

func validateEmbedRequest(req *modality.EmbedRequest) error {
	if strings.TrimSpace(req.Model) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	if req.Input.Empty() {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_input", "input", "Field 'input' is required.")
	}
	for _, value := range req.Input.Values() {
		if strings.TrimSpace(value) == "" {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_input", "input", "Field 'input' must not contain empty strings.")
		}
	}
	if req.Dimensions != nil && *req.Dimensions <= 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_dimensions", "dimensions", "Field 'dimensions' must be greater than zero.")
	}
	switch req.EncodingFormat {
	case "", "float", "base64":
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_encoding_format", "encoding_format", "Field 'encoding_format' must be 'float' or 'base64'.")
	}
	if req.EncodingFormat == "" {
		req.EncodingFormat = "float"
	}
	return nil
}
