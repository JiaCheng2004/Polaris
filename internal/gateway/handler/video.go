package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/gin-gonic/gin"
)

type VideoHandler struct {
	runtime *gwruntime.Holder
}

var errVideoResponseWritten = errors.New("video response already written")

func NewVideoHandler(runtime *gwruntime.Holder) *VideoHandler {
	return &VideoHandler{runtime: runtime}
}

func (h *VideoHandler) Generate(c *gin.Context) {
	var req modality.VideoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if err := validateVideoRequest(&req); err != nil {
		httputil.WriteError(c, err)
		return
	}

	registry := h.registry(c)
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if registry == nil || snapshot == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable."))
		return
	}

	required := []modality.Capability{modality.CapabilityTextToVideo}
	if strings.TrimSpace(req.FirstFrame) != "" {
		required[0] = modality.CapabilityImageToVideo
	}
	if strings.TrimSpace(req.LastFrame) != "" {
		required = append(required, modality.CapabilityLastFrame)
	}
	if len(req.ReferenceImages) > 0 {
		required = append(required, modality.CapabilityReferenceImages)
	}
	if len(req.ReferenceVideos) > 0 {
		required = append(required, modality.CapabilityVideoInput)
	}
	if strings.TrimSpace(req.Audio) != "" {
		required = append(required, modality.CapabilityAudioInput)
	}
	if req.WithAudio {
		required = append(required, modality.CapabilityNativeAudio)
	}

	auth := middleware.GetAuthContext(c)
	resolved, err := resolveEndpointModel(c.Request.Context(), registry, auth, req.Model, req.Routing, modality.ModalityVideo, required...)
	if err != nil {
		writeModalityTargetError(c, err, "video")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model
	if err := validateVideoModelRequest(model, &req); err != nil {
		httputil.WriteError(c, err)
		return
	}

	adapter, _, err := registry.GetVideoAdapter(model.ID)
	if err != nil {
		writeModalityTargetError(c, err, "video")
		return
	}

	req.Model = model.ID
	job, err := adapter.Generate(c.Request.Context(), &req)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if job == nil || strings.TrimSpace(job.JobID) == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Video provider did not return a job id."))
		return
	}

	signedJobID, err := signVideoJobID(snapshot, model, job.JobID, auth.KeyID)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	job.JobID = signedJobID
	if strings.TrimSpace(job.Model) == "" {
		job.Model = model.ID
	}
	if strings.TrimSpace(job.Status) == "" {
		job.Status = "queued"
	}

	middleware.SetRequestOutcome(c, middleware.RequestOutcome{
		Model:      model.ID,
		Provider:   model.Provider,
		Modality:   modality.ModalityVideo,
		StatusCode: http.StatusOK,
	})
	c.JSON(http.StatusOK, job)
}

func (h *VideoHandler) Get(c *gin.Context) {
	token, _, adapter, err := h.resolveAuthorizedJob(c)
	if err != nil {
		if !errors.Is(err, errVideoResponseWritten) {
			httputil.WriteError(c, err)
		}
		return
	}

	status, err := adapter.GetStatus(c.Request.Context(), token.ProviderJobID)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if status == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Video provider returned an empty job status."))
		return
	}
	status.JobID = c.Param("id")
	decorateVideoStatus(c, status)
	c.JSON(http.StatusOK, status)
}

func (h *VideoHandler) Cancel(c *gin.Context) {
	token, model, adapter, err := h.resolveAuthorizedJob(c)
	if err != nil {
		if !errors.Is(err, errVideoResponseWritten) {
			httputil.WriteError(c, err)
		}
		return
	}
	if !model.Cancelable {
		httputil.WriteError(c, httputil.NewError(http.StatusConflict, "invalid_request_error", "job_not_cancelable", "id", "This video job cannot be cancelled."))
		return
	}

	if err := adapter.Cancel(c.Request.Context(), token.ProviderJobID); err != nil {
		httputil.WriteError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *VideoHandler) Content(c *gin.Context) {
	token, _, adapter, err := h.resolveAuthorizedJob(c)
	if err != nil {
		if !errors.Is(err, errVideoResponseWritten) {
			httputil.WriteError(c, err)
		}
		return
	}

	status, err := adapter.GetStatus(c.Request.Context(), token.ProviderJobID)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if status == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Video provider returned an empty job status."))
		return
	}
	if status.Status != "completed" {
		httputil.WriteError(c, httputil.NewError(http.StatusConflict, "invalid_request_error", "job_not_ready", "id", "Video job is not ready yet."))
		return
	}
	if status.ExpiresAt > 0 && time.Now().Unix() >= status.ExpiresAt {
		httputil.WriteError(c, httputil.NewError(http.StatusGone, "invalid_request_error", "asset_expired", "id", "Video asset has expired."))
		return
	}

	asset, err := adapter.Download(c.Request.Context(), token.ProviderJobID, status)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if asset == nil || len(asset.Data) == 0 {
		httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Video provider returned an empty asset."))
		return
	}
	contentType := strings.TrimSpace(asset.ContentType)
	if contentType == "" && status.Result != nil {
		contentType = strings.TrimSpace(status.Result.ContentType)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Data(http.StatusOK, contentType, asset.Data)
}

func (h *VideoHandler) registry(c *gin.Context) *provider.Registry {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil {
		return nil
	}
	return snapshot.Registry
}

func validateVideoRequest(req *modality.VideoRequest) error {
	if strings.TrimSpace(req.Model) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_prompt", "prompt", "Field 'prompt' is required.")
	}
	if req.Duration < 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_duration", "duration", "Field 'duration' must be positive.")
	}
	if strings.TrimSpace(req.LastFrame) != "" && strings.TrimSpace(req.FirstFrame) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_first_frame", "last_frame", "Field 'last_frame' requires 'first_frame'.")
	}
	if strings.TrimSpace(req.FirstFrame) != "" || strings.TrimSpace(req.LastFrame) != "" {
		if len(req.ReferenceImages) > 0 {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "conflicting_inputs", "reference_images", "Fields 'first_frame'/'last_frame' cannot be combined with 'reference_images'.")
		}
		if len(req.ReferenceVideos) > 0 {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "conflicting_inputs", "reference_videos", "Fields 'first_frame'/'last_frame' cannot be combined with 'reference_videos'.")
		}
		if strings.TrimSpace(req.Audio) != "" {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "conflicting_inputs", "audio", "Fields 'first_frame'/'last_frame' cannot be combined with input 'audio'.")
		}
	}
	if strings.TrimSpace(req.Audio) != "" && len(req.ReferenceImages) == 0 && len(req.ReferenceVideos) == 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_audio", "audio", "Field 'audio' requires at least one reference image or reference video.")
	}
	for _, image := range req.ReferenceImages {
		if strings.TrimSpace(image) == "" {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_reference_images", "reference_images", "Field 'reference_images' must not contain empty entries.")
		}
	}
	for _, video := range req.ReferenceVideos {
		if strings.TrimSpace(video) == "" {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_reference_videos", "reference_videos", "Field 'reference_videos' must not contain empty entries.")
		}
	}
	return nil
}

func validateVideoModelRequest(model provider.Model, req *modality.VideoRequest) error {
	if req.Duration != 0 {
		if len(model.AllowedDurations) > 0 && !containsInt(model.AllowedDurations, req.Duration) {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_duration", "duration", fmt.Sprintf("Requested duration is not supported by this model. Allowed values: %s.", joinInts(model.AllowedDurations)))
		}
		if model.MaxDuration > 0 && req.Duration > model.MaxDuration {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_duration", "duration", "Field 'duration' exceeds the selected model's maximum duration.")
		}
	}
	if strings.TrimSpace(req.AspectRatio) != "" && len(model.AspectRatios) > 0 && !containsString(model.AspectRatios, req.AspectRatio) {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_aspect_ratio", "aspect_ratio", "Requested aspect ratio is not supported by this model.")
	}
	if strings.TrimSpace(req.Resolution) != "" && len(model.Resolutions) > 0 && !containsString(model.Resolutions, req.Resolution) {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_resolution", "resolution", "Requested resolution is not supported by this model.")
	}
	return nil
}

func (h *VideoHandler) resolveAuthorizedJob(c *gin.Context) (*videoJobToken, provider.Model, modality.VideoAdapter, error) {
	registry := h.registry(c)
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if registry == nil || snapshot == nil {
		return nil, provider.Model{}, nil, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable.")
	}

	token := c.Param("id")
	auth := middleware.GetAuthContext(c)
	jobToken, err := parseVideoJobID(snapshot, token)
	if err != nil {
		return nil, provider.Model{}, nil, err
	}
	if auth.KeyID != jobToken.KeyID {
		return nil, provider.Model{}, nil, httputil.NewError(http.StatusForbidden, "permission_error", "job_not_owned", "id", "Video job was created by a different API key.")
	}

	resolved, err := resolveEndpointModel(c.Request.Context(), registry, auth, jobToken.Model, nil, modality.ModalityVideo)
	if err != nil {
		writeModalityTargetError(c, err, "video")
		return nil, provider.Model{}, nil, errVideoResponseWritten
	}
	model := resolved.Model
	if model.Provider != jobToken.Provider {
		return nil, provider.Model{}, nil, httputil.NewError(http.StatusNotFound, "invalid_request_error", "job_not_found", "id", "Video job was not found.")
	}

	adapter, _, err := registry.GetVideoAdapter(model.ID)
	if err != nil {
		writeModalityTargetError(c, err, "video")
		return nil, provider.Model{}, nil, errVideoResponseWritten
	}
	return &jobToken, model, adapter, nil
}

func decorateVideoStatus(c *gin.Context, status *modality.VideoStatus) {
	if status == nil || status.Result == nil || status.Status != "completed" {
		return
	}
	status.Result.DownloadURL = videoContentURL(c, c.Param("id"))
	if strings.TrimSpace(status.Result.ContentType) == "" {
		status.Result.ContentType = "video/mp4"
	}
}

func videoContentURL(c *gin.Context, token string) string {
	scheme := c.Request.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return fmt.Sprintf("%s://%s/v1/video/generations/%s/content", scheme, c.Request.Host, token)
}

func containsInt(values []int, candidate int) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func joinInts(values []int) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, ", ")
}
