package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

type VideoAdapter struct {
	client *Client
	model  string
}

type openAIVideoCreateRequest struct {
	Model          string                     `json:"model,omitempty"`
	Prompt         string                     `json:"prompt"`
	Seconds        string                     `json:"seconds,omitempty"`
	Size           string                     `json:"size,omitempty"`
	InputReference *openAIVideoInputReference `json:"input_reference,omitempty"`
}

type openAIVideoInputReference struct {
	ImageURL string `json:"image_url,omitempty"`
}

type openAIVideoObject struct {
	ID          string            `json:"id"`
	Object      string            `json:"object"`
	Model       string            `json:"model"`
	Status      string            `json:"status"`
	Progress    float64           `json:"progress"`
	CreatedAt   int64             `json:"created_at"`
	CompletedAt int64             `json:"completed_at"`
	ExpiresAt   int64             `json:"expires_at"`
	Size        string            `json:"size"`
	Seconds     string            `json:"seconds"`
	Error       *openAIVideoError `json:"error"`
}

type openAIVideoError struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewVideoAdapter(client *Client, model string) *VideoAdapter {
	return &VideoAdapter{
		client: client,
		model:  model,
	}
}

func (a *VideoAdapter) Generate(ctx context.Context, req *modality.VideoRequest) (*modality.VideoJob, error) {
	payload := openAIVideoCreateRequest{
		Model:  providerModelName(req.Model, a.model),
		Prompt: req.Prompt,
	}
	if req.Duration > 0 {
		payload.Seconds = strconv.Itoa(req.Duration)
	}
	if size := openAIVideoSize(req); size != "" {
		payload.Size = size
	}
	if normalized := normalizeOpenAIImageReference(req.FirstFrame); normalized != "" {
		payload.InputReference = &openAIVideoInputReference{ImageURL: normalized}
	}

	var response openAIVideoObject
	if err := a.client.JSON(ctx, "/videos", payload, &response); err != nil {
		return nil, err
	}
	if strings.TrimSpace(response.ID) == "" {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "OpenAI did not return a video id.")
	}

	return &modality.VideoJob{
		JobID:  response.ID,
		Status: normalizeOpenAIVideoStatus(response.Status),
		Model:  firstNonEmpty(req.Model, a.model),
	}, nil
}

func (a *VideoAdapter) GetStatus(ctx context.Context, jobID string) (*modality.VideoStatus, error) {
	resp, err := a.request(ctx, http.MethodGet, "/videos/"+jobID, nil)
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

	var raw openAIVideoObject
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "OpenAI returned an invalid video response.")
	}

	status := &modality.VideoStatus{
		JobID:       jobID,
		Status:      normalizeOpenAIVideoStatus(raw.Status),
		Progress:    normalizeOpenAIVideoProgress(raw.Progress),
		CreatedAt:   raw.CreatedAt,
		CompletedAt: raw.CompletedAt,
		ExpiresAt:   raw.ExpiresAt,
	}
	if raw.Error != nil {
		status.Error = &modality.VideoError{
			Type:    firstNonEmpty(raw.Error.Type, "provider_error"),
			Code:    strings.TrimSpace(raw.Error.Code),
			Message: firstNonEmpty(raw.Error.Message, "OpenAI video generation failed."),
		}
	}
	if status.Status == "completed" {
		width, height := parseOpenAIVideoSize(raw.Size)
		duration, _ := strconv.Atoi(strings.TrimSpace(raw.Seconds))
		status.Result = &modality.VideoResult{
			ContentType: "video/mp4",
			Duration:    duration,
			Width:       width,
			Height:      height,
		}
	}
	if status.Status == "failed" && status.Error == nil {
		status.Error = &modality.VideoError{
			Type:    "provider_error",
			Code:    "video_generation_failed",
			Message: "OpenAI video generation failed.",
		}
	}
	return status, nil
}

func (a *VideoAdapter) Cancel(context.Context, string) error {
	return httputil.NewError(http.StatusConflict, "invalid_request_error", "job_not_cancelable", "id", "This video job cannot be cancelled.")
}

func (a *VideoAdapter) Download(ctx context.Context, jobID string, status *modality.VideoStatus) (*modality.VideoAsset, error) {
	resp, err := a.request(ctx, http.MethodGet, "/videos/"+jobID+"/content", nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusGone {
		return nil, httputil.NewError(http.StatusGone, "invalid_request_error", "asset_expired", "id", "Video asset has expired.")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, httputil.NewError(http.StatusNotFound, "invalid_request_error", "job_not_found", "id", "Video job was not found.")
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, a.client.apiError(resp)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "OpenAI returned an invalid video asset.")
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" && status != nil && status.Result != nil {
		contentType = strings.TrimSpace(status.Result.ContentType)
	}
	if contentType == "" {
		contentType = "video/mp4"
	}
	return &modality.VideoAsset{Data: data, ContentType: contentType}, nil
}

func (a *VideoAdapter) request(ctx context.Context, method string, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, a.client.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build openai video request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.client.apiKey)
	req.Header.Set("Accept", "*/*")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.httpClient.Do(req)
	if err != nil {
		return nil, translateTransportError(err, "OpenAI")
	}
	return resp, nil
}

func openAIVideoSize(req *modality.VideoRequest) string {
	resolution := strings.TrimSpace(req.Resolution)
	aspectRatio := strings.TrimSpace(req.AspectRatio)
	switch {
	case resolution == "" && aspectRatio == "":
		return ""
	case firstNonEmpty(resolution, "720p") == "720p" && aspectRatio == "16:9":
		return "1280x720"
	case firstNonEmpty(resolution, "720p") == "720p" && aspectRatio == "9:16":
		return "720x1280"
	default:
		return ""
	}
}

func normalizeOpenAIImageReference(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "data:") {
		return value
	}
	return "data:image/png;base64," + value
}

func normalizeOpenAIVideoStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "queued", "pending":
		return "queued"
	case "processing", "in_progress", "in-progress", "running":
		return "processing"
	case "completed", "succeeded", "success":
		return "completed"
	case "failed", "cancelled", "canceled":
		return "failed"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeOpenAIVideoProgress(value float64) float64 {
	switch {
	case value <= 0:
		return 0
	case value > 1:
		return value / 100
	default:
		return value
	}
}

func parseOpenAIVideoSize(value string) (int, int) {
	parts := strings.Split(strings.TrimSpace(value), "x")
	if len(parts) != 2 {
		return 0, 0
	}
	width, _ := strconv.Atoi(parts[0])
	height, _ := strconv.Atoi(parts[1])
	return width, height
}
