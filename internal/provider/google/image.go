package google

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

type ImageAdapter struct {
	client *Client
	model  string
}

func NewImageAdapter(client *Client, model string) *ImageAdapter {
	return &ImageAdapter{
		client: client,
		model:  model,
	}
}

type imageGenerateContentRequest struct {
	Contents         []googleContent        `json:"contents"`
	GenerationConfig *imageGenerationConfig `json:"generationConfig,omitempty"`
}

type imageGenerationConfig struct {
	ResponseModalities []string `json:"responseModalities,omitempty"`
}

func (a *ImageAdapter) Generate(ctx context.Context, req *modality.ImageRequest) (*modality.ImageResponse, error) {
	parts := []googlePart{{Text: req.Prompt}}
	references, err := translateReferenceImages(req.ReferenceImages)
	if err != nil {
		return nil, err
	}
	parts = append(parts, references...)

	payload := imageGenerateContentRequest{
		Contents: []googleContent{
			{Role: "user", Parts: parts},
		},
		GenerationConfig: &imageGenerationConfig{ResponseModalities: []string{"TEXT", "IMAGE"}},
	}

	var response generateContentResponse
	if err := a.client.JSON(ctx, a.generatePath(req.Model), payload, &response); err != nil {
		return nil, err
	}
	return translateImageResponse(response, req.ResponseFormat, req.Model, a.model)
}

func (a *ImageAdapter) Edit(ctx context.Context, req *modality.ImageEditRequest) (*modality.ImageResponse, error) {
	parts := []googlePart{{Text: req.Prompt}}
	imagePart, err := inlineImagePart(req.ImageType, req.Image)
	if err != nil {
		return nil, err
	}
	parts = append(parts, imagePart)
	if len(req.Mask) > 0 {
		maskPart, maskErr := inlineImagePart(req.MaskType, req.Mask)
		if maskErr != nil {
			return nil, maskErr
		}
		parts = append(parts, maskPart)
	}

	payload := imageGenerateContentRequest{
		Contents: []googleContent{
			{Role: "user", Parts: parts},
		},
		GenerationConfig: &imageGenerationConfig{ResponseModalities: []string{"TEXT", "IMAGE"}},
	}

	var response generateContentResponse
	if err := a.client.JSON(ctx, a.generatePath(req.Model), payload, &response); err != nil {
		return nil, err
	}
	return translateImageResponse(response, req.ResponseFormat, req.Model, a.model)
}

func (a *ImageAdapter) generatePath(requestModel string) string {
	return "/v1beta/models/" + providerImageModelName(requestModel, a.model) + ":generateContent"
}

func providerImageModelName(requestModel string, fallbackModel string) string {
	name := providerModelName(requestModel, fallbackModel)
	switch name {
	case "nano-banana-2":
		return "gemini-2.5-flash-image"
	case "nano-banana-pro":
		return "gemini-3-pro-image-preview"
	default:
		return name
	}
}

func translateReferenceImages(values []string) ([]googlePart, error) {
	parts := make([]googlePart, 0, len(values))
	for _, value := range values {
		part, err := translateReferenceImage(value)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	return parts, nil
}

func translateReferenceImage(raw string) (googlePart, error) {
	if strings.HasPrefix(raw, "data:") {
		return translateImagePart(raw)
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "gs://") {
		return translateImagePart(raw)
	}
	if strings.TrimSpace(raw) == "" {
		return googlePart{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_reference_image", "reference_images", "Reference images must be URLs, data URIs, or base64 strings.")
	}
	return googlePart{
		InlineData: &googleBlob{
			MimeType: "image/png",
			Data:     raw,
		},
	}, nil
}

func inlineImagePart(contentType string, data []byte) (googlePart, error) {
	if len(data) == 0 {
		return googlePart{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_image", "image", "Image payload is required.")
	}
	mimeType := strings.TrimSpace(contentType)
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return googlePart{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_image", "image", "Google image models require image input.")
	}
	return googlePart{
		InlineData: &googleBlob{
			MimeType: mimeType,
			Data:     base64.StdEncoding.EncodeToString(data),
		},
	}, nil
}

func translateImageResponse(response generateContentResponse, requestedFormat string, canonicalModel string, fallbackModel string) (*modality.ImageResponse, error) {
	candidate := firstCandidate(response.Candidates)
	revisedPrompt := ""
	var items []modality.ImageResultData

	for _, part := range candidate.Content.Parts {
		if strings.TrimSpace(part.Text) != "" {
			if revisedPrompt == "" {
				revisedPrompt = part.Text
			}
			continue
		}
		if part.InlineData == nil || strings.TrimSpace(part.InlineData.Data) == "" {
			continue
		}
		item := modality.ImageResultData{RevisedPrompt: revisedPrompt}
		if requestedFormat == "b64_json" {
			item.B64JSON = part.InlineData.Data
		} else {
			item.URL = dataURI(part.InlineData.MimeType, part.InlineData.Data)
		}
		items = append(items, item)
	}

	if len(items) == 0 {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Google returned an image response without image data.")
	}
	return &modality.ImageResponse{
		Created: time.Now().Unix(),
		Data:    items,
	}, nil
}

func dataURI(mimeType string, data string) string {
	if strings.TrimSpace(mimeType) == "" {
		mimeType = "image/png"
	}
	return fmt.Sprintf("data:%s;base64,%s", mimeType, data)
}
