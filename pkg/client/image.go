package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
)

func (c *Client) GenerateImage(ctx context.Context, req *ImageGenerationRequest) (*ImageResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	var response ImageResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/images/generations", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) EditImage(ctx context.Context, req *ImageEditRequest) (*ImageResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	writeField := func(name string, value string) error {
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return writer.WriteField(name, value)
	}

	if err := writeField("model", req.Model); err != nil {
		return nil, fmt.Errorf("write model field: %w", err)
	}
	if err := writeRoutingField(writer, req.Routing); err != nil {
		return nil, fmt.Errorf("write routing field: %w", err)
	}
	if err := writeField("prompt", req.Prompt); err != nil {
		return nil, fmt.Errorf("write prompt field: %w", err)
	}
	if req.N > 0 {
		if err := writeField("n", fmt.Sprintf("%d", req.N)); err != nil {
			return nil, fmt.Errorf("write n field: %w", err)
		}
	}
	if err := writeField("size", req.Size); err != nil {
		return nil, fmt.Errorf("write size field: %w", err)
	}
	if err := writeField("response_format", req.ResponseFormat); err != nil {
		return nil, fmt.Errorf("write response_format field: %w", err)
	}
	if err := writeMultipartFile(writer, "image", defaultFilename(req.ImageFilename, "image.png"), defaultContentType(req.ImageContentType, req.Image), req.Image); err != nil {
		return nil, err
	}
	if len(req.Mask) > 0 {
		if err := writeMultipartFile(writer, "mask", defaultFilename(req.MaskFilename, "mask.png"), defaultContentType(req.MaskContentType, req.Mask), req.Mask); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart body: %w", err)
	}

	resp, err := c.do(ctx, http.MethodPost, "/v1/images/edits", nil, bytes.NewReader(body.Bytes()), writer.FormDataContentType(), "application/json")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, decodeAPIErrorResponse(resp)
	}

	var response ImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode image edit response: %w", err)
	}
	return &response, nil
}

func writeMultipartFile(writer *multipart.Writer, fieldName string, filename string, contentType string, data []byte) error {
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, filename))
	if strings.TrimSpace(contentType) != "" {
		header.Set("Content-Type", contentType)
	}

	part, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("create multipart part %s: %w", fieldName, err)
	}
	if _, err := part.Write(data); err != nil {
		return fmt.Errorf("write multipart part %s: %w", fieldName, err)
	}
	return nil
}

func defaultFilename(filename string, fallback string) string {
	if strings.TrimSpace(filename) == "" {
		return fallback
	}
	return filename
}

func defaultContentType(contentType string, data []byte) string {
	if strings.TrimSpace(contentType) != "" {
		return contentType
	}
	if len(data) == 0 {
		return ""
	}
	return http.DetectContentType(data)
}

func writeRoutingField(writer *multipart.Writer, routing *RoutingOptions) error {
	if routing == nil {
		return nil
	}
	payload, err := json.Marshal(routing)
	if err != nil {
		return fmt.Errorf("marshal routing JSON: %w", err)
	}
	if string(payload) == "null" || string(payload) == "{}" {
		return nil
	}
	return writer.WriteField("routing", string(payload))
}
