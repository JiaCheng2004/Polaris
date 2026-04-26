package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/store"
	cachepkg "github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/gin-gonic/gin"
)

const (
	musicModeSync   = "sync"
	musicModeAsync  = "async"
	musicModeStream = "stream"

	musicOperationGenerate = "generate"
	musicOperationRemix    = "remix"
	musicOperationExtend   = "extend"
	musicOperationCover    = "cover"
	musicOperationInpaint  = "inpaint"
	musicOperationStems    = "stems"
	musicJobTTL            = 24 * time.Hour
)

type MusicHandler struct {
	runtime       *gwruntime.Holder
	cache         cachepkg.Cache
	requestLogger *store.AsyncRequestLogger

	cancelMu sync.Mutex
	cancels  map[string]context.CancelFunc
}

type musicGenerationEnvelope struct {
	Mode string `json:"mode,omitempty"`
	modality.MusicGenerationRequest
}

type musicEditEnvelope struct {
	Mode string `json:"mode,omitempty"`
	modality.MusicEditRequest
}

type musicStemEnvelope struct {
	Mode string `json:"mode,omitempty"`
	modality.MusicStemRequest
}

type musicJobRecord struct {
	Status    modality.MusicStatus `json:"status"`
	AssetBody string               `json:"asset_body,omitempty"`
}

func NewMusicHandler(runtime *gwruntime.Holder, cache cachepkg.Cache, requestLogger *store.AsyncRequestLogger) *MusicHandler {
	return &MusicHandler{
		runtime:       runtime,
		cache:         cache,
		requestLogger: requestLogger,
		cancels:       map[string]context.CancelFunc{},
	}
}

func (h *MusicHandler) Generate(c *gin.Context) {
	var env musicGenerationEnvelope
	if err := c.ShouldBindJSON(&env); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if err := validateMusicGenerationEnvelope(&env); err != nil {
		httputil.WriteError(c, err)
		return
	}

	registry := h.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable."))
		return
	}

	auth := middleware.GetAuthContext(c)
	required := []modality.Capability{modality.CapabilityMusicGeneration}
	if env.Mode == musicModeStream {
		required = append(required, modality.CapabilityMusicStreaming)
	}
	if env.Instrumental {
		required = append(required, modality.CapabilityInstrumental)
	}
	resolved, err := resolveEndpointModel(c.Request.Context(), registry, auth, env.Model, env.Routing, modality.ModalityMusic, required...)
	if err != nil {
		writeModalityTargetError(c, err, "music")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model
	if err := validateMusicModelRequest(model, env.OutputFormat, env.SampleRateHz, env.DurationMS); err != nil {
		httputil.WriteError(c, err)
		return
	}

	adapter, _, err := registry.GetMusicAdapter(model.ID)
	if err != nil {
		writeModalityTargetError(c, err, "music")
		return
	}

	req := env.MusicGenerationRequest
	req.Model = model.ID
	switch env.Mode {
	case musicModeSync:
		h.generateSync(c, model, adapter, &req)
	case musicModeStream:
		h.generateStream(c, model, adapter, &req)
	case musicModeAsync:
		h.generateAsync(c, model, adapter, auth, &req)
	default:
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_mode", "mode", "Field 'mode' must be 'sync', 'async', or 'stream'."))
	}
}

func (h *MusicHandler) Edit(c *gin.Context) {
	env, err := parseMusicEditEnvelope(c)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if err := validateMusicEditEnvelope(env); err != nil {
		httputil.WriteError(c, err)
		return
	}

	registry := h.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable."))
		return
	}

	auth := middleware.GetAuthContext(c)
	required := requiredMusicEditCapabilities(env)
	resolved, err := resolveEndpointModel(c.Request.Context(), registry, auth, env.Model, env.Routing, modality.ModalityMusic, required...)
	if err != nil {
		writeModalityTargetError(c, err, "music edits")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model
	if err := validateMusicModelRequest(model, env.OutputFormat, env.SampleRateHz, env.DurationMS); err != nil {
		httputil.WriteError(c, err)
		return
	}
	if err := h.resolveSourceMusicAsset(c, auth, &env.MusicEditRequest); err != nil {
		httputil.WriteError(c, err)
		return
	}

	adapter, _, err := registry.GetMusicAdapter(model.ID)
	if err != nil {
		writeModalityTargetError(c, err, "music edits")
		return
	}

	req := env.MusicEditRequest
	req.Model = model.ID
	switch env.Mode {
	case musicModeSync:
		h.editSync(c, model, adapter, &req)
	case musicModeStream:
		h.editStream(c, model, adapter, &req)
	case musicModeAsync:
		h.editAsync(c, model, adapter, auth, &req)
	default:
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_mode", "mode", "Field 'mode' must be 'sync', 'async', or 'stream'."))
	}
}

func (h *MusicHandler) Stems(c *gin.Context) {
	env, err := parseMusicStemEnvelope(c)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if err := validateMusicStemEnvelope(env); err != nil {
		httputil.WriteError(c, err)
		return
	}

	registry := h.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable."))
		return
	}

	auth := middleware.GetAuthContext(c)
	resolved, err := resolveEndpointModel(c.Request.Context(), registry, auth, env.Model, env.Routing, modality.ModalityMusic, modality.CapabilityMusicStems)
	if err != nil {
		writeModalityTargetError(c, err, "music stems")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model
	if err := validateMusicModelRequest(model, env.OutputFormat, 0, 0); err != nil {
		httputil.WriteError(c, err)
		return
	}
	if err := h.resolveMusicStemSource(c, auth, &env.MusicStemRequest); err != nil {
		httputil.WriteError(c, err)
		return
	}

	adapter, _, err := registry.GetMusicAdapter(model.ID)
	if err != nil {
		writeModalityTargetError(c, err, "music stems")
		return
	}

	req := env.MusicStemRequest
	req.Model = model.ID
	switch env.Mode {
	case musicModeSync:
		h.stemsSync(c, model, adapter, &req)
	case musicModeAsync:
		h.stemsAsync(c, model, adapter, auth, &req)
	default:
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_mode", "mode", "Field 'mode' must be 'sync' or 'async'."))
	}
}

func (h *MusicHandler) Lyrics(c *gin.Context) {
	var req modality.MusicLyricsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if err := validateMusicLyricsRequest(&req); err != nil {
		httputil.WriteError(c, err)
		return
	}

	registry := h.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable."))
		return
	}

	auth := middleware.GetAuthContext(c)
	resolved, err := resolveEndpointModel(c.Request.Context(), registry, auth, req.Model, req.Routing, modality.ModalityMusic, modality.CapabilityLyricsGeneration)
	if err != nil {
		writeModalityTargetError(c, err, "music lyrics")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model

	adapter, _, err := registry.GetMusicAdapter(model.ID)
	if err != nil {
		writeModalityTargetError(c, err, "music lyrics")
		return
	}

	cacheCtl := newResponseCache(c, h.runtime, h.cache)
	cacheKey := exactCacheKey("music-lyrics", model.ID, req)
	if cacheCtl != nil && cacheCtl.tryExact(c, cacheKey, model, modality.ModalityMusic) {
		return
	}
	if cacheCtl == nil {
		c.Header(cacheHeader, "bypass")
	}

	req.Model = model.ID
	response, err := adapter.GenerateLyrics(c.Request.Context(), &req)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	middleware.SetRequestOutcome(c, middleware.RequestOutcome{
		Model:      model.ID,
		Provider:   model.Provider,
		Modality:   modality.ModalityMusic,
		StatusCode: http.StatusOK,
	})
	if cacheCtl != nil {
		cacheCtl.storeJSON(c, cacheKey, http.StatusOK, response)
	}
	c.JSON(http.StatusOK, response)
}

func (h *MusicHandler) Plans(c *gin.Context) {
	var req modality.MusicPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if err := validateMusicPlanRequest(&req); err != nil {
		httputil.WriteError(c, err)
		return
	}

	registry := h.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable."))
		return
	}

	auth := middleware.GetAuthContext(c)
	resolved, err := resolveEndpointModel(c.Request.Context(), registry, auth, req.Model, req.Routing, modality.ModalityMusic, modality.CapabilityCompositionPlans)
	if err != nil {
		writeModalityTargetError(c, err, "music plans")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model

	adapter, _, err := registry.GetMusicAdapter(model.ID)
	if err != nil {
		writeModalityTargetError(c, err, "music plans")
		return
	}

	cacheCtl := newResponseCache(c, h.runtime, h.cache)
	cacheKey := exactCacheKey("music-plans", model.ID, req)
	if cacheCtl != nil && cacheCtl.tryExact(c, cacheKey, model, modality.ModalityMusic) {
		return
	}
	if cacheCtl == nil {
		c.Header(cacheHeader, "bypass")
	}

	req.Model = model.ID
	response, err := adapter.CreatePlan(c.Request.Context(), &req)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	middleware.SetRequestOutcome(c, middleware.RequestOutcome{
		Model:      model.ID,
		Provider:   model.Provider,
		Modality:   modality.ModalityMusic,
		StatusCode: http.StatusOK,
	})
	if cacheCtl != nil {
		cacheCtl.storeJSON(c, cacheKey, http.StatusOK, response)
	}
	c.JSON(http.StatusOK, response)
}

func (h *MusicHandler) GetJob(c *gin.Context) {
	record, _, err := h.resolveAuthorizedJobRecord(c)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	status := record.Status
	status.JobID = c.Param("id")
	decorateMusicStatus(c, &status)
	c.JSON(http.StatusOK, status)
}

func (h *MusicHandler) CancelJob(c *gin.Context) {
	record, token, err := h.resolveAuthorizedJobRecord(c)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	switch record.Status.Status {
	case "completed", "failed", "cancelled", "canceled":
		c.Status(http.StatusNoContent)
		return
	}

	h.cancelMu.Lock()
	cancel := h.cancels[token.CacheKey]
	delete(h.cancels, token.CacheKey)
	h.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}

	record.Status.Status = "cancelled"
	record.Status.CompletedAt = time.Now().Unix()
	record.Status.Error = &modality.MusicError{
		Type:    "invalid_request_error",
		Code:    "job_cancelled",
		Message: "Music job was cancelled.",
	}
	if err := h.storeMusicJob(c.Request.Context(), token.CacheKey, record); err != nil {
		httputil.WriteError(c, err)
		return
	}
	h.logAsyncMusicOutcome(c, token, record.Status)
	c.Status(http.StatusNoContent)
}

func (h *MusicHandler) GetJobContent(c *gin.Context) {
	record, _, err := h.resolveAuthorizedJobRecord(c)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if record.Status.Status != "completed" || record.Status.Result == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusConflict, "invalid_request_error", "job_not_ready", "id", "Music job is not ready yet."))
		return
	}
	if record.Status.ExpiresAt > 0 && time.Now().Unix() >= record.Status.ExpiresAt {
		httputil.WriteError(c, httputil.NewError(http.StatusGone, "invalid_request_error", "asset_expired", "id", "Music asset has expired."))
		return
	}
	if strings.TrimSpace(record.AssetBody) == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Music provider returned an empty asset."))
		return
	}
	payload, err := base64.StdEncoding.DecodeString(record.AssetBody)
	if err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Music asset is invalid."))
		return
	}
	contentType := "application/octet-stream"
	if record.Status.Result != nil && strings.TrimSpace(record.Status.Result.ContentType) != "" {
		contentType = record.Status.Result.ContentType
	}
	c.Data(http.StatusOK, contentType, payload)
}

func (h *MusicHandler) generateSync(c *gin.Context, model provider.Model, adapter modality.MusicAdapter, req *modality.MusicGenerationRequest) {
	cacheCtl := newResponseCache(c, h.runtime, h.cache)
	cacheKey := exactCacheKey("music-generate", model.ID, req)
	if cacheCtl != nil && cacheCtl.tryExact(c, cacheKey, model, modality.ModalityMusic) {
		return
	}
	if cacheCtl == nil {
		c.Header(cacheHeader, "bypass")
	}

	result, err := adapter.Generate(c.Request.Context(), req)
	if err != nil {
		httputil.WriteError(c, normalizeMusicTimeoutError(err))
		return
	}
	if result == nil || result.Asset == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Music provider returned an empty asset."))
		return
	}
	middleware.SetRequestOutcome(c, middleware.RequestOutcome{
		Model:      model.ID,
		Provider:   model.Provider,
		Modality:   modality.ModalityMusic,
		StatusCode: http.StatusOK,
	})
	if cacheCtl != nil {
		cacheCtl.storeRaw(c, cacheKey, http.StatusOK, result.Asset.ContentType, result.Asset.Data)
	}
	c.Data(http.StatusOK, firstNonEmptyMusicContentType(result.Asset.ContentType, req.OutputFormat), result.Asset.Data)
}

func (h *MusicHandler) generateStream(c *gin.Context, model provider.Model, adapter modality.MusicAdapter, req *modality.MusicGenerationRequest) {
	cacheCtl := newResponseCache(c, h.runtime, h.cache)
	if cacheCtl != nil {
		cacheCtl.markBypass(c)
	}
	stream, err := adapter.StreamGenerate(c.Request.Context(), req)
	if err != nil {
		httputil.WriteError(c, normalizeMusicTimeoutError(err))
		return
	}
	if stream == nil || stream.Body == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Music provider returned an empty stream."))
		return
	}
	defer func() {
		_ = stream.Body.Close()
	}()
	middleware.SetRequestOutcome(c, middleware.RequestOutcome{
		Model:      model.ID,
		Provider:   model.Provider,
		Modality:   modality.ModalityMusic,
		StatusCode: http.StatusOK,
	})
	c.DataFromReader(http.StatusOK, -1, firstNonEmptyMusicContentType(stream.ContentType, req.OutputFormat), stream.Body, nil)
}

func (h *MusicHandler) generateAsync(c *gin.Context, model provider.Model, adapter modality.MusicAdapter, auth middleware.AuthContext, req *modality.MusicGenerationRequest) {
	if h.cache == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusConflict, "invalid_request_error", "async_jobs_unavailable", "mode", "Async music jobs require a configured cache backend."))
		return
	}
	job, token, err := h.issueMusicJob(c, model, auth.KeyID, musicOperationGenerate)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	h.runAsyncMusicJob(c, token, func(ctx context.Context) (*modality.MusicOperationResult, error) {
		return adapter.Generate(ctx, req)
	})
	c.JSON(http.StatusOK, job)
}

func (h *MusicHandler) editSync(c *gin.Context, model provider.Model, adapter modality.MusicAdapter, req *modality.MusicEditRequest) {
	cacheCtl := newResponseCache(c, h.runtime, h.cache)
	cacheKey := exactCacheKey("music-edit", model.ID, map[string]any{
		"operation":         req.Operation,
		"prompt":            req.Prompt,
		"lyrics":            req.Lyrics,
		"plan":              req.Plan,
		"source_audio":      req.SourceAudio,
		"file":              hashBytes(req.File),
		"duration_ms":       req.DurationMS,
		"instrumental":      req.Instrumental,
		"seed":              req.Seed,
		"output_format":     req.OutputFormat,
		"sample_rate_hz":    req.SampleRateHz,
		"bitrate":           req.Bitrate,
		"store_for_editing": req.StoreForEditing,
		"sign_with_c2pa":    req.SignWithC2PA,
	})
	if cacheCtl != nil && cacheCtl.tryExact(c, cacheKey, model, modality.ModalityMusic) {
		return
	}
	if cacheCtl == nil {
		c.Header(cacheHeader, "bypass")
	}

	result, err := adapter.Edit(c.Request.Context(), req)
	if err != nil {
		httputil.WriteError(c, normalizeMusicTimeoutError(err))
		return
	}
	if result == nil || result.Asset == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Music provider returned an empty asset."))
		return
	}
	middleware.SetRequestOutcome(c, middleware.RequestOutcome{
		Model:      model.ID,
		Provider:   model.Provider,
		Modality:   modality.ModalityMusic,
		StatusCode: http.StatusOK,
	})
	if cacheCtl != nil {
		cacheCtl.storeRaw(c, cacheKey, http.StatusOK, result.Asset.ContentType, result.Asset.Data)
	}
	c.Data(http.StatusOK, firstNonEmptyMusicContentType(result.Asset.ContentType, req.OutputFormat), result.Asset.Data)
}

func (h *MusicHandler) editStream(c *gin.Context, model provider.Model, adapter modality.MusicAdapter, req *modality.MusicEditRequest) {
	cacheCtl := newResponseCache(c, h.runtime, h.cache)
	if cacheCtl != nil {
		cacheCtl.markBypass(c)
	}
	stream, err := adapter.StreamEdit(c.Request.Context(), req)
	if err != nil {
		httputil.WriteError(c, normalizeMusicTimeoutError(err))
		return
	}
	if stream == nil || stream.Body == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Music provider returned an empty stream."))
		return
	}
	defer func() {
		_ = stream.Body.Close()
	}()
	middleware.SetRequestOutcome(c, middleware.RequestOutcome{
		Model:      model.ID,
		Provider:   model.Provider,
		Modality:   modality.ModalityMusic,
		StatusCode: http.StatusOK,
	})
	c.DataFromReader(http.StatusOK, -1, firstNonEmptyMusicContentType(stream.ContentType, req.OutputFormat), stream.Body, nil)
}

func (h *MusicHandler) editAsync(c *gin.Context, model provider.Model, adapter modality.MusicAdapter, auth middleware.AuthContext, req *modality.MusicEditRequest) {
	if h.cache == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusConflict, "invalid_request_error", "async_jobs_unavailable", "mode", "Async music jobs require a configured cache backend."))
		return
	}
	job, token, err := h.issueMusicJob(c, model, auth.KeyID, strings.ToLower(strings.TrimSpace(req.Operation)))
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	h.runAsyncMusicJob(c, token, func(ctx context.Context) (*modality.MusicOperationResult, error) {
		return adapter.Edit(ctx, req)
	})
	c.JSON(http.StatusOK, job)
}

func (h *MusicHandler) stemsSync(c *gin.Context, model provider.Model, adapter modality.MusicAdapter, req *modality.MusicStemRequest) {
	cacheCtl := newResponseCache(c, h.runtime, h.cache)
	cacheKey := exactCacheKey("music-stems", model.ID, map[string]any{
		"source_audio":   req.SourceAudio,
		"file":           hashBytes(req.File),
		"stem_variant":   req.StemVariant,
		"output_format":  req.OutputFormat,
		"sign_with_c2pa": req.SignWithC2PA,
	})
	if cacheCtl != nil && cacheCtl.tryExact(c, cacheKey, model, modality.ModalityMusic) {
		return
	}
	if cacheCtl == nil {
		c.Header(cacheHeader, "bypass")
	}

	result, err := adapter.SeparateStems(c.Request.Context(), req)
	if err != nil {
		httputil.WriteError(c, normalizeMusicTimeoutError(err))
		return
	}
	if result == nil || result.Asset == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Music provider returned an empty asset."))
		return
	}
	middleware.SetRequestOutcome(c, middleware.RequestOutcome{
		Model:      model.ID,
		Provider:   model.Provider,
		Modality:   modality.ModalityMusic,
		StatusCode: http.StatusOK,
	})
	if cacheCtl != nil {
		cacheCtl.storeRaw(c, cacheKey, http.StatusOK, result.Asset.ContentType, result.Asset.Data)
	}
	c.Data(http.StatusOK, firstNonEmptyMusicContentType(result.Asset.ContentType, req.OutputFormat), result.Asset.Data)
}

func (h *MusicHandler) stemsAsync(c *gin.Context, model provider.Model, adapter modality.MusicAdapter, auth middleware.AuthContext, req *modality.MusicStemRequest) {
	if h.cache == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusConflict, "invalid_request_error", "async_jobs_unavailable", "mode", "Async music jobs require a configured cache backend."))
		return
	}
	job, token, err := h.issueMusicJob(c, model, auth.KeyID, musicOperationStems)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	h.runAsyncMusicJob(c, token, func(ctx context.Context) (*modality.MusicOperationResult, error) {
		return adapter.SeparateStems(ctx, req)
	})
	c.JSON(http.StatusOK, job)
}

func (h *MusicHandler) issueMusicJob(c *gin.Context, model provider.Model, keyID string, operation string) (*modality.MusicJob, *musicJobToken, error) {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil {
		return nil, nil, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "", "Runtime configuration is unavailable.")
	}
	cacheKey, err := newMusicJobCacheKey()
	if err != nil {
		return nil, nil, httputil.NewError(http.StatusInternalServerError, "internal_error", "job_id_encoding_failed", "", "Unable to issue music job id.")
	}
	expiresAt := time.Now().Add(musicJobTTL).Unix()
	jobID, err := signMusicJobID(snapshot, model, operation, cacheKey, keyID, expiresAt)
	if err != nil {
		return nil, nil, err
	}
	record := musicJobRecord{
		Status: modality.MusicStatus{
			JobID:     jobID,
			Status:    "queued",
			Model:     model.ID,
			Operation: operation,
			CreatedAt: time.Now().Unix(),
			ExpiresAt: expiresAt,
		},
	}
	if err := h.storeMusicJob(c.Request.Context(), cacheKey, record); err != nil {
		return nil, nil, err
	}
	token := &musicJobToken{
		Version:   1,
		Provider:  model.Provider,
		Model:     model.ID,
		Operation: operation,
		CacheKey:  cacheKey,
		KeyID:     keyID,
		ExpiresAt: expiresAt,
	}
	return &modality.MusicJob{
		JobID:     jobID,
		Status:    "queued",
		Model:     model.ID,
		Operation: operation,
	}, token, nil
}

func (h *MusicHandler) runAsyncMusicJob(c *gin.Context, token *musicJobToken, fn func(context.Context) (*modality.MusicOperationResult, error)) {
	if token == nil {
		return
	}
	runCtx, cancel := context.WithCancel(context.Background())
	h.cancelMu.Lock()
	h.cancels[token.CacheKey] = cancel
	h.cancelMu.Unlock()

	go func() {
		defer cancel()
		defer func() {
			h.cancelMu.Lock()
			delete(h.cancels, token.CacheKey)
			h.cancelMu.Unlock()
		}()

		stateCtx := context.Background()
		record, err := h.getMusicJob(stateCtx, token.CacheKey)
		if err != nil {
			return
		}
		record.Status.Status = "processing"
		_ = h.storeMusicJob(stateCtx, token.CacheKey, record)

		result, err := fn(runCtx)
		if err != nil {
			record.Status.Status = "failed"
			record.Status.CompletedAt = time.Now().Unix()
			record.Status.Error = musicErrorFromErr(normalizeMusicTimeoutError(err))
			if runCtx.Err() == context.Canceled {
				record.Status.Status = "cancelled"
				record.Status.Error = &modality.MusicError{
					Type:    "invalid_request_error",
					Code:    "job_cancelled",
					Message: "Music job was cancelled.",
				}
			}
			if storeErr := h.storeMusicJob(stateCtx, token.CacheKey, record); storeErr == nil {
				h.logAsyncMusicOutcome(nil, token, record.Status)
			}
			return
		}

		record.Status.Status = "completed"
		record.Status.CompletedAt = time.Now().Unix()
		record.Status.Result = buildMusicResult(result)
		record.AssetBody = encodeMusicAsset(result)
		if storeErr := h.storeMusicJob(stateCtx, token.CacheKey, record); storeErr == nil {
			h.logAsyncMusicOutcome(nil, token, record.Status)
		}
	}()
}

func (h *MusicHandler) resolveAuthorizedJobRecord(c *gin.Context) (musicJobRecord, *musicJobToken, error) {
	registry := h.registry(c)
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if registry == nil || snapshot == nil {
		return musicJobRecord{}, nil, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable.")
	}

	token, err := parseMusicJobID(snapshot, c.Param("id"))
	if err != nil {
		return musicJobRecord{}, nil, err
	}
	auth := middleware.GetAuthContext(c)
	if auth.KeyID != token.KeyID {
		return musicJobRecord{}, nil, httputil.NewError(http.StatusForbidden, "permission_error", "job_not_owned", "id", "Music job was created by a different API key.")
	}
	resolved, err := resolveEndpointModel(c.Request.Context(), registry, auth, token.Model, nil, modality.ModalityMusic)
	if err != nil {
		return musicJobRecord{}, nil, err
	}
	model := resolved.Model
	if model.Provider != token.Provider {
		return musicJobRecord{}, nil, invalidMusicJobIDError()
	}
	record, err := h.getMusicJob(c.Request.Context(), token.CacheKey)
	if err != nil {
		return musicJobRecord{}, nil, err
	}
	return record, &token, nil
}

func (h *MusicHandler) resolveSourceMusicAsset(c *gin.Context, auth middleware.AuthContext, req *modality.MusicEditRequest) error {
	if strings.TrimSpace(req.SourceJobID) == "" {
		return nil
	}
	asset, err := h.getSourceMusicAsset(c, auth, req.SourceJobID)
	if err != nil {
		return err
	}
	req.File = append([]byte(nil), asset.Data...)
	req.Filename = asset.Filename
	req.ContentType = asset.ContentType
	req.SourceAudio = ""
	return nil
}

func (h *MusicHandler) resolveMusicStemSource(c *gin.Context, auth middleware.AuthContext, req *modality.MusicStemRequest) error {
	if strings.TrimSpace(req.SourceJobID) == "" {
		return nil
	}
	asset, err := h.getSourceMusicAsset(c, auth, req.SourceJobID)
	if err != nil {
		return err
	}
	req.File = append([]byte(nil), asset.Data...)
	req.Filename = asset.Filename
	req.ContentType = asset.ContentType
	req.SourceAudio = ""
	return nil
}

func (h *MusicHandler) getSourceMusicAsset(c *gin.Context, auth middleware.AuthContext, sourceJobID string) (*modality.MusicAsset, error) {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil {
		return nil, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "", "Runtime configuration is unavailable.")
	}
	token, err := parseMusicJobID(snapshot, sourceJobID)
	if err != nil {
		return nil, err
	}
	if token.KeyID != auth.KeyID {
		return nil, httputil.NewError(http.StatusForbidden, "permission_error", "job_not_owned", "source_job_id", "Music job was created by a different API key.")
	}
	record, err := h.getMusicJob(c.Request.Context(), token.CacheKey)
	if err != nil {
		return nil, err
	}
	if record.Status.Status != "completed" || record.Status.Result == nil {
		return nil, httputil.NewError(http.StatusConflict, "invalid_request_error", "job_not_ready", "source_job_id", "Source music job is not ready yet.")
	}
	payload, err := base64.StdEncoding.DecodeString(record.AssetBody)
	if err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Source music asset is invalid.")
	}
	return &modality.MusicAsset{
		Data:        payload,
		ContentType: record.Status.Result.ContentType,
		Filename:    record.Status.Result.Filename,
	}, nil
}

func (h *MusicHandler) getMusicJob(ctx context.Context, cacheKey string) (musicJobRecord, error) {
	if h.cache == nil {
		return musicJobRecord{}, httputil.NewError(http.StatusConflict, "invalid_request_error", "async_jobs_unavailable", "", "Async music jobs require a configured cache backend.")
	}
	raw, ok, err := h.cache.Get(ctx, cacheKey)
	if err != nil {
		return musicJobRecord{}, httputil.NewError(http.StatusBadGateway, "provider_error", "cache_unavailable", "", "Music job state is unavailable.")
	}
	if !ok {
		return musicJobRecord{}, invalidMusicJobIDError()
	}
	var record musicJobRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return musicJobRecord{}, invalidMusicJobIDError()
	}
	return record, nil
}

func (h *MusicHandler) storeMusicJob(ctx context.Context, cacheKey string, record musicJobRecord) error {
	if h.cache == nil {
		return httputil.NewError(http.StatusConflict, "invalid_request_error", "async_jobs_unavailable", "", "Async music jobs require a configured cache backend.")
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return httputil.NewError(http.StatusInternalServerError, "internal_error", "job_encoding_failed", "", "Unable to store music job state.")
	}
	if err := h.cache.Set(ctx, cacheKey, string(raw), musicJobTTL); err != nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "cache_unavailable", "", "Music job state is unavailable.")
	}
	return nil
}

func (h *MusicHandler) logAsyncMusicOutcome(c *gin.Context, token *musicJobToken, status modality.MusicStatus) {
	if h.requestLogger == nil || token == nil {
		return
	}
	statusCode := http.StatusOK
	errorType := ""
	switch status.Status {
	case "failed":
		statusCode = http.StatusBadGateway
	case "cancelled", "canceled":
		statusCode = 499
		errorType = "canceled"
	}
	if status.Error != nil && strings.TrimSpace(status.Error.Type) != "" {
		errorType = status.Error.Type
	}
	_ = h.requestLogger.Log(store.RequestLog{
		RequestID:         "music:" + token.CacheKey,
		KeyID:             token.KeyID,
		Model:             token.Model,
		Modality:          modality.ModalityMusic,
		ProviderLatencyMs: 0,
		TotalLatencyMs:    0,
		InputTokens:       0,
		OutputTokens:      0,
		TotalTokens:       0,
		EstimatedCost:     middleware.EstimateCostUSD(token.Model, 0, 0),
		StatusCode:        statusCode,
		ErrorType:         errorType,
		CreatedAt:         time.Now().UTC(),
	})
}

func (h *MusicHandler) registry(c *gin.Context) *provider.Registry {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil {
		return nil
	}
	return snapshot.Registry
}

func parseMusicEditEnvelope(c *gin.Context) (*musicEditEnvelope, error) {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(c.ContentType())), "multipart/form-data") {
		return parseMusicEditMultipart(c)
	}
	var env musicEditEnvelope
	if err := c.ShouldBindJSON(&env); err != nil {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON.")
	}
	return &env, nil
}

func parseMusicStemEnvelope(c *gin.Context) (*musicStemEnvelope, error) {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(c.ContentType())), "multipart/form-data") {
		return parseMusicStemMultipart(c)
	}
	var env musicStemEnvelope
	if err := c.ShouldBindJSON(&env); err != nil {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON.")
	}
	return &env, nil
}

func parseMusicEditMultipart(c *gin.Context) (*musicEditEnvelope, error) {
	req := &musicEditEnvelope{
		Mode: c.DefaultPostForm("mode", musicModeSync),
		MusicEditRequest: modality.MusicEditRequest{
			Model:           c.PostForm("model"),
			Operation:       c.PostForm("operation"),
			Prompt:          c.PostForm("prompt"),
			Lyrics:          c.PostForm("lyrics"),
			SourceJobID:     c.PostForm("source_job_id"),
			SourceAudio:     c.PostForm("source_audio"),
			OutputFormat:    c.PostForm("output_format"),
			StoreForEditing: parseBoolFormValue(c.PostForm("store_for_editing")),
			SignWithC2PA:    parseBoolFormValue(c.PostForm("sign_with_c2pa")),
			Instrumental:    parseBoolFormValue(c.PostForm("instrumental")),
		},
	}
	routing, err := parseRoutingFormValue(c.PostForm("routing"))
	if err != nil {
		return nil, err
	}
	req.Routing = routing
	if value := strings.TrimSpace(c.PostForm("duration_ms")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_duration_ms", "duration_ms", "Field 'duration_ms' must be an integer.")
		}
		req.DurationMS = parsed
	}
	if value := strings.TrimSpace(c.PostForm("sample_rate_hz")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_sample_rate_hz", "sample_rate_hz", "Field 'sample_rate_hz' must be an integer.")
		}
		req.SampleRateHz = parsed
	}
	if value := strings.TrimSpace(c.PostForm("bitrate")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_bitrate", "bitrate", "Field 'bitrate' must be an integer.")
		}
		req.Bitrate = parsed
	}
	if value := strings.TrimSpace(c.PostForm("seed")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_seed", "seed", "Field 'seed' must be an integer.")
		}
		req.Seed = &parsed
	}
	if value := strings.TrimSpace(c.PostForm("plan")); value != "" {
		if !json.Valid([]byte(value)) {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_plan", "plan", "Field 'plan' must be valid JSON.")
		}
		req.Plan = json.RawMessage(value)
	}
	if fileHeader, err := c.FormFile("file"); err == nil {
		data, contentType, readErr := readMultipartFile(fileHeader)
		if readErr != nil {
			if httputil.IsRequestBodyTooLarge(readErr) {
				return nil, httputil.RequestBodyTooLargeError(0)
			}
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_file", "file", "Unable to read uploaded audio file.")
		}
		req.File = data
		req.Filename = fileHeader.Filename
		req.ContentType = contentType
	} else if httputil.IsRequestBodyTooLarge(err) {
		return nil, httputil.RequestBodyTooLargeError(0)
	}
	return req, nil
}

func parseMusicStemMultipart(c *gin.Context) (*musicStemEnvelope, error) {
	req := &musicStemEnvelope{
		Mode: c.DefaultPostForm("mode", musicModeSync),
		MusicStemRequest: modality.MusicStemRequest{
			Model:        c.PostForm("model"),
			SourceJobID:  c.PostForm("source_job_id"),
			SourceAudio:  c.PostForm("source_audio"),
			StemVariant:  c.PostForm("stem_variant"),
			OutputFormat: c.PostForm("output_format"),
			SignWithC2PA: parseBoolFormValue(c.PostForm("sign_with_c2pa")),
		},
	}
	routing, err := parseRoutingFormValue(c.PostForm("routing"))
	if err != nil {
		return nil, err
	}
	req.Routing = routing
	if fileHeader, err := c.FormFile("file"); err == nil {
		data, contentType, readErr := readMultipartFile(fileHeader)
		if readErr != nil {
			if httputil.IsRequestBodyTooLarge(readErr) {
				return nil, httputil.RequestBodyTooLargeError(0)
			}
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_file", "file", "Unable to read uploaded audio file.")
		}
		req.File = data
		req.Filename = fileHeader.Filename
		req.ContentType = contentType
	} else if httputil.IsRequestBodyTooLarge(err) {
		return nil, httputil.RequestBodyTooLargeError(0)
	}
	return req, nil
}

func validateMusicGenerationEnvelope(env *musicGenerationEnvelope) error {
	if strings.TrimSpace(env.Model) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	if strings.TrimSpace(env.Mode) == "" {
		env.Mode = musicModeSync
	}
	switch env.Mode {
	case musicModeSync, musicModeAsync, musicModeStream:
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_mode", "mode", "Field 'mode' must be 'sync', 'async', or 'stream'.")
	}
	if strings.TrimSpace(env.Prompt) == "" && strings.TrimSpace(env.Lyrics) == "" && len(env.Plan) == 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_prompt", "prompt", "Field 'prompt', 'lyrics', or 'plan' is required.")
	}
	if env.DurationMS < 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_duration_ms", "duration_ms", "Field 'duration_ms' must be positive.")
	}
	if env.SampleRateHz < 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_sample_rate_hz", "sample_rate_hz", "Field 'sample_rate_hz' must be positive.")
	}
	if env.Bitrate < 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_bitrate", "bitrate", "Field 'bitrate' must be positive.")
	}
	return nil
}

func validateMusicEditEnvelope(env *musicEditEnvelope) error {
	if strings.TrimSpace(env.Model) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	if strings.TrimSpace(env.Mode) == "" {
		env.Mode = musicModeSync
	}
	switch env.Mode {
	case musicModeSync, musicModeAsync, musicModeStream:
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_mode", "mode", "Field 'mode' must be 'sync', 'async', or 'stream'.")
	}
	switch strings.ToLower(strings.TrimSpace(env.Operation)) {
	case musicOperationRemix, musicOperationExtend, musicOperationCover, musicOperationInpaint:
		env.Operation = strings.ToLower(strings.TrimSpace(env.Operation))
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_operation", "operation", "Field 'operation' must be remix, extend, cover, or inpaint.")
	}
	if env.DurationMS < 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_duration_ms", "duration_ms", "Field 'duration_ms' must be positive.")
	}
	if env.SampleRateHz < 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_sample_rate_hz", "sample_rate_hz", "Field 'sample_rate_hz' must be positive.")
	}
	if env.Bitrate < 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_bitrate", "bitrate", "Field 'bitrate' must be positive.")
	}
	if countMusicSources(env.SourceJobID, env.SourceAudio, env.File) != 1 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_source", "source_job_id", "Exactly one source must be provided: 'source_job_id', 'source_audio', or an uploaded file.")
	}
	return nil
}

func validateMusicStemEnvelope(env *musicStemEnvelope) error {
	if strings.TrimSpace(env.Model) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	if strings.TrimSpace(env.Mode) == "" {
		env.Mode = musicModeSync
	}
	switch env.Mode {
	case musicModeSync, musicModeAsync:
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_mode", "mode", "Field 'mode' must be 'sync' or 'async'.")
	}
	if countMusicSources(env.SourceJobID, env.SourceAudio, env.File) != 1 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_source", "source_job_id", "Exactly one source must be provided: 'source_job_id', 'source_audio', or an uploaded file.")
	}
	return nil
}

func validateMusicLyricsRequest(req *modality.MusicLyricsRequest) error {
	if strings.TrimSpace(req.Model) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	if strings.TrimSpace(req.Prompt) == "" && strings.TrimSpace(req.Lyrics) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_prompt", "prompt", "Field 'prompt' or 'lyrics' is required.")
	}
	return nil
}

func validateMusicPlanRequest(req *modality.MusicPlanRequest) error {
	if strings.TrimSpace(req.Model) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_prompt", "prompt", "Field 'prompt' is required.")
	}
	if req.DurationMS < 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_duration_ms", "duration_ms", "Field 'duration_ms' must be positive.")
	}
	return nil
}

func validateMusicModelRequest(model provider.Model, outputFormat string, sampleRateHz int, durationMS int) error {
	if strings.TrimSpace(outputFormat) != "" && len(model.OutputFormats) > 0 && !containsString(model.OutputFormats, outputFormat) {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_output_format", "output_format", "Requested output format is not supported by this model.")
	}
	if sampleRateHz > 0 && len(model.SampleRatesHz) > 0 {
		allowed := false
		for _, candidate := range model.SampleRatesHz {
			if candidate == sampleRateHz {
				allowed = true
				break
			}
		}
		if !allowed {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_sample_rate", "sample_rate_hz", "Requested sample rate is not supported by this model.")
		}
	}
	if durationMS > 0 {
		if model.MinDurationMs > 0 && durationMS < model.MinDurationMs {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_duration_ms", "duration_ms", "Requested duration is below the selected model's minimum duration.")
		}
		if model.MaxDurationMs > 0 && durationMS > model.MaxDurationMs {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_duration_ms", "duration_ms", "Requested duration exceeds the selected model's maximum duration.")
		}
	}
	return nil
}

func requiredMusicEditCapabilities(env *musicEditEnvelope) []modality.Capability {
	required := []modality.Capability{modality.CapabilityMusicEditing}
	switch env.Operation {
	case musicOperationExtend:
		required = append(required, modality.CapabilityMusicExtension)
	case musicOperationCover:
		required = append(required, modality.CapabilityMusicCover)
	case musicOperationInpaint:
		required = append(required, modality.CapabilityMusicInpainting)
	}
	if env.Mode == musicModeStream {
		required = append(required, modality.CapabilityMusicStreaming)
	}
	if env.Instrumental {
		required = append(required, modality.CapabilityInstrumental)
	}
	return required
}

func decorateMusicStatus(c *gin.Context, status *modality.MusicStatus) {
	if status == nil || status.Result == nil || status.Status != "completed" {
		return
	}
	status.Result.DownloadURL = musicContentURL(c, c.Param("id"))
}

func musicContentURL(c *gin.Context, token string) string {
	scheme := c.Request.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return fmt.Sprintf("%s://%s/v1/music/jobs/%s/content", scheme, c.Request.Host, token)
}

func buildMusicResult(result *modality.MusicOperationResult) *modality.MusicResult {
	if result == nil {
		return nil
	}
	response := &modality.MusicResult{
		SongID:       result.SongID,
		DurationMS:   result.DurationMS,
		SampleRateHz: result.SampleRateHz,
		Bitrate:      result.Bitrate,
		SizeBytes:    result.SizeBytes,
		Lyrics:       result.Lyrics,
	}
	if len(result.Plan) > 0 {
		response.Plan = append(json.RawMessage(nil), result.Plan...)
	}
	if result.Asset != nil {
		response.ContentType = result.Asset.ContentType
		response.Filename = result.Asset.Filename
		if response.SizeBytes == 0 {
			response.SizeBytes = len(result.Asset.Data)
		}
	}
	return response
}

func encodeMusicAsset(result *modality.MusicOperationResult) string {
	if result == nil || result.Asset == nil || len(result.Asset.Data) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(result.Asset.Data)
}

func musicErrorFromErr(err error) *modality.MusicError {
	var apiErr *httputil.APIError
	if errorAsAPIError(err, &apiErr) {
		return &modality.MusicError{
			Type:    apiErr.Type,
			Code:    apiErr.Code,
			Message: apiErr.Message,
		}
	}
	if err == nil {
		return nil
	}
	return &modality.MusicError{
		Type:    "provider_error",
		Code:    "provider_error",
		Message: err.Error(),
	}
}

func normalizeMusicTimeoutError(err error) error {
	var apiErr *httputil.APIError
	if !errorAsAPIError(err, &apiErr) {
		return err
	}
	if apiErr.Type != "timeout_error" || apiErr.Code != "provider_timeout" {
		return err
	}
	return httputil.NewError(apiErr.Status, apiErr.Type, apiErr.Code, apiErr.Param, "Music provider timed out. Retry with mode=async for long-running jobs or increase the provider timeout.")
}

func errorAsAPIError(err error, target **httputil.APIError) bool {
	if err == nil {
		return false
	}
	apiErr, ok := err.(*httputil.APIError)
	if ok {
		*target = apiErr
		return true
	}
	return false
}

func firstNonEmptyMusicContentType(contentType string, requestedFormat string) string {
	if strings.TrimSpace(contentType) != "" {
		return contentType
	}
	switch strings.ToLower(strings.TrimSpace(requestedFormat)) {
	case "wav", "pcm":
		return "audio/wav"
	case "flac":
		return "audio/flac"
	case "opus":
		return "audio/opus"
	case "zip":
		return "application/zip"
	default:
		return "audio/mpeg"
	}
}

func countMusicSources(sourceJobID string, sourceAudio string, file []byte) int {
	count := 0
	if strings.TrimSpace(sourceJobID) != "" {
		count++
	}
	if strings.TrimSpace(sourceAudio) != "" {
		count++
	}
	if len(file) > 0 {
		count++
	}
	return count
}

func parseBoolFormValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
