package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
)

func (c *Client) CreateSpeech(ctx context.Context, req *SpeechRequest) (*Audio, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	resp, err := c.do(ctx, http.MethodPost, "/v1/audio/speech", nil, req, "application/json", "*/*")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, decodeAPIErrorResponse(resp)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read speech response: %w", err)
	}

	return &Audio{
		Data:        data,
		ContentType: resp.Header.Get("Content-Type"),
	}, nil
}

func (c *Client) CreateTranscription(ctx context.Context, req *TranscriptionRequest) (*TranscriptionResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	writeField := func(name string, value string) error {
		if value == "" {
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
	if err := writeField("language", req.Language); err != nil {
		return nil, fmt.Errorf("write language field: %w", err)
	}
	if err := writeField("response_format", req.ResponseFormat); err != nil {
		return nil, fmt.Errorf("write response_format field: %w", err)
	}
	if req.Temperature != nil {
		if err := writeField("temperature", strconv.FormatFloat(*req.Temperature, 'f', -1, 64)); err != nil {
			return nil, fmt.Errorf("write temperature field: %w", err)
		}
	}
	if err := writeMultipartFile(writer, "file", defaultFilename(req.Filename, "audio.wav"), defaultContentType(req.ContentType, req.File), req.File); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart body: %w", err)
	}

	resp, err := c.do(ctx, http.MethodPost, "/v1/audio/transcriptions", nil, bytes.NewReader(body.Bytes()), writer.FormDataContentType(), "*/*")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, decodeAPIErrorResponse(resp)
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read transcription response: %w", err)
	}

	requestedFormat := req.ResponseFormat
	if requestedFormat == "" {
		requestedFormat = "json"
	}

	response := &TranscriptionResponse{
		Raw:         append([]byte(nil), payload...),
		ContentType: resp.Header.Get("Content-Type"),
		Format:      requestedFormat,
	}
	if requestedFormat != "json" {
		response.Text = string(payload)
		return response, nil
	}

	if err := json.Unmarshal(payload, response); err != nil {
		return nil, fmt.Errorf("decode transcription JSON response: %w", err)
	}
	return response, nil
}
