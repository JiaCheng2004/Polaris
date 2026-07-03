package qwen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/apierror"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/openaicompat"
)

type FilesAdapter struct {
	client   *Client
	provider string
}

func NewFilesAdapter(client *Client, provider string) *FilesAdapter {
	return &FilesAdapter{client: client, provider: provider}
}

func (a *FilesAdapter) Upload(ctx context.Context, req *modality.FileUploadRequest) (*modality.ProviderFileHandle, error) {
	if req == nil || req.Body == nil {
		return nil, apierror.NewError(http.StatusBadRequest, "invalid_request_error", "missing_file", "file", "File body is required.")
	}
	data, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("read qwen file body: %w", err)
	}
	filename := strings.TrimSpace(req.Filename)
	if filename == "" {
		filename = "file"
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("create qwen file part: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return nil, fmt.Errorf("write qwen file part: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close qwen multipart writer: %w", err)
	}

	var response qwenFileResponse
	if err := a.multipart(ctx, "/files?purpose=file-extract", body.Bytes(), writer.FormDataContentType(), &response); err != nil {
		return nil, err
	}
	return response.handle(a.provider, req.MimeType, int64(len(data))), nil
}

func (a *FilesAdapter) Get(ctx context.Context, providerFileID string) (*modality.ProviderFileHandle, error) {
	providerFileID = strings.TrimSpace(providerFileID)
	if providerFileID == "" {
		return nil, apierror.NewError(http.StatusBadRequest, "invalid_request_error", "missing_file_id", "file_id", "Provider file id is required.")
	}
	var response qwenFileResponse
	if err := a.jsonRequest(ctx, http.MethodGet, "/files/"+providerFileID, nil, &response); err != nil {
		return nil, err
	}
	return response.handle(a.provider, "", 0), nil
}

func (a *FilesAdapter) Delete(ctx context.Context, providerFileID string) error {
	providerFileID = strings.TrimSpace(providerFileID)
	if providerFileID == "" {
		return apierror.NewError(http.StatusBadRequest, "invalid_request_error", "missing_file_id", "file_id", "Provider file id is required.")
	}
	return a.jsonRequest(ctx, http.MethodDelete, "/files/"+providerFileID, nil, nil)
}

func (a *FilesAdapter) Materialize(ctx context.Context, req *modality.FileMaterializeRequest) (*modality.ProviderFileHandle, error) {
	if req == nil || len(req.InlineSrc) == 0 {
		return nil, apierror.NewError(http.StatusBadRequest, "invalid_request_error", "missing_file_bytes", "file", "Qwen file materialization requires file bytes.")
	}
	return a.Upload(ctx, &modality.FileUploadRequest{
		Body:     bytes.NewReader(req.InlineSrc),
		Size:     req.Size,
		Filename: req.Filename,
		MimeType: req.MimeType,
		Purpose:  modality.FilePurposeFileExtract,
	})
}

type qwenFileResponse struct {
	ID        string `json:"id"`
	Object    string `json:"object"`
	Bytes     int64  `json:"bytes"`
	Filename  string `json:"filename"`
	Purpose   string `json:"purpose"`
	CreatedAt int64  `json:"created_at"`
}

func (r qwenFileResponse) handle(provider string, fallbackMime string, fallbackSize int64) *modality.ProviderFileHandle {
	createdAt := time.Now().UTC()
	if r.CreatedAt > 0 {
		createdAt = time.Unix(r.CreatedAt, 0).UTC()
	}
	size := r.Bytes
	if size == 0 {
		size = fallbackSize
	}
	return &modality.ProviderFileHandle{
		Provider:       provider,
		ProviderFileID: r.ID,
		Purpose:        modality.FilePurposeFileExtract,
		SizeBytes:      size,
		MimeType:       fallbackMime,
		CreatedAt:      createdAt,
	}
}

func (a *FilesAdapter) multipart(ctx context.Context, path string, payload []byte, contentType string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.client.BaseURL()+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build qwen files request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.client.APIKey())
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	resp, err := a.client.HTTPClient().Do(req)
	if err != nil {
		return openaicompat.TranslateTransportError(err, "Qwen")
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= http.StatusBadRequest {
		return a.client.APIError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return apierror.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Qwen returned an invalid file response.")
	}
	return nil
}

func (a *FilesAdapter) jsonRequest(ctx context.Context, method string, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal qwen files request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.client.BaseURL()+path, reader)
	if err != nil {
		return fmt.Errorf("build qwen files request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.client.APIKey())
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.HTTPClient().Do(req)
	if err != nil {
		return openaicompat.TranslateTransportError(err, "Qwen")
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= http.StatusBadRequest {
		return a.client.APIError(resp)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return apierror.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Qwen returned an invalid file response.")
		}
	}
	return nil
}
