package bytedance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

const bytedanceVideoTasksPath = "/contents/generations/tasks"

type VideoAdapter struct {
	client   *Client
	model    string
	endpoint string
}

type videoTaskRequest struct {
	Model         string              `json:"model"`
	Content       []videoContentInput `json:"content"`
	Duration      int                 `json:"duration,omitempty"`
	Resolution    string              `json:"resolution,omitempty"`
	Ratio         string              `json:"ratio,omitempty"`
	GenerateAudio *bool               `json:"generate_audio,omitempty"`
}

type videoContentInput struct {
	Type     string             `json:"type"`
	Text     string             `json:"text,omitempty"`
	Role     string             `json:"role,omitempty"`
	ImageURL *videoContentImage `json:"image_url,omitempty"`
	VideoURL *videoContentMedia `json:"video_url,omitempty"`
	AudioURL *videoContentMedia `json:"audio_url,omitempty"`
}

type videoContentImage struct {
	URL string `json:"url"`
}

type videoContentMedia struct {
	URL string `json:"url"`
}

func NewVideoAdapter(client *Client, model string, endpoint string) *VideoAdapter {
	return &VideoAdapter{
		client:   client,
		model:    model,
		endpoint: endpoint,
	}
}

func (a *VideoAdapter) Generate(ctx context.Context, req *modality.VideoRequest) (*modality.VideoJob, error) {
	payload := videoTaskRequest{
		Model:      providerVideoModelName(req.Model, a.model),
		Content:    buildVideoContent(req),
		Duration:   req.Duration,
		Resolution: req.Resolution,
		Ratio:      req.AspectRatio,
	}
	if req.WithAudio {
		payload.GenerateAudio = &req.WithAudio
	}

	raw, err := a.requestJSON(ctx, http.MethodPost, bytedanceVideoTasksPath, payload)
	if err != nil {
		return nil, err
	}

	jobID := firstPopulatedString(raw, "id", "task_id", "taskId", "data.id", "data.task_id")
	if jobID == "" {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance did not return a video task id.")
	}
	status := normalizeVideoTaskState(firstPopulatedString(raw, "status", "state", "data.status", "data.state"))
	if status == "" {
		status = "queued"
	}

	return &modality.VideoJob{
		JobID:         jobID,
		Status:        status,
		EstimatedTime: int(math.Round(firstPopulatedFloat(raw, "estimated_time", "eta", "data.estimated_time", "data.eta"))),
		Model:         req.Model,
	}, nil
}

func (a *VideoAdapter) GetStatus(ctx context.Context, jobID string) (*modality.VideoStatus, error) {
	raw, err := a.requestJSON(ctx, http.MethodGet, bytedanceVideoTasksPath+"/"+jobID, nil)
	if err != nil {
		return nil, err
	}

	status := normalizeVideoTaskState(firstPopulatedString(raw, "status", "state", "data.status", "data.state"))
	if status == "" {
		status = "queued"
	}

	resultContent := firstPopulatedMap(raw, "content", "result", "output", "data.content", "data.result", "data.output")
	result := normalizeVideoResult(resultContent)
	videoErr := normalizeVideoError(raw)

	if status == "completed" && result == nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned a completed video job without a result.")
	}
	if status == "failed" && videoErr == nil {
		videoErr = &modality.VideoError{
			Type:    "provider_error",
			Code:    "video_generation_failed",
			Message: "ByteDance video generation failed.",
		}
	}

	return &modality.VideoStatus{
		JobID:       jobID,
		Status:      status,
		Progress:    normalizeVideoProgress(firstPopulatedFloat(raw, "progress", "percent", "data.progress", "data.percent")),
		Result:      result,
		Error:       videoErr,
		CreatedAt:   int64(math.Round(firstPopulatedFloat(raw, "created_at", "created", "data.created_at", "data.created"))),
		CompletedAt: int64(math.Round(firstPopulatedFloat(raw, "completed_at", "finished_at", "data.completed_at", "data.finished_at"))),
		ExpiresAt:   int64(math.Round(firstPopulatedFloat(raw, "expires_at", "data.expires_at"))),
	}, nil
}

func (a *VideoAdapter) Cancel(ctx context.Context, jobID string) error {
	resp, err := a.client.RawRequest(ctx, http.MethodDelete, a.endpoint, bytedanceVideoTasksPath+"/"+jobID, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusAccepted, http.StatusNoContent:
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	case http.StatusNotFound:
		return httputil.NewError(http.StatusNotFound, "invalid_request_error", "job_not_found", "id", "Video job was not found.")
	case http.StatusConflict:
		return httputil.NewError(http.StatusConflict, "invalid_request_error", "job_immutable", "id", "Completed or failed video jobs cannot be cancelled.")
	default:
		return a.client.apiError(resp)
	}
}

func (a *VideoAdapter) Download(ctx context.Context, jobID string, status *modality.VideoStatus) (*modality.VideoAsset, error) {
	if status == nil || status.Result == nil || strings.TrimSpace(status.Result.VideoURL) == "" {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance did not return a downloadable video URL.")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, status.Result.VideoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build bytedance video download request: %w", err)
	}
	resp, err := a.client.httpClient.Do(req)
	if err != nil {
		return nil, translateTransportError(err, "ByteDance")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusGone {
		return nil, httputil.NewError(http.StatusGone, "invalid_request_error", "asset_expired", "id", "Video asset has expired.")
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_download_failed", "", "ByteDance video download failed.")
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid video asset.")
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = firstNonEmpty(status.Result.ContentType, "video/mp4")
	}
	return &modality.VideoAsset{
		Data:        data,
		ContentType: contentType,
	}, nil
}

func (a *VideoAdapter) requestJSON(ctx context.Context, method string, path string, body any) (map[string]any, error) {
	resp, err := a.client.RawRequest(ctx, method, a.endpoint, path, body)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, httputil.NewError(http.StatusNotFound, "invalid_request_error", "job_not_found", "id", "Video job was not found.")
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, a.client.apiError(resp)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid video response.")
	}

	var raw map[string]any
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid JSON response.")
	}
	return raw, nil
}

func buildVideoContent(req *modality.VideoRequest) []videoContentInput {
	content := []videoContentInput{{
		Type: "text",
		Text: req.Prompt,
	}}

	if normalized := normalizeSingleReference(req.FirstFrame); normalized != "" {
		content = append(content, videoContentInput{
			Type: "image_url",
			Role: "first_frame",
			ImageURL: &videoContentImage{
				URL: normalized,
			},
		})
	}

	if normalized := normalizeSingleReference(req.LastFrame); normalized != "" {
		content = append(content, videoContentInput{
			Type: "image_url",
			Role: "last_frame",
			ImageURL: &videoContentImage{
				URL: normalized,
			},
		})
	}

	for _, image := range normalizeReferenceImages(req.ReferenceImages) {
		content = append(content, videoContentInput{
			Type: "image_url",
			Role: "reference_image",
			ImageURL: &videoContentImage{
				URL: image,
			},
		})
	}

	for _, video := range normalizeReferenceVideos(req.ReferenceVideos) {
		content = append(content, videoContentInput{
			Type: "video_url",
			Role: "reference_video",
			VideoURL: &videoContentMedia{
				URL: video,
			},
		})
	}

	if normalized := normalizeAudioReference(req.Audio); normalized != "" {
		content = append(content, videoContentInput{
			Type: "audio_url",
			Role: "reference_audio",
			AudioURL: &videoContentMedia{
				URL: normalized,
			},
		})
	}

	return content
}

func normalizeSingleReference(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return normalizeReferenceImages([]string{value})[0]
}

func normalizeReferenceVideos(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		normalized = append(normalized, value)
	}
	return normalized
}

func normalizeAudioReference(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if isStructuredMediaReference(value) {
		return value
	}
	return "data:audio/wav;base64," + value
}

func isStructuredMediaReference(value string) bool {
	return strings.HasPrefix(value, "http://") ||
		strings.HasPrefix(value, "https://") ||
		strings.HasPrefix(value, "data:") ||
		strings.HasPrefix(value, "asset://")
}

func normalizeVideoTaskState(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "queued", "submitted", "pending", "created":
		return "queued"
	case "processing", "running", "in_progress", "in-progress":
		return "processing"
	case "succeeded", "success", "completed", "done":
		return "completed"
	case "failed", "error":
		return "failed"
	case "cancelled", "canceled":
		return "failed"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeVideoProgress(value float64) float64 {
	if value <= 0 {
		return 0
	}
	if value > 1 {
		value = value / 100
	}
	if value > 1 {
		return 1
	}
	return value
}

func normalizeVideoResult(raw map[string]any) *modality.VideoResult {
	if len(raw) == 0 {
		return nil
	}
	videoURL := firstPopulatedString(raw, "video_url", "url", "videoUrl")
	audioURL := firstPopulatedString(raw, "audio_url", "audioUrl")
	if strings.TrimSpace(videoURL) == "" && strings.TrimSpace(audioURL) == "" {
		return nil
	}
	return &modality.VideoResult{
		VideoURL:    videoURL,
		AudioURL:    audioURL,
		ContentType: firstNonEmpty(firstPopulatedString(raw, "mime_type", "content_type"), "video/mp4"),
		Duration:    int(math.Round(firstPopulatedFloat(raw, "duration", "duration_seconds"))),
		Width:       int(math.Round(firstPopulatedFloat(raw, "width"))),
		Height:      int(math.Round(firstPopulatedFloat(raw, "height"))),
	}
}

func normalizeVideoError(raw map[string]any) *modality.VideoError {
	errorMap := firstPopulatedMap(raw, "error", "data.error")
	if len(errorMap) == 0 {
		status := strings.ToLower(strings.TrimSpace(firstPopulatedString(raw, "status", "state", "data.status", "data.state")))
		if status == "cancelled" || status == "canceled" {
			return &modality.VideoError{
				Type:    "invalid_request_error",
				Code:    "job_cancelled",
				Message: "Video job was cancelled.",
			}
		}
		return nil
	}

	return &modality.VideoError{
		Type:    firstNonEmpty(firstPopulatedString(errorMap, "type"), "provider_error"),
		Code:    firstPopulatedString(errorMap, "code"),
		Message: firstNonEmpty(firstPopulatedString(errorMap, "message"), "ByteDance video generation failed."),
	}
}

func firstPopulatedString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := lookupPath(raw, key); ok {
			switch typed := value.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					return typed
				}
			}
		}
	}
	return ""
}

func firstPopulatedFloat(raw map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if value, ok := lookupPath(raw, key); ok {
			switch typed := value.(type) {
			case float64:
				return typed
			case json.Number:
				if parsed, err := typed.Float64(); err == nil {
					return parsed
				}
			}
		}
	}
	return 0
}

func firstPopulatedMap(raw map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if value, ok := lookupPath(raw, key); ok {
			if typed, ok := value.(map[string]any); ok {
				return typed
			}
		}
	}
	return nil
}

func lookupPath(raw map[string]any, key string) (any, bool) {
	current := any(raw)
	for _, part := range strings.Split(key, ".") {
		asMap, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = asMap[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}
