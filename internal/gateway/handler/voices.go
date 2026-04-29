package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/gin-gonic/gin"
)

type VoicesHandler struct {
	runtime *gwruntime.Holder
	store   store.Store
}

func NewVoicesHandler(runtime *gwruntime.Holder, appStore store.Store) *VoicesHandler {
	return &VoicesHandler{runtime: runtime, store: appStore}
}

func (h *VoicesHandler) List(c *gin.Context) {
	req, err := parseVoiceCatalogRequest(c)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}

	registry := h.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "", "Model registry is unavailable."))
		return
	}

	switch req.Scope {
	case "config":
		items, err := registry.ListConfiguredVoices(req.Provider, req.Model)
		if err != nil {
			writeVoicesTargetError(c, err)
			return
		}
		if req.Limit > 0 && len(items) > req.Limit {
			items = items[:req.Limit]
		}
		c.JSON(http.StatusOK, modality.VoiceCatalogResponse{
			Object: "list",
			Scope:  "config",
			Data:   items,
		})
	case "provider":
		target, err := h.resolveVoiceProvider(c, req.Provider, req.Model)
		if err != nil {
			httputil.WriteError(c, err)
			return
		}
		req.Provider = target.Provider
		req.Model = target.ModelID
		req.ConfiguredVoiceIDs = append([]string(nil), target.ConfiguredVoiceIDs...)

		resp, err := h.providerVoiceList(c, registry, &req)
		if err != nil {
			httputil.WriteError(c, err)
			return
		}
		archived, err := h.archivedVoicesMap(c, req.Provider, req.Model)
		if err != nil {
			httputil.WriteError(c, err)
			return
		}
		resp.Data = filterArchivedVoiceItems(resp.Data, archived, req.IncludeArchived)
		c.JSON(http.StatusOK, resp)
	default:
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_scope", "scope", "Field 'scope' must be either 'config' or 'provider'."))
	}
}

func (h *VoicesHandler) Get(c *gin.Context) {
	registry := h.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "", "Model registry is unavailable."))
		return
	}

	scope := modality.NormalizeVoiceCatalogScope(c.DefaultQuery("scope", "provider"))
	if scope == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_scope", "scope", "Field 'scope' must be either 'config' or 'provider'."))
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_voice_id", "id", "Voice id is required."))
		return
	}

	if scope == "config" {
		providerName := strings.TrimSpace(c.Query("provider"))
		modelName := strings.TrimSpace(c.Query("model"))
		items, err := registry.ListConfiguredVoices(providerName, modelName)
		if err != nil {
			writeVoicesTargetError(c, err)
			return
		}
		for _, item := range items {
			if strings.EqualFold(item.ID, id) {
				c.JSON(http.StatusOK, item)
				return
			}
		}
		httputil.WriteError(c, httputil.NewError(http.StatusNotFound, "invalid_request_error", "voice_not_found", "id", "Voice was not found."))
		return
	}

	target, err := h.resolveVoiceProvider(c, c.Query("provider"), c.Query("model"))
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	voiceType := modality.NormalizeVoiceCatalogType(c.DefaultQuery("type", "all"))
	if voiceType == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_voice_type", "type", "Field 'type' must be one of 'builtin', 'custom', or 'all'."))
		return
	}

	item, err := h.providerVoiceGet(c, registry, target, id, voiceType)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	archived, err := h.isVoiceArchived(c, target.Provider, target.ModelID, item.ID)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if archived && !includeArchived(c) {
		httputil.WriteError(c, httputil.NewError(http.StatusNotFound, "invalid_request_error", "voice_not_found", "id", "Voice was not found."))
		return
	}
	annotateVoiceArchived(item, archived)
	c.JSON(http.StatusOK, item)
}

func (h *VoicesHandler) CreateClone(c *gin.Context) {
	var req modality.VoiceCloneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required."))
		return
	}
	if strings.TrimSpace(req.Audio) == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_audio", "audio", "Field 'audio' is required."))
		return
	}

	registry := h.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "", "Model registry is unavailable."))
		return
	}
	auth := middleware.GetAuthContext(c)
	resolved, err := resolveEndpointModel(c, registry, auth, req.Model, req.Routing, modality.ModalityVoice, modality.CapabilityVoiceCloning)
	if err != nil {
		writeModalityTargetError(c, err, "voice cloning")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model
	adapter, err := h.voiceAssetAdapter(registry, model.Provider)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}

	req.Model = model.ID
	item, err := adapter.CreateClone(c.Request.Context(), &req)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	ensureVoiceModel(item, model.ID)
	c.JSON(http.StatusOK, item)
}

func (h *VoicesHandler) CreateDesign(c *gin.Context) {
	var req modality.VoiceDesignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required."))
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_text", "text", "Field 'text' is required."))
		return
	}

	registry := h.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "", "Model registry is unavailable."))
		return
	}
	auth := middleware.GetAuthContext(c)
	resolved, err := resolveEndpointModel(c, registry, auth, req.Model, req.Routing, modality.ModalityVoice, modality.CapabilityVoiceDesign)
	if err != nil {
		writeModalityTargetError(c, err, "voice design")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model
	adapter, err := h.voiceAssetAdapter(registry, model.Provider)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}

	req.Model = model.ID
	item, err := adapter.CreateDesign(c.Request.Context(), &req)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	ensureVoiceModel(item, model.ID)
	c.JSON(http.StatusOK, item)
}

func (h *VoicesHandler) Retrain(c *gin.Context) {
	var req modality.VoiceCloneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required."))
		return
	}
	if strings.TrimSpace(req.Audio) == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_audio", "audio", "Field 'audio' is required."))
		return
	}
	req.VoiceID = strings.TrimSpace(c.Param("id"))
	if req.VoiceID == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_voice_id", "id", "Voice id is required."))
		return
	}

	registry := h.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "", "Model registry is unavailable."))
		return
	}
	auth := middleware.GetAuthContext(c)
	resolved, err := resolveEndpointModel(c, registry, auth, req.Model, req.Routing, modality.ModalityVoice, modality.CapabilityVoiceCloning)
	if err != nil {
		writeModalityTargetError(c, err, "voice retraining")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model
	adapter, err := h.voiceAssetAdapter(registry, model.Provider)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}

	req.Model = model.ID
	item, err := adapter.RetrainVoice(c.Request.Context(), &req)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	ensureVoiceModel(item, model.ID)
	c.JSON(http.StatusOK, item)
}

func (h *VoicesHandler) Activate(c *gin.Context) {
	var body struct {
		Model    string `json:"model"`
		Provider string `json:"provider,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}

	registry := h.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "", "Model registry is unavailable."))
		return
	}
	target, err := h.resolveVoiceProvider(c, body.Provider, body.Model)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if target.ModelID == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required."))
		return
	}
	if !containsCapability(target.Model.Capabilities, modality.CapabilityVoiceCloning) && !containsCapability(target.Model.Capabilities, modality.CapabilityVoiceDesign) {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "capability_missing", "model", "Requested model does not support voice asset activation."))
		return
	}

	adapter, err := h.voiceAssetAdapter(registry, target.Provider)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	item, err := adapter.ActivateVoice(c.Request.Context(), &modality.VoiceLookupRequest{
		Provider: target.Provider,
		Model:    target.ModelID,
		ID:       strings.TrimSpace(c.Param("id")),
	})
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	ensureVoiceModel(item, target.ModelID)
	c.JSON(http.StatusOK, item)
}

func (h *VoicesHandler) Delete(c *gin.Context) {
	registry := h.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "", "Model registry is unavailable."))
		return
	}
	target, err := h.resolveVoiceProvider(c, c.Query("provider"), c.Query("model"))
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	adapter, err := h.voiceAssetAdapter(registry, target.Provider)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if err := adapter.DeleteVoice(c.Request.Context(), &modality.VoiceLookupRequest{
		Provider: target.Provider,
		Model:    target.ModelID,
		ID:       strings.TrimSpace(c.Param("id")),
	}); err != nil {
		httputil.WriteError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *VoicesHandler) Archive(c *gin.Context) {
	registry := h.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "", "Model registry is unavailable."))
		return
	}
	if h.store == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "store_unavailable", "", "Voice archive store is unavailable."))
		return
	}

	target, err := h.resolveVoiceProvider(c, c.Query("provider"), c.Query("model"))
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	voiceType := modality.NormalizeVoiceCatalogType(c.DefaultQuery("type", "all"))
	if voiceType == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_voice_type", "type", "Field 'type' must be one of 'builtin', 'custom', or 'all'."))
		return
	}
	item, err := h.providerVoiceGet(c, registry, target, strings.TrimSpace(c.Param("id")), voiceType)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if err := h.store.ArchiveVoice(c.Request.Context(), store.ArchivedVoice{
		Provider: target.Provider,
		Model:    target.ModelID,
		VoiceID:  item.ID,
	}); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "provider_error", "voice_archive_failed", "", "Voice archive could not be saved."))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *VoicesHandler) Unarchive(c *gin.Context) {
	if h.store == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "store_unavailable", "", "Voice archive store is unavailable."))
		return
	}
	target, err := h.resolveVoiceProvider(c, c.Query("provider"), c.Query("model"))
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if err := h.store.UnarchiveVoice(c.Request.Context(), target.Provider, target.ModelID, strings.TrimSpace(c.Param("id"))); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httputil.WriteError(c, httputil.NewError(http.StatusNotFound, "invalid_request_error", "voice_not_found", "id", "Voice archive was not found."))
			return
		}
		httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "provider_error", "voice_unarchive_failed", "", "Voice archive could not be removed."))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *VoicesHandler) registry(c *gin.Context) *provider.Registry {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil {
		return nil
	}
	return snapshot.Registry
}

func parseVoiceCatalogRequest(c *gin.Context) (modality.VoiceCatalogRequest, error) {
	req := modality.VoiceCatalogRequest{
		Provider:        strings.TrimSpace(c.Query("provider")),
		Model:           strings.TrimSpace(c.Query("model")),
		Scope:           modality.NormalizeVoiceCatalogScope(c.DefaultQuery("scope", "config")),
		Type:            modality.NormalizeVoiceCatalogType(c.DefaultQuery("type", "builtin")),
		State:           strings.TrimSpace(c.Query("state")),
		IncludeArchived: includeArchived(c),
	}
	if req.Scope == "" {
		return modality.VoiceCatalogRequest{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_scope", "scope", "Field 'scope' must be either 'config' or 'provider'.")
	}
	if req.Type == "" {
		return modality.VoiceCatalogRequest{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_voice_type", "type", "Field 'type' must be one of 'builtin', 'custom', or 'all'.")
	}
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit <= 0 {
			return modality.VoiceCatalogRequest{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_limit", "limit", "Field 'limit' must be a positive integer.")
		}
		req.Limit = limit
	}
	return req, nil
}

func writeVoicesTargetError(c *gin.Context, err error) {
	switch {
	case err == nil:
		return
	case errors.Is(err, provider.ErrUnknownModel):
		httputil.WriteError(c, httputil.NewError(http.StatusNotFound, "invalid_request_error", "unknown_model", "model", "Model was not found."))
	case errors.Is(err, provider.ErrUnknownAlias):
		httputil.WriteError(c, httputil.NewError(http.StatusNotFound, "invalid_request_error", "unknown_model", "model", "Model alias was not found."))
	case errors.Is(err, provider.ErrAdapterMissing):
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "provider_catalog_unavailable", "provider", "No voice catalog adapter is available for the selected provider."))
	default:
		httputil.WriteError(c, err)
	}
}

type resolvedVoiceProvider struct {
	Provider           string
	ModelID            string
	Model              provider.Model
	ConfiguredVoiceIDs []string
}

func (h *VoicesHandler) resolveVoiceProvider(c *gin.Context, providerName string, modelName string) (resolvedVoiceProvider, error) {
	registry := h.registry(c)
	if registry == nil {
		return resolvedVoiceProvider{}, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "", "Model registry is unavailable.")
	}

	providerName = strings.TrimSpace(providerName)
	modelName = strings.TrimSpace(modelName)
	if modelName != "" {
		auth := middleware.GetAuthContext(c)
		resolved, err := resolveEndpointModel(c, registry, auth, modelName, nil, modality.ModalityVoice)
		if err != nil {
			return resolvedVoiceProvider{}, translateVoiceTargetError(err)
		}
		model := resolved.Model
		if providerName != "" && providerName != model.Provider {
			return resolvedVoiceProvider{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "provider_model_mismatch", "provider", "Field 'provider' must match the selected model provider.")
		}
		return resolvedVoiceProvider{
			Provider:           model.Provider,
			ModelID:            model.ID,
			Model:              model,
			ConfiguredVoiceIDs: append([]string(nil), model.Voices...),
		}, nil
	}
	if providerName == "" {
		return resolvedVoiceProvider{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_provider", "provider", "Field 'provider' is required when model is not set.")
	}
	auth := middleware.GetAuthContext(c)
	if !middleware.ModalityScopeAllowed(auth.AllowedModalities, auth.PolicyModalities, modality.ModalityVoice) {
		return resolvedVoiceProvider{}, httputil.NewError(http.StatusForbidden, "permission_error", "modality_not_allowed", "model", "API key is not permitted to use this modality.")
	}
	return resolvedVoiceProvider{Provider: providerName}, nil
}

func (h *VoicesHandler) providerVoiceList(c *gin.Context, registry *provider.Registry, req *modality.VoiceCatalogRequest) (*modality.VoiceCatalogResponse, error) {
	switch req.Type {
	case "builtin":
		adapter, err := registry.GetVoiceCatalogAdapter(req.Provider)
		if err != nil {
			return nil, translateVoiceTargetError(err)
		}
		return adapter.ListVoices(c.Request.Context(), req)
	case "custom":
		adapter, err := h.voiceAssetAdapter(registry, req.Provider)
		if err != nil {
			return nil, err
		}
		return adapter.ListCustomVoices(c.Request.Context(), req)
	case "all":
		items := make([]modality.VoiceCatalogItem, 0)
		seen := map[string]struct{}{}
		assetAdapter, err := h.voiceAssetAdapter(registry, req.Provider)
		if err == nil {
			custom, err := assetAdapter.ListCustomVoices(c.Request.Context(), req)
			if err != nil {
				return nil, err
			}
			for _, item := range custom.Data {
				items = append(items, item)
				seen[strings.ToLower(strings.TrimSpace(item.ID))] = struct{}{}
			}
		}
		catalogAdapter, err := registry.GetVoiceCatalogAdapter(req.Provider)
		if err != nil {
			return nil, translateVoiceTargetError(err)
		}
		builtin, err := catalogAdapter.ListVoices(c.Request.Context(), req)
		if err != nil {
			return nil, err
		}
		for _, item := range builtin.Data {
			key := strings.ToLower(strings.TrimSpace(item.ID))
			if _, ok := seen[key]; ok {
				continue
			}
			items = append(items, item)
		}
		if req.Limit > 0 && len(items) > req.Limit {
			items = items[:req.Limit]
		}
		return &modality.VoiceCatalogResponse{
			Object:   "list",
			Scope:    "provider",
			Provider: req.Provider,
			Data:     items,
		}, nil
	default:
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_voice_type", "type", "Field 'type' must be one of 'builtin', 'custom', or 'all'.")
	}
}

func (h *VoicesHandler) providerVoiceGet(c *gin.Context, registry *provider.Registry, target resolvedVoiceProvider, id string, voiceType string) (*modality.VoiceCatalogItem, error) {
	switch voiceType {
	case "custom":
		adapter, err := h.voiceAssetAdapter(registry, target.Provider)
		if err != nil {
			return nil, err
		}
		item, err := adapter.GetVoice(c.Request.Context(), &modality.VoiceLookupRequest{
			Provider: target.Provider,
			Model:    target.ModelID,
			ID:       id,
		})
		if err != nil {
			return nil, err
		}
		ensureVoiceModel(item, target.ModelID)
		return item, nil
	case "builtin":
		req := &modality.VoiceCatalogRequest{
			Provider:           target.Provider,
			Model:              target.ModelID,
			Scope:              "provider",
			Type:               "builtin",
			ConfiguredVoiceIDs: append([]string(nil), target.ConfiguredVoiceIDs...),
		}
		resp, err := h.providerVoiceList(c, registry, req)
		if err != nil {
			return nil, err
		}
		for _, item := range resp.Data {
			if strings.EqualFold(item.ID, id) {
				cloned := item
				return &cloned, nil
			}
		}
		return nil, httputil.NewError(http.StatusNotFound, "invalid_request_error", "voice_not_found", "id", "Voice was not found.")
	case "all":
		item, err := h.providerVoiceGet(c, registry, target, id, "custom")
		if err == nil {
			return item, nil
		}
		if !isVoiceNotFound(err) {
			return nil, err
		}
		return h.providerVoiceGet(c, registry, target, id, "builtin")
	default:
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_voice_type", "type", "Field 'type' must be one of 'builtin', 'custom', or 'all'.")
	}
}

func (h *VoicesHandler) archivedVoicesMap(c *gin.Context, providerName string, modelName string) (map[string]struct{}, error) {
	if h.store == nil {
		return nil, nil
	}
	voices, err := h.store.ListArchivedVoices(c.Request.Context(), providerName, modelName)
	if err != nil {
		return nil, err
	}
	index := make(map[string]struct{}, len(voices))
	for _, voice := range voices {
		index[strings.ToLower(strings.TrimSpace(voice.VoiceID))] = struct{}{}
	}
	return index, nil
}

func (h *VoicesHandler) isVoiceArchived(c *gin.Context, providerName string, modelName string, voiceID string) (bool, error) {
	if h.store == nil {
		return false, nil
	}
	_, err := h.store.GetArchivedVoice(c.Request.Context(), providerName, modelName, voiceID)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	return false, err
}

func (h *VoicesHandler) voiceAssetAdapter(registry *provider.Registry, providerName string) (modality.VoiceAssetAdapter, error) {
	adapter, err := registry.GetVoiceAssetAdapter(providerName)
	if err != nil {
		return nil, translateVoiceTargetError(err)
	}
	return adapter, nil
}

func translateVoiceTargetError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, provider.ErrUnknownAlias):
		return httputil.NewError(http.StatusNotFound, "model_not_found", "unknown_alias", "model", "Model alias is not defined.")
	case errors.Is(err, provider.ErrUnknownModel):
		return httputil.NewError(http.StatusNotFound, "model_not_found", "unknown_model", "model", "Requested model is not registered.")
	case errors.Is(err, provider.ErrModalityMismatch):
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "modality_mismatch", "model", "Requested model does not support voice APIs.")
	case errors.Is(err, provider.ErrCapabilityMissing):
		return httputil.NewError(http.StatusBadRequest, "capability_not_supported", "capability_missing", "model", "Requested model does not support the required voice capability.")
	case errors.Is(err, provider.ErrAdapterMissing):
		return httputil.NewError(http.StatusBadGateway, "provider_error", "adapter_unavailable", "provider", "No voice adapter is available for the selected provider.")
	default:
		return err
	}
}

func ensureVoiceModel(item *modality.VoiceCatalogItem, modelID string) {
	if item == nil || strings.TrimSpace(modelID) == "" {
		return
	}
	if containsString(item.Models, modelID) {
		return
	}
	item.Models = append(item.Models, modelID)
}

func annotateVoiceArchived(item *modality.VoiceCatalogItem, archived bool) {
	if item == nil || !archived {
		return
	}
	if item.Metadata == nil {
		item.Metadata = make(map[string]any, 1)
	}
	item.Metadata["archived"] = true
}

func filterArchivedVoiceItems(items []modality.VoiceCatalogItem, archived map[string]struct{}, includeArchived bool) []modality.VoiceCatalogItem {
	if len(items) == 0 || len(archived) == 0 {
		return items
	}
	filtered := make([]modality.VoiceCatalogItem, 0, len(items))
	for _, item := range items {
		_, isArchived := archived[strings.ToLower(strings.TrimSpace(item.ID))]
		if isArchived && !includeArchived {
			continue
		}
		cloned := item
		annotateVoiceArchived(&cloned, isArchived)
		filtered = append(filtered, cloned)
	}
	return filtered
}

func includeArchived(c *gin.Context) bool {
	raw := strings.TrimSpace(c.Query("include_archived"))
	if raw == "" {
		return false
	}
	include, err := strconv.ParseBool(raw)
	return err == nil && include
}

func isVoiceNotFound(err error) bool {
	var apiErr *httputil.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code == "voice_not_found"
	}
	return false
}

func containsCapability(capabilities []modality.Capability, candidate modality.Capability) bool {
	for _, capability := range capabilities {
		if capability == candidate {
			return true
		}
	}
	return false
}
