package qwen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/openaicompat"
)

type ImageAdapter struct {
	client *Client
	model  string
}

type imageRequest struct {
	Model      string           `json:"model"`
	Input      imageInput       `json:"input"`
	Parameters *imageParameters `json:"parameters,omitempty"`
}

type imageInput struct {
	Messages []imageMessage `json:"messages"`
}

type imageMessage struct {
	Role    string             `json:"role"`
	Content []imageContentPart `json:"content"`
}

type imageContentPart struct {
	Text  string `json:"text,omitempty"`
	Image string `json:"image,omitempty"`
}

type imageParameters struct {
	N    int    `json:"n,omitempty"`
	Size string `json:"size,omitempty"`
}

type imageResponse struct {
	Output struct {
		Choices []struct {
			Message struct {
				Content []struct {
					Text  string `json:"text,omitempty"`
					Image string `json:"image,omitempty"`
				} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	} `json:"output"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func NewImageAdapter(client *Client, model string) *ImageAdapter {
	return &ImageAdapter{
		client: client,
		model:  model,
	}
}

func (a *ImageAdapter) Generate(ctx context.Context, req *modality.ImageRequest) (*modality.ImageResponse, error) {
	content := []imageContentPart{{Text: req.Prompt}}
	references, err := referenceImageParts(req.ReferenceImages)
	if err != nil {
		return nil, err
	}
	content = append(content, references...)

	payload := imageRequest{
		Model: providerImageModelName(req.Model, a.model),
		Input: imageInput{
			Messages: []imageMessage{{
				Role:    "user",
				Content: content,
			}},
		},
		Parameters: imageParametersFromGenerate(req),
	}

	var response imageResponse
	if err := a.json(ctx, "/services/aigc/multimodal-generation/generation", payload, &response); err != nil {
		return nil, err
	}
	return a.translateImageResponse(ctx, response, req.ResponseFormat)
}

func (a *ImageAdapter) Edit(ctx context.Context, req *modality.ImageEditRequest) (*modality.ImageResponse, error) {
	content := []imageContentPart{
		{Image: dataURI(req.ImageType, req.Image)},
		{Text: req.Prompt},
	}
	if len(req.Mask) > 0 {
		content = append(content, imageContentPart{Image: dataURI(req.MaskType, req.Mask)})
	}

	payload := imageRequest{
		Model: providerImageModelName(req.Model, a.model),
		Input: imageInput{
			Messages: []imageMessage{{
				Role:    "user",
				Content: content,
			}},
		},
		Parameters: imageParametersFromEdit(req),
	}

	var response imageResponse
	if err := a.json(ctx, "/services/aigc/multimodal-generation/generation", payload, &response); err != nil {
		return nil, err
	}
	return a.translateImageResponse(ctx, response, req.ResponseFormat)
}

func (a *ImageAdapter) json(ctx context.Context, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal qwen image request: %w", err)
	}

	resp, err := a.do(ctx, path, payload)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		return a.client.APIError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Qwen returned an invalid JSON response.")
	}
	return nil
}

func (a *ImageAdapter) do(ctx context.Context, path string, payload []byte) (*http.Response, error) {
	attempts := a.client.MaxAttempts()
	if attempts <= 0 {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, imageAPIBaseURL(a.client.BaseURL())+path, strings.NewReader(string(payload)))
		if err != nil {
			return nil, fmt.Errorf("build qwen image request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+a.client.APIKey())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := a.client.HTTPClient().Do(req)
		if err != nil {
			lastErr = err
			if attempt < attempts && openaicompat.RetryableTransportError(err) {
				if sleepErr := openaicompat.SleepWithContext(ctx, openaicompat.BackoffDelay(a.client.InitialDelay(), attempt)); sleepErr == nil {
					continue
				}
			}
			return nil, openaicompat.TranslateTransportError(err, "Qwen")
		}
		if openaicompat.RetryableStatus(resp.StatusCode) && attempt < attempts {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if sleepErr := openaicompat.SleepWithContext(ctx, openaicompat.BackoffDelay(a.client.InitialDelay(), attempt)); sleepErr == nil {
				continue
			}
		}
		return resp, nil
	}

	return nil, openaicompat.TranslateTransportError(lastErr, "Qwen")
}

func (a *ImageAdapter) translateImageResponse(ctx context.Context, response imageResponse, requestedFormat string) (*modality.ImageResponse, error) {
	var items []modality.ImageResultData
	for _, choice := range response.Output.Choices {
		revisedPrompt := ""
		for _, content := range choice.Message.Content {
			if strings.TrimSpace(content.Text) != "" && revisedPrompt == "" {
				revisedPrompt = content.Text
				continue
			}
			if strings.TrimSpace(content.Image) == "" {
				continue
			}
			item := modality.ImageResultData{RevisedPrompt: revisedPrompt}
			if requestedFormat == "b64_json" {
				b64, err := a.fetchImageAsBase64(ctx, content.Image)
				if err != nil {
					return nil, err
				}
				item.B64JSON = b64
			} else {
				item.URL = content.Image
			}
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Qwen returned an image response without image data.")
	}
	return &modality.ImageResponse{
		Created: time.Now().Unix(),
		Data:    items,
	}, nil
}

func (a *ImageAdapter) fetchImageAsBase64(ctx context.Context, imageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", fmt.Errorf("build qwen image fetch request: %w", err)
	}
	resp, err := a.client.HTTPClient().Do(req)
	if err != nil {
		return "", openaicompat.TranslateTransportError(err, "Qwen")
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= http.StatusBadRequest {
		return "", httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Qwen image download failed.")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 25<<20))
	if err != nil {
		return "", httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Qwen image download failed.")
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func imageParametersFromGenerate(req *modality.ImageRequest) *imageParameters {
	params := &imageParameters{}
	if req.N > 0 {
		params.N = req.N
	}
	if strings.TrimSpace(req.Size) != "" {
		params.Size = strings.ReplaceAll(req.Size, "x", "*")
	}
	if params.N == 0 && params.Size == "" {
		return nil
	}
	return params
}

func imageParametersFromEdit(req *modality.ImageEditRequest) *imageParameters {
	params := &imageParameters{}
	if req.N > 0 {
		params.N = req.N
	}
	if strings.TrimSpace(req.Size) != "" {
		params.Size = strings.ReplaceAll(req.Size, "x", "*")
	}
	if params.N == 0 && params.Size == "" {
		return nil
	}
	return params
}

func imageAPIBaseURL(baseURL string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	switch {
	case strings.HasSuffix(trimmed, "/compatible-mode/v1"):
		return strings.TrimSuffix(trimmed, "/compatible-mode/v1") + "/api/v1"
	case strings.HasSuffix(trimmed, "/compatible-mode"):
		return strings.TrimSuffix(trimmed, "/compatible-mode") + "/api/v1"
	default:
		return trimmed
	}
}

func providerImageModelName(requestModel string, fallbackModel string) string {
	return openaicompat.ProviderModelName(requestModel, fallbackModel)
}

func referenceImageParts(values []string) ([]imageContentPart, error) {
	parts := make([]imageContentPart, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_reference_image", "reference_images", "Reference images must not be empty.")
		}
		if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") && !strings.HasPrefix(value, "data:") {
			value = "data:image/png;base64," + value
		}
		parts = append(parts, imageContentPart{Image: value})
	}
	return parts, nil
}

func dataURI(contentType string, data []byte) string {
	mimeType := strings.TrimSpace(contentType)
	if mimeType == "" {
		mimeType = "image/png"
	}
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))
}
