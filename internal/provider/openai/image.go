package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
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

type imageGenerateRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	Style          string `json:"style,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

type openAIImageResponse struct {
	Created int64             `json:"created"`
	Data    []openAIImageData `json:"data"`
}

type openAIImageData struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

func (a *ImageAdapter) Generate(ctx context.Context, req *modality.ImageRequest) (*modality.ImageResponse, error) {
	providerModel := providerModelName(req.Model, a.model)
	payload := imageGenerateRequest{
		Model:   providerModel,
		Prompt:  req.Prompt,
		N:       req.N,
		Size:    req.Size,
		Quality: req.Quality,
		Style:   req.Style,
	}
	if shouldSendOpenAIImageResponseFormat(providerModel) {
		payload.ResponseFormat = req.ResponseFormat
	}

	var response openAIImageResponse
	if err := a.client.JSON(ctx, "/images/generations", payload, &response); err != nil {
		return nil, err
	}
	return translateOpenAIImageResponse(&response, req.ResponseFormat)
}

func (a *ImageAdapter) Edit(ctx context.Context, req *modality.ImageEditRequest) (*modality.ImageResponse, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	providerModel := providerModelName(req.Model, a.model)

	writeField := func(name, value string) error {
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return writer.WriteField(name, value)
	}

	if err := writeField("model", providerModel); err != nil {
		return nil, fmt.Errorf("write image edit model: %w", err)
	}
	if err := writeField("prompt", req.Prompt); err != nil {
		return nil, fmt.Errorf("write image edit prompt: %w", err)
	}
	if req.N > 0 {
		if err := writeField("n", fmt.Sprintf("%d", req.N)); err != nil {
			return nil, fmt.Errorf("write image edit n: %w", err)
		}
	}
	if err := writeField("size", req.Size); err != nil {
		return nil, fmt.Errorf("write image edit size: %w", err)
	}
	if shouldSendOpenAIImageResponseFormat(providerModel) {
		if err := writeField("response_format", req.ResponseFormat); err != nil {
			return nil, fmt.Errorf("write image edit response_format: %w", err)
		}
	}
	if err := writeFile(writer, "image", req.ImageFilename, req.ImageType, req.Image); err != nil {
		return nil, err
	}
	if len(req.Mask) > 0 {
		if err := writeFile(writer, "mask", req.MaskFilename, req.MaskType, req.Mask); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	resp, err := a.multipart(ctx, "/images/edits", body.Bytes(), writer.FormDataContentType())
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var response openAIImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "OpenAI returned an invalid JSON response.")
	}
	return translateOpenAIImageResponse(&response, req.ResponseFormat)
}

func (a *ImageAdapter) multipart(ctx context.Context, path string, payload []byte, contentType string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.client.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build openai multipart request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.client.apiKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.httpClient.Do(req)
	if err != nil {
		return nil, translateTransportError(err, "OpenAI")
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer func() {
			_ = resp.Body.Close()
		}()
		return nil, a.client.apiError(resp)
	}
	return resp, nil
}

func writeFile(writer *multipart.Writer, fieldName string, filename string, contentType string, data []byte) error {
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, filename))
	if strings.TrimSpace(contentType) != "" {
		header.Set("Content-Type", contentType)
	}
	part, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("create multipart part %s: %w", fieldName, err)
	}
	if _, err := io.Copy(part, bytes.NewReader(data)); err != nil {
		return fmt.Errorf("write multipart part %s: %w", fieldName, err)
	}
	return nil
}

func shouldSendOpenAIImageResponseFormat(model string) bool {
	return !strings.HasPrefix(strings.TrimSpace(model), "gpt-image-")
}

func translateOpenAIImageResponse(response *openAIImageResponse, requestedFormat string) (*modality.ImageResponse, error) {
	if response == nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "OpenAI returned an image response without image data.")
	}

	created := response.Created
	if created == 0 {
		created = time.Now().Unix()
	}

	items := make([]modality.ImageResultData, 0, len(response.Data))
	for _, data := range response.Data {
		item := modality.ImageResultData{RevisedPrompt: data.RevisedPrompt}
		switch requestedFormat {
		case "b64_json":
			if strings.TrimSpace(data.B64JSON) == "" {
				return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "OpenAI returned an image response without inline image data.")
			}
			item.B64JSON = data.B64JSON
		default:
			if strings.TrimSpace(data.URL) != "" {
				item.URL = data.URL
			} else if strings.TrimSpace(data.B64JSON) != "" {
				item.URL = openAIImageDataURI("image/png", data.B64JSON)
			} else {
				return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "OpenAI returned an image response without image data.")
			}
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "OpenAI returned an image response without image data.")
	}
	return &modality.ImageResponse{
		Created: created,
		Data:    items,
	}, nil
}

func openAIImageDataURI(mimeType string, data string) string {
	if strings.TrimSpace(mimeType) == "" {
		mimeType = "image/png"
	}
	return fmt.Sprintf("data:%s;base64,%s", mimeType, data)
}
