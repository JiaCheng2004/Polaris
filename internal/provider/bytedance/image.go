package bytedance

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/modality"
)

type ImageAdapter struct {
	client   *Client
	model    string
	endpoint string
}

type imageRequest struct {
	Model                           string                            `json:"model"`
	Prompt                          string                            `json:"prompt"`
	Image                           any                               `json:"image,omitempty"`
	Size                            string                            `json:"size,omitempty"`
	ResponseFormat                  string                            `json:"response_format,omitempty"`
	SequentialImageGeneration       string                            `json:"sequential_image_generation,omitempty"`
	SequentialImageGenerationConfig *sequentialImageGenerationOptions `json:"sequential_image_generation_options,omitempty"`
}

type sequentialImageGenerationOptions struct {
	MaxImages int `json:"max_images"`
}

type imageResponse struct {
	Model   string               `json:"model"`
	Created int64                `json:"created"`
	Data    []bytedanceImageData `json:"data"`
}

type bytedanceImageData struct {
	URL     string `json:"url,omitempty"`
	B64JSON string `json:"b64_json,omitempty"`
}

func NewImageAdapter(client *Client, model string, endpoint string) *ImageAdapter {
	return &ImageAdapter{
		client:   client,
		model:    model,
		endpoint: endpoint,
	}
}

func (a *ImageAdapter) Generate(ctx context.Context, req *modality.ImageRequest) (*modality.ImageResponse, error) {
	references := normalizeReferenceImages(req.ReferenceImages)
	payload := imageRequest{
		Model:          providerImageModelName(req.Model, a.model),
		Prompt:         req.Prompt,
		Image:          referenceImageField(references),
		Size:           req.Size,
		ResponseFormat: req.ResponseFormat,
	}
	if req.N > 1 {
		payload.SequentialImageGeneration = "auto"
		payload.SequentialImageGenerationConfig = &sequentialImageGenerationOptions{MaxImages: req.N}
	}

	var response imageResponse
	if err := a.client.JSON(ctx, a.endpoint, "/images/generations", payload, &response); err != nil {
		return nil, err
	}
	return normalizeImageResponse(response, req.Model, a.model), nil
}

func (a *ImageAdapter) Edit(ctx context.Context, req *modality.ImageEditRequest) (*modality.ImageResponse, error) {
	references := []string{dataURI(req.ImageType, req.Image)}
	if len(req.Mask) > 0 {
		references = append(references, dataURI(req.MaskType, req.Mask))
	}

	payload := imageRequest{
		Model:          providerImageModelName(req.Model, a.model),
		Prompt:         req.Prompt,
		Image:          referenceImageField(references),
		Size:           req.Size,
		ResponseFormat: req.ResponseFormat,
	}
	if req.N > 1 {
		payload.SequentialImageGeneration = "auto"
		payload.SequentialImageGenerationConfig = &sequentialImageGenerationOptions{MaxImages: req.N}
	}

	var response imageResponse
	if err := a.client.JSON(ctx, a.endpoint, "/images/generations", payload, &response); err != nil {
		return nil, err
	}
	return normalizeImageResponse(response, req.Model, a.model), nil
}

func referenceImageField(values []string) any {
	switch len(values) {
	case 0:
		return nil
	case 1:
		return values[0]
	default:
		return values
	}
}

func normalizeReferenceImages(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if isStructuredMediaReference(value) {
			normalized = append(normalized, value)
			continue
		}
		normalized = append(normalized, "data:image/png;base64,"+value)
	}
	return normalized
}

func dataURI(contentType string, data []byte) string {
	mimeType := strings.TrimSpace(contentType)
	if mimeType == "" {
		mimeType = "image/png"
	}
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))
}

func normalizeImageResponse(response imageResponse, canonicalModel string, fallbackModel string) *modality.ImageResponse {
	items := make([]modality.ImageResultData, 0, len(response.Data))
	for _, item := range response.Data {
		items = append(items, modality.ImageResultData{
			URL:     item.URL,
			B64JSON: item.B64JSON,
		})
	}
	created := response.Created
	if created == 0 {
		created = time.Now().Unix()
	}
	return &modality.ImageResponse{
		Created: created,
		Data:    items,
	}
}
