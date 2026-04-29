package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
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

const podcastJobTTL = 24 * time.Hour

type PodcastHandler struct {
	runtime       *gwruntime.Holder
	cache         cachepkg.Cache
	requestLogger *store.AsyncRequestLogger

	cancelMu sync.Mutex
	cancels  map[string]context.CancelFunc
}

type podcastJobRecord struct {
	Status    modality.PodcastStatus `json:"status"`
	AssetBody string                 `json:"asset_body,omitempty"`
}

func NewPodcastHandler(runtime *gwruntime.Holder, cache cachepkg.Cache, requestLogger *store.AsyncRequestLogger) *PodcastHandler {
	return &PodcastHandler{
		runtime:       runtime,
		cache:         cache,
		requestLogger: requestLogger,
		cancels:       map[string]context.CancelFunc{},
	}
}

func (h *PodcastHandler) Create(c *gin.Context) {
	var req modality.PodcastRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if err := validatePodcastRequest(&req); err != nil {
		httputil.WriteError(c, err)
		return
	}

	registry, snapshot := h.runtimeDeps(c)
	if registry == nil || snapshot == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable."))
		return
	}
	if h.cache == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusConflict, "invalid_request_error", "async_jobs_unavailable", "", "Async podcast jobs require a configured cache backend."))
		return
	}
	auth := middleware.GetAuthContext(c)
	resolved, err := resolveEndpointModel(c, registry, auth, req.Model, req.Routing, modality.ModalityPodcast, modality.CapabilityPodcastGeneration)
	if err != nil {
		writeModalityTargetError(c, err, "audio podcasts")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model
	adapter, _, err := registry.GetPodcastAdapter(model.ID)
	if err != nil {
		writeModalityTargetError(c, err, "audio podcasts")
		return
	}

	req.Model = model.ID
	cacheKey, err := newPodcastJobCacheKey()
	if err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "job_id_encoding_failed", "", "Unable to create podcast job."))
		return
	}
	expiresAt := time.Now().Add(podcastJobTTL).Unix()
	jobID, err := signPodcastJobID(snapshot, model, cacheKey, auth.KeyID, expiresAt)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	record := podcastJobRecord{
		Status: modality.PodcastStatus{
			ID:     jobID,
			Object: "audio.podcast",
			Model:  model.ID,
			Status: modality.PodcastStatusQueued,
		},
	}
	if err := h.storePodcastJob(c.Request.Context(), cacheKey, record); err != nil {
		httputil.WriteError(c, err)
		return
	}
	token, err := parsePodcastJobID(snapshot, jobID)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	h.runAsyncPodcastJob(&token, func(ctx context.Context) (*modality.PodcastResult, error) {
		return adapter.GeneratePodcast(ctx, &req)
	})
	c.JSON(http.StatusOK, modality.PodcastJob{
		ID:     jobID,
		Object: "audio.podcast",
		Model:  model.ID,
		Status: modality.PodcastStatusQueued,
	})
}

func (h *PodcastHandler) Get(c *gin.Context) {
	record, _, err := h.resolveAuthorizedPodcast(c)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, record.Status)
}

func (h *PodcastHandler) Content(c *gin.Context) {
	record, _, err := h.resolveAuthorizedPodcast(c)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if record.Status.Status != modality.PodcastStatusCompleted || record.Status.Result == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusConflict, "invalid_request_error", "job_not_ready", "id", "Podcast job is not ready yet."))
		return
	}
	if strings.TrimSpace(record.AssetBody) == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Podcast provider returned an empty asset."))
		return
	}
	payload, err := base64.StdEncoding.DecodeString(record.AssetBody)
	if err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Podcast asset is invalid."))
		return
	}
	contentType := "application/octet-stream"
	if record.Status.Result != nil && strings.TrimSpace(record.Status.Result.ContentType) != "" {
		contentType = record.Status.Result.ContentType
	}
	c.Data(http.StatusOK, contentType, payload)
}

func (h *PodcastHandler) Cancel(c *gin.Context) {
	record, token, err := h.resolveAuthorizedPodcast(c)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	switch record.Status.Status {
	case modality.PodcastStatusCompleted, modality.PodcastStatusFailed:
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
	record.Status.Status = modality.PodcastStatusFailed
	record.Status.Error = &modality.AudioError{
		Type:    "invalid_request_error",
		Code:    "job_cancelled",
		Message: "Podcast job was cancelled.",
	}
	if err := h.storePodcastJob(c.Request.Context(), token.CacheKey, record); err != nil {
		httputil.WriteError(c, err)
		return
	}
	h.logAsyncPodcastOutcome(token, record.Status)
	c.Status(http.StatusNoContent)
}

func (h *PodcastHandler) runAsyncPodcastJob(token *podcastJobToken, fn func(context.Context) (*modality.PodcastResult, error)) {
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
		record, err := h.getPodcastJob(stateCtx, token.CacheKey)
		if err != nil {
			return
		}
		record.Status.Status = modality.PodcastStatusRunning
		_ = h.storePodcastJob(stateCtx, token.CacheKey, record)

		result, err := fn(runCtx)
		if err != nil {
			record.Status.Status = modality.PodcastStatusFailed
			record.Status.Error = podcastErrorFromErr(err)
			if runCtx.Err() == context.Canceled {
				record.Status.Error = &modality.AudioError{
					Type:    "invalid_request_error",
					Code:    "job_cancelled",
					Message: "Podcast job was cancelled.",
				}
			}
			if storeErr := h.storePodcastJob(stateCtx, token.CacheKey, record); storeErr == nil {
				h.logAsyncPodcastOutcome(token, record.Status)
			}
			return
		}

		record.Status.Status = modality.PodcastStatusCompleted
		record.Status.Result = result
		record.AssetBody = encodePodcastAsset(result)
		if storeErr := h.storePodcastJob(stateCtx, token.CacheKey, record); storeErr == nil {
			h.logAsyncPodcastOutcome(token, record.Status)
		}
	}()
}

func (h *PodcastHandler) resolveAuthorizedPodcast(c *gin.Context) (podcastJobRecord, *podcastJobToken, error) {
	registry, snapshot := h.runtimeDeps(c)
	if registry == nil || snapshot == nil {
		return podcastJobRecord{}, nil, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable.")
	}
	token, err := parsePodcastJobID(snapshot, c.Param("id"))
	if err != nil {
		return podcastJobRecord{}, nil, err
	}
	auth := middleware.GetAuthContext(c)
	if token.KeyID != "" && auth.KeyID != "" && token.KeyID != auth.KeyID {
		return podcastJobRecord{}, nil, invalidPodcastJobIDError()
	}
	if _, err := resolveEndpointModel(c, registry, auth, token.Model, nil, modality.ModalityPodcast, modality.CapabilityPodcastGeneration); err != nil {
		return podcastJobRecord{}, nil, translatePodcastTargetError(err)
	}
	record, err := h.getPodcastJob(c.Request.Context(), token.CacheKey)
	if err != nil {
		return podcastJobRecord{}, nil, err
	}
	record.Status.ID = c.Param("id")
	if strings.TrimSpace(record.Status.Model) == "" {
		record.Status.Model = token.Model
	}
	return record, &token, nil
}

func (h *PodcastHandler) getPodcastJob(ctx context.Context, cacheKey string) (podcastJobRecord, error) {
	if h.cache == nil {
		return podcastJobRecord{}, httputil.NewError(http.StatusConflict, "invalid_request_error", "async_jobs_unavailable", "", "Async podcast jobs require a configured cache backend.")
	}
	raw, ok, err := h.cache.Get(ctx, cacheKey)
	if err != nil {
		return podcastJobRecord{}, httputil.NewError(http.StatusBadGateway, "provider_error", "cache_unavailable", "", "Podcast job state is unavailable.")
	}
	if !ok {
		return podcastJobRecord{}, invalidPodcastJobIDError()
	}
	var record podcastJobRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return podcastJobRecord{}, invalidPodcastJobIDError()
	}
	return record, nil
}

func (h *PodcastHandler) storePodcastJob(ctx context.Context, cacheKey string, record podcastJobRecord) error {
	if h.cache == nil {
		return httputil.NewError(http.StatusConflict, "invalid_request_error", "async_jobs_unavailable", "", "Async podcast jobs require a configured cache backend.")
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return httputil.NewError(http.StatusInternalServerError, "internal_error", "job_encoding_failed", "", "Unable to store podcast job state.")
	}
	if err := h.cache.Set(ctx, cacheKey, string(raw), podcastJobTTL); err != nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "cache_unavailable", "", "Podcast job state is unavailable.")
	}
	return nil
}

func (h *PodcastHandler) runtimeDeps(c *gin.Context) (*provider.Registry, *gwruntime.Snapshot) {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil {
		return nil, nil
	}
	return snapshot.Registry, snapshot
}

func (h *PodcastHandler) logAsyncPodcastOutcome(token *podcastJobToken, status modality.PodcastStatus) {
	if h.requestLogger == nil || token == nil {
		return
	}
	statusCode := http.StatusOK
	errorType := ""
	if status.Status == modality.PodcastStatusFailed {
		statusCode = http.StatusBadGateway
	}
	if status.Error != nil && strings.TrimSpace(status.Error.Type) != "" {
		errorType = status.Error.Type
	}
	promptTokens := 0
	completionTokens := 0
	totalTokens := 0
	tokenSource := modality.TokenCountSourceUnavailable
	if status.Result != nil {
		usage := normalizeUsage(status.Result.Usage)
		promptTokens = usage.PromptTokens
		completionTokens = usage.CompletionTokens
		totalTokens = usage.TotalTokens
		tokenSource = usage.Source
	}
	_ = h.requestLogger.Log(store.RequestLog{
		RequestID:       "podcast:" + token.CacheKey,
		KeyID:           token.KeyID,
		Model:           token.Model,
		Modality:        modality.ModalityPodcast,
		InterfaceFamily: "audio_podcasts",
		TokenSource:     string(tokenSource),
		InputTokens:     promptTokens,
		OutputTokens:    completionTokens,
		TotalTokens:     totalTokens,
		EstimatedCost:   middleware.EstimateCostUSD(token.Model, promptTokens, completionTokens),
		StatusCode:      statusCode,
		ErrorType:       errorType,
		CreatedAt:       time.Now().UTC(),
	})
}

func validatePodcastRequest(req *modality.PodcastRequest) error {
	if strings.TrimSpace(req.Model) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	if len(req.Segments) == 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_segments", "segments", "Field 'segments' is required.")
	}
	for index, segment := range req.Segments {
		if strings.TrimSpace(segment.Text) == "" {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_segment", "segments", "Podcast segment text must not be empty.")
		}
		if strings.TrimSpace(segment.Speaker) == "" && strings.TrimSpace(segment.Voice) == "" {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_segment_voice", "segments", "Each podcast segment must include 'speaker' or 'voice'.")
		}
		if index >= 100 {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "too_many_segments", "segments", "Field 'segments' must contain at most 100 items.")
		}
	}
	return nil
}

func encodePodcastAsset(result *modality.PodcastResult) string {
	if result == nil || len(result.Audio) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(result.Audio)
}

func podcastErrorFromErr(err error) *modality.AudioError {
	var apiErr *httputil.APIError
	if errors.As(err, &apiErr) {
		return &modality.AudioError{
			Type:    apiErr.Type,
			Code:    apiErr.Code,
			Message: apiErr.Message,
			Param:   apiErr.Param,
		}
	}
	return &modality.AudioError{
		Type:    "provider_error",
		Code:    "provider_error",
		Message: firstNonBlank(strings.TrimSpace(err.Error()), "Podcast generation failed."),
	}
}

func translatePodcastTargetError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, provider.ErrUnknownAlias):
		return httputil.NewError(http.StatusNotFound, "model_not_found", "unknown_alias", "model", "Model alias is not defined.")
	case errors.Is(err, provider.ErrUnknownModel):
		return httputil.NewError(http.StatusNotFound, "model_not_found", "unknown_model", "model", "Requested model is not registered.")
	case errors.Is(err, provider.ErrModalityMismatch):
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "modality_mismatch", "model", "Requested model does not support audio podcasts.")
	case errors.Is(err, provider.ErrCapabilityMissing):
		return httputil.NewError(http.StatusBadRequest, "capability_not_supported", "capability_missing", "model", "Requested model does not support podcast generation.")
	case errors.Is(err, provider.ErrAdapterMissing):
		return httputil.NewError(http.StatusBadGateway, "provider_error", "adapter_unavailable", "model", "Requested model is configured but not available in this runtime build.")
	default:
		return err
	}
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
