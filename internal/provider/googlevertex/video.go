package googlevertex

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

type VideoAdapter struct {
	client *Client
	model  string
}

type vertexVideoRequest struct {
	Instances  []vertexVideoInstance `json:"instances"`
	Parameters vertexVideoParams     `json:"parameters"`
}

type vertexVideoInstance struct {
	Prompt    string           `json:"prompt,omitempty"`
	Image     *vertexImageData `json:"image,omitempty"`
	LastFrame *vertexImageData `json:"lastFrame,omitempty"`
}

type vertexImageData struct {
	MimeType           string `json:"mimeType"`
	BytesBase64Encoded string `json:"bytesBase64Encoded,omitempty"`
	GCSURI             string `json:"gcsUri,omitempty"`
}

type vertexVideoParams struct {
	DurationSeconds int    `json:"durationSeconds,omitempty"`
	AspectRatio     string `json:"aspectRatio,omitempty"`
	Resolution      string `json:"resolution,omitempty"`
	GenerateAudio   bool   `json:"generateAudio"`
	SampleCount     int    `json:"sampleCount,omitempty"`
}

type vertexOperation struct {
	Name     string                `json:"name"`
	Done     bool                  `json:"done"`
	Response *vertexVideoResponse  `json:"response"`
	Error    *vertexOperationError `json:"error"`
}

type vertexOperationError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

type vertexVideoResponse struct {
	Videos []vertexVideoOutput `json:"videos"`
}

type vertexVideoOutput struct {
	MimeType           string `json:"mimeType"`
	GCSURI             string `json:"gcsUri"`
	BytesBase64Encoded string `json:"bytesBase64Encoded"`
}

func NewVideoAdapter(client *Client, model string) *VideoAdapter {
	return &VideoAdapter{
		client: client,
		model:  model,
	}
}

func (a *VideoAdapter) Generate(ctx context.Context, req *modality.VideoRequest) (*modality.VideoJob, error) {
	instance := vertexVideoInstance{
		Prompt: strings.TrimSpace(req.Prompt),
	}
	if strings.TrimSpace(req.FirstFrame) != "" {
		image, err := vertexImageFromReference(ctx, a.client, req.FirstFrame)
		if err != nil {
			return nil, err
		}
		instance.Image = image
	}
	if strings.TrimSpace(req.LastFrame) != "" {
		image, err := vertexImageFromReference(ctx, a.client, req.LastFrame)
		if err != nil {
			return nil, err
		}
		instance.LastFrame = image
	}

	payload := vertexVideoRequest{
		Instances: []vertexVideoInstance{instance},
		Parameters: vertexVideoParams{
			DurationSeconds: req.Duration,
			AspectRatio:     req.AspectRatio,
			Resolution:      req.Resolution,
			GenerateAudio:   req.WithAudio,
			SampleCount:     1,
		},
	}

	var operation vertexOperation
	path := "/v1/" + a.client.endpoint(providerModelName(req.Model, a.model)) + ":predictLongRunning"
	if err := a.client.JSON(ctx, http.MethodPost, path, payload, &operation); err != nil {
		return nil, err
	}
	if strings.TrimSpace(operation.Name) == "" {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Google Vertex did not return an operation name.")
	}
	return &modality.VideoJob{
		JobID:  operation.Name,
		Status: "queued",
		Model:  firstNonEmpty(req.Model, a.model),
	}, nil
}

func (a *VideoAdapter) GetStatus(ctx context.Context, jobID string) (*modality.VideoStatus, error) {
	operation, err := a.fetchOperation(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if !operation.Done {
		return &modality.VideoStatus{
			JobID:  jobID,
			Status: "processing",
		}, nil
	}
	if operation.Error != nil {
		code := strings.TrimSpace(operation.Error.Status)
		message := firstNonEmpty(operation.Error.Message, "Google Vertex video generation failed.")
		if operation.Error.Code == 1 {
			code = "job_cancelled"
		}
		return &modality.VideoStatus{
			JobID:  jobID,
			Status: "failed",
			Error: &modality.VideoError{
				Type:    "provider_error",
				Code:    firstNonEmpty(strings.ToLower(code), "video_generation_failed"),
				Message: message,
			},
		}, nil
	}

	result := &modality.VideoResult{
		ContentType: "video/mp4",
	}
	if operation.Response != nil && len(operation.Response.Videos) > 0 {
		video := operation.Response.Videos[0]
		if strings.TrimSpace(video.MimeType) != "" {
			result.ContentType = video.MimeType
		}
		if strings.TrimSpace(video.GCSURI) != "" {
			result.VideoURL = video.GCSURI
		}
	}
	return &modality.VideoStatus{
		JobID:  jobID,
		Status: "completed",
		Result: result,
	}, nil
}

func (a *VideoAdapter) Cancel(ctx context.Context, jobID string) error {
	resp, err := a.client.do(ctx, http.MethodPost, "/v1/"+jobID+":cancel", map[string]any{}, "application/json")
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return httputil.NewError(http.StatusNotFound, "invalid_request_error", "job_not_found", "id", "Video job was not found.")
	}
	if resp.StatusCode == http.StatusConflict {
		return httputil.NewError(http.StatusConflict, "invalid_request_error", "job_immutable", "id", "Completed or failed video jobs cannot be cancelled.")
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return a.client.apiError(resp)
	}
	return nil
}

func (a *VideoAdapter) Download(ctx context.Context, jobID string, status *modality.VideoStatus) (*modality.VideoAsset, error) {
	operation, err := a.fetchOperation(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if operation.Response == nil || len(operation.Response.Videos) == 0 {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Google Vertex returned no generated video.")
	}
	video := operation.Response.Videos[0]
	if strings.TrimSpace(video.BytesBase64Encoded) == "" {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Google Vertex did not return inline video bytes.")
	}
	data, err := base64.StdEncoding.DecodeString(video.BytesBase64Encoded)
	if err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Google Vertex returned invalid video bytes.")
	}
	contentType := strings.TrimSpace(video.MimeType)
	if contentType == "" && status != nil && status.Result != nil {
		contentType = strings.TrimSpace(status.Result.ContentType)
	}
	if contentType == "" {
		contentType = "video/mp4"
	}
	return &modality.VideoAsset{Data: data, ContentType: contentType}, nil
}

func (a *VideoAdapter) fetchOperation(ctx context.Context, jobID string) (*vertexOperation, error) {
	var operation vertexOperation
	path := "/v1/" + a.client.endpoint(providerModelName(a.model, a.model)) + ":fetchPredictOperation"
	body := map[string]string{"operationName": jobID}
	if err := a.client.JSON(ctx, http.MethodPost, path, body, &operation); err != nil {
		var apiErr *httputil.APIError
		if strings.Contains(strings.ToLower(err.Error()), "not found") && strings.Contains(strings.ToLower(err.Error()), "operation") {
			return nil, httputil.NewError(http.StatusNotFound, "invalid_request_error", "job_not_found", "id", "Video job was not found.")
		}
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return nil, httputil.NewError(http.StatusNotFound, "invalid_request_error", "job_not_found", "id", "Video job was not found.")
		}
		return nil, err
	}
	return &operation, nil
}

func vertexImageFromReference(ctx context.Context, client *Client, value string) (*vertexImageData, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if strings.HasPrefix(value, "gs://") {
		return &vertexImageData{
			MimeType: "image/png",
			GCSURI:   value,
		}, nil
	}
	if strings.HasPrefix(value, "data:") {
		mimeType, data, err := decodeDataURI(value)
		if err != nil {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_first_frame", "first_frame", "Image data URI is invalid.")
		}
		return &vertexImageData{
			MimeType:           mimeType,
			BytesBase64Encoded: data,
		}, nil
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, value, nil)
		if err != nil {
			return nil, fmt.Errorf("build google vertex image request: %w", err)
		}
		resp, err := client.httpClient.Do(req)
		if err != nil {
			return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_transport_error", "", "Failed to fetch the input image.")
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		if resp.StatusCode >= http.StatusBadRequest {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_first_frame", "first_frame", "Input image URL could not be fetched.")
		}
		payload, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Failed to read the input image.")
		}
		mimeType := strings.TrimSpace(resp.Header.Get("Content-Type"))
		if mimeType == "" {
			mimeType = http.DetectContentType(payload)
		}
		return &vertexImageData{
			MimeType:           mimeType,
			BytesBase64Encoded: base64.StdEncoding.EncodeToString(payload),
		}, nil
	}
	return &vertexImageData{
		MimeType:           "image/png",
		BytesBase64Encoded: value,
	}, nil
}

func decodeDataURI(value string) (string, string, error) {
	parts := strings.SplitN(value, ",", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid data uri")
	}
	meta := strings.TrimPrefix(parts[0], "data:")
	mimeType := "application/octet-stream"
	if idx := strings.Index(meta, ";"); idx >= 0 {
		mimeType = meta[:idx]
	} else if meta != "" {
		mimeType = meta
	}
	return mimeType, parts[1], nil
}

func providerModelName(requestModel string, fallbackModel string) string {
	canonicalize := func(model string) string {
		model = strings.TrimSpace(model)
		if model == "" {
			return ""
		}
		if strings.HasPrefix(model, "veo-") && !strings.Contains(model, "@") {
			return model + "@default"
		}
		return model
	}
	if requestModel == "" {
		return canonicalize(strings.TrimPrefix(fallbackModel[strings.Index(fallbackModel, "/")+1:], "/"))
	}
	if idx := strings.IndexByte(requestModel, '/'); idx >= 0 {
		return canonicalize(requestModel[idx+1:])
	}
	if fallbackModel != "" {
		if idx := strings.IndexByte(fallbackModel, '/'); idx >= 0 {
			return canonicalize(fallbackModel[idx+1:])
		}
	}
	return canonicalize(requestModel)
}
