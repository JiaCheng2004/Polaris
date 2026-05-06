package google

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	retrypkg "github.com/JiaCheng2004/Polaris/internal/provider/common/retry"
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
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_file", "file", "File body is required.")
	}
	data, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("read google file body: %w", err)
	}
	filename := strings.TrimSpace(req.Filename)
	if filename == "" {
		filename = "file"
	}
	mimeType := strings.TrimSpace(req.MimeType)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	uploadURL, err := a.startResumableUpload(ctx, filename, mimeType, int64(len(data)))
	if err != nil {
		return nil, err
	}
	response, err := a.finalizeResumableUpload(ctx, uploadURL, data)
	if err != nil {
		return nil, err
	}
	return response.handle(a.provider, req.Purpose, mimeType, int64(len(data))), nil
}

func (a *FilesAdapter) Get(ctx context.Context, providerFileID string) (*modality.ProviderFileHandle, error) {
	name := googleFileName(providerFileID)
	if name == "" {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_file_id", "file_id", "Provider file id is required.")
	}
	var response googleFileEnvelope
	if err := a.jsonRequest(ctx, http.MethodGet, "/v1beta/"+name, nil, &response); err != nil {
		return nil, err
	}
	return response.handle(a.provider, modality.FilePurposeUserData, "", 0), nil
}

func (a *FilesAdapter) Delete(ctx context.Context, providerFileID string) error {
	name := googleFileName(providerFileID)
	if name == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_file_id", "file_id", "Provider file id is required.")
	}
	return a.jsonRequest(ctx, http.MethodDelete, "/v1beta/"+name, nil, nil)
}

func (a *FilesAdapter) Materialize(ctx context.Context, req *modality.FileMaterializeRequest) (*modality.ProviderFileHandle, error) {
	if req == nil || len(req.InlineSrc) == 0 {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_file_bytes", "file", "Google file materialization requires file bytes.")
	}
	return a.Upload(ctx, &modality.FileUploadRequest{
		Body:     bytes.NewReader(req.InlineSrc),
		Size:     req.Size,
		Filename: req.Filename,
		MimeType: req.MimeType,
		Purpose:  req.Purpose,
	})
}

type googleFileEnvelope struct {
	File googleFile `json:"file"`
}

type googleFile struct {
	Name           string `json:"name"`
	URI            string `json:"uri"`
	MimeType       string `json:"mimeType"`
	SizeBytes      string `json:"sizeBytes"`
	CreateTime     string `json:"createTime"`
	ExpirationTime string `json:"expirationTime"`
}

func (e googleFileEnvelope) handle(provider string, fallbackPurpose modality.FilePurpose, fallbackMime string, fallbackSize int64) *modality.ProviderFileHandle {
	size := fallbackSize
	if parsed, err := strconv.ParseInt(e.File.SizeBytes, 10, 64); err == nil && parsed > 0 {
		size = parsed
	}
	createdAt := time.Now().UTC()
	if parsed, err := time.Parse(time.RFC3339, e.File.CreateTime); err == nil {
		createdAt = parsed.UTC()
	}
	var expiresAt *time.Time
	if parsed, err := time.Parse(time.RFC3339, e.File.ExpirationTime); err == nil {
		value := parsed.UTC()
		expiresAt = &value
	}
	return &modality.ProviderFileHandle{
		Provider:       provider,
		ProviderFileID: firstNonEmpty(e.File.URI, e.File.Name),
		Purpose:        fallbackPurpose,
		SizeBytes:      size,
		MimeType:       firstNonEmpty(e.File.MimeType, fallbackMime),
		ExpiresAt:      expiresAt,
		CreatedAt:      createdAt,
	}
}

func (a *FilesAdapter) startResumableUpload(ctx context.Context, filename string, mimeType string, size int64) (string, error) {
	payload, err := json.Marshal(map[string]any{"file": map[string]any{"display_name": filename}})
	if err != nil {
		return "", fmt.Errorf("marshal google file metadata: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.client.baseURL+"/upload/v1beta/files?uploadType=resumable", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build google file upload request: %w", err)
	}
	req.Header.Set("x-goog-api-key", a.client.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Upload-Protocol", "resumable")
	req.Header.Set("X-Goog-Upload-Command", "start")
	req.Header.Set("X-Goog-Upload-Header-Content-Length", strconv.FormatInt(size, 10))
	req.Header.Set("X-Goog-Upload-Header-Content-Type", mimeType)

	resp, err := a.client.httpClient.Do(req)
	if err != nil {
		return "", retrypkg.TranslateTransportError(err, "Google")
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= http.StatusBadRequest {
		return "", a.client.apiError(resp)
	}
	uploadURL := strings.TrimSpace(resp.Header.Get("X-Goog-Upload-URL"))
	if uploadURL == "" {
		return "", httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Google did not return a resumable upload URL.")
	}
	return uploadURL, nil
}

func (a *FilesAdapter) finalizeResumableUpload(ctx context.Context, uploadURL string, data []byte) (googleFileEnvelope, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(data))
	if err != nil {
		return googleFileEnvelope{}, fmt.Errorf("build google file upload finalize request: %w", err)
	}
	req.Header.Set("Content-Length", strconv.Itoa(len(data)))
	req.Header.Set("X-Goog-Upload-Offset", "0")
	req.Header.Set("X-Goog-Upload-Command", "upload, finalize")

	resp, err := a.client.httpClient.Do(req)
	if err != nil {
		return googleFileEnvelope{}, retrypkg.TranslateTransportError(err, "Google")
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= http.StatusBadRequest {
		return googleFileEnvelope{}, a.client.apiError(resp)
	}
	var response googleFileEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return googleFileEnvelope{}, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Google returned an invalid file response.")
	}
	return response, nil
}

func (a *FilesAdapter) jsonRequest(ctx context.Context, method string, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal google files request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.client.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build google files request: %w", err)
	}
	req.Header.Set("x-goog-api-key", a.client.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.httpClient.Do(req)
	if err != nil {
		return retrypkg.TranslateTransportError(err, "Google")
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= http.StatusBadRequest {
		return a.client.apiError(resp)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Google returned an invalid file response.")
		}
	}
	return nil
}

func googleFileName(providerFileID string) string {
	trimmed := strings.TrimSpace(providerFileID)
	if strings.HasPrefix(trimmed, "files/") {
		return trimmed
	}
	if idx := strings.LastIndex(trimmed, "/files/"); idx >= 0 {
		return trimmed[idx+1:]
	}
	return trimmed
}
