package replicate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

type VideoAdapter struct {
	client *Client
	model  string
}

type predictionCreateRequest struct {
	Input map[string]any `json:"input"`
}

type predictionResponse struct {
	ID          string `json:"id"`
	Model       string `json:"model"`
	Version     string `json:"version"`
	Status      string `json:"status"`
	Error       string `json:"error"`
	CreatedAt   string `json:"created_at"`
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at"`
	Output      any    `json:"output"`
	URLs        struct {
		Get    string `json:"get"`
		Cancel string `json:"cancel"`
		Stream string `json:"stream"`
	} `json:"urls"`
}

func NewVideoAdapter(client *Client, model string) *VideoAdapter {
	return &VideoAdapter{client: client, model: model}
}

func (a *VideoAdapter) Generate(ctx context.Context, req *modality.VideoRequest) (*modality.VideoJob, error) {
	modelOwner, modelName, err := parseProviderModel(providerModelName(req.Model, a.model))
	if err != nil {
		return nil, err
	}

	payload := predictionCreateRequest{
		Input: buildPredictionInput(req),
	}
	var response predictionResponse
	if err := a.client.JSON(ctx, http.MethodPost, "/models/"+url.PathEscape(modelOwner)+"/"+url.PathEscape(modelName)+"/predictions", payload, &response); err != nil {
		return nil, err
	}
	if strings.TrimSpace(response.ID) == "" {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Replicate did not return a prediction id.")
	}
	return &modality.VideoJob{
		JobID:  response.ID,
		Status: normalizePredictionStatus(response.Status),
		Model:  firstNonEmpty(req.Model, a.model),
	}, nil
}

func (a *VideoAdapter) GetStatus(ctx context.Context, jobID string) (*modality.VideoStatus, error) {
	prediction, err := a.getPrediction(ctx, jobID)
	if err != nil {
		return nil, err
	}
	status := normalizePredictionStatus(prediction.Status)
	result := predictionToResult(prediction.Output)
	videoErr := predictionToError(prediction)

	completedAt := parseTimestamp(prediction.CompletedAt)
	expiresAt := int64(0)
	if completedAt > 0 {
		expiresAt = completedAt + int64(time.Hour.Seconds())
	}

	return &modality.VideoStatus{
		JobID:       prediction.ID,
		Status:      status,
		Result:      result,
		Error:       videoErr,
		CreatedAt:   parseTimestamp(prediction.CreatedAt),
		CompletedAt: completedAt,
		ExpiresAt:   expiresAt,
	}, nil
}

func (a *VideoAdapter) Cancel(ctx context.Context, jobID string) error {
	resp, err := a.client.do(ctx, http.MethodPost, "/predictions/"+url.PathEscape(jobID)+"/cancel", nil, nil)
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
	case http.StatusConflict, http.StatusUnprocessableEntity:
		return httputil.NewError(http.StatusConflict, "invalid_request_error", "job_immutable", "id", "Completed or failed video jobs cannot be cancelled.")
	default:
		return a.client.apiError(resp)
	}
}

func (a *VideoAdapter) Download(ctx context.Context, jobID string, status *modality.VideoStatus) (*modality.VideoAsset, error) {
	videoURL := ""
	if status != nil && status.Result != nil {
		videoURL = strings.TrimSpace(status.Result.VideoURL)
	}
	if videoURL == "" {
		prediction, err := a.getPrediction(ctx, jobID)
		if err != nil {
			return nil, err
		}
		result := predictionToResult(prediction.Output)
		if result == nil {
			return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Replicate returned no downloadable video URL.")
		}
		videoURL = strings.TrimSpace(result.VideoURL)
	}
	if videoURL == "" {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Replicate returned no downloadable video URL.")
	}

	resp, err := a.client.Download(ctx, videoURL)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	switch resp.StatusCode {
	case http.StatusGone:
		return nil, httputil.NewError(http.StatusGone, "invalid_request_error", "asset_expired", "id", "Video asset has expired.")
	case http.StatusNotFound:
		return nil, httputil.NewError(http.StatusGone, "invalid_request_error", "asset_expired", "id", "Video asset is no longer available.")
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_download_failed", "", "Replicate video download failed.")
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Replicate returned an invalid video asset.")
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "video/mp4"
	}
	return &modality.VideoAsset{Data: data, ContentType: contentType}, nil
}

func (a *VideoAdapter) getPrediction(ctx context.Context, jobID string) (*predictionResponse, error) {
	var prediction predictionResponse
	if err := a.client.JSON(ctx, http.MethodGet, "/predictions/"+url.PathEscape(jobID), nil, &prediction); err != nil {
		var apiErr *httputil.APIError
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, httputil.NewError(http.StatusNotFound, "invalid_request_error", "job_not_found", "id", "Video job was not found.")
		}
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return nil, httputil.NewError(http.StatusNotFound, "invalid_request_error", "job_not_found", "id", "Video job was not found.")
		}
		return nil, err
	}
	return &prediction, nil
}

func buildPredictionInput(req *modality.VideoRequest) map[string]any {
	input := map[string]any{
		"prompt": req.Prompt,
	}
	if ref := firstNonEmpty(strings.TrimSpace(req.FirstFrame), firstReference(req.ReferenceImages)); ref != "" {
		input["image"] = ref
		input["first_frame_image"] = ref
	}
	if ref := strings.TrimSpace(req.LastFrame); ref != "" {
		input["last_frame_image"] = ref
	}
	if req.Duration > 0 {
		input["duration"] = req.Duration
		input["duration_seconds"] = req.Duration
	}
	if strings.TrimSpace(req.AspectRatio) != "" {
		input["aspect_ratio"] = req.AspectRatio
	}
	if strings.TrimSpace(req.Resolution) != "" {
		input["resolution"] = req.Resolution
	}
	if req.WithAudio {
		input["with_audio"] = true
		input["generate_audio"] = true
	}
	return input
}

func predictionToResult(output any) *modality.VideoResult {
	videoURL := extractOutputURL(output)
	if videoURL == "" {
		return nil
	}
	return &modality.VideoResult{
		VideoURL:    videoURL,
		ContentType: guessVideoContentType(videoURL),
	}
}

func predictionToError(prediction *predictionResponse) *modality.VideoError {
	if prediction == nil {
		return nil
	}
	status := strings.ToLower(strings.TrimSpace(prediction.Status))
	switch status {
	case "failed":
		return &modality.VideoError{
			Type:    "provider_error",
			Code:    "video_generation_failed",
			Message: firstNonEmpty(strings.TrimSpace(prediction.Error), "Replicate video generation failed."),
		}
	case "canceled":
		return &modality.VideoError{
			Type:    "provider_error",
			Code:    "job_cancelled",
			Message: "Replicate prediction was cancelled.",
		}
	default:
		return nil
	}
}

func normalizePredictionStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "starting":
		return "queued"
	case "processing":
		return "processing"
	case "succeeded":
		return "completed"
	case "failed", "canceled":
		return "failed"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func parseProviderModel(value string) (string, string, error) {
	trimmed := strings.Trim(strings.TrimSpace(value), "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_model", "model", "Replicate models must use owner/model identifiers.")
	}
	return parts[0], parts[1], nil
}

func providerModelName(requestModel string, fallbackModel string) string {
	if requestModel == "" {
		requestModel = fallbackModel
	}
	if idx := strings.IndexByte(requestModel, '/'); idx >= 0 {
		return requestModel[idx+1:]
	}
	return requestModel
}

func extractOutputURL(output any) string {
	switch value := output.(type) {
	case string:
		return strings.TrimSpace(value)
	case []any:
		for _, item := range value {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
			if nested, ok := item.(map[string]any); ok {
				for _, key := range []string{"url", "video", "output"} {
					if text, ok := nested[key].(string); ok && strings.TrimSpace(text) != "" {
						return strings.TrimSpace(text)
					}
				}
			}
		}
	case map[string]any:
		for _, key := range []string{"video", "url", "output"} {
			if text, ok := value[key].(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

func firstReference(values []string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseTimestamp(value string) int64 {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	parsed, err := time.Parse(time.RFC3339Nano, trimmed)
	if err != nil {
		return 0
	}
	return parsed.Unix()
}

func guessVideoContentType(raw string) string {
	if parsed, err := url.Parse(raw); err == nil {
		raw = parsed.Path
	}
	switch strings.ToLower(path.Ext(raw)) {
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	default:
		return "video/mp4"
	}
}
