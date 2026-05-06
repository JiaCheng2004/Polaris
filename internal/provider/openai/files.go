package openai

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

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
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

	filename := strings.TrimSpace(req.Filename)
	if filename == "" {
		filename = "file"
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("read file upload body: %w", err)
	}

	var multipartBody bytes.Buffer
	writer := multipart.NewWriter(&multipartBody)
	if err := writer.WriteField("purpose", openAIFilePurpose(req.Purpose)); err != nil {
		return nil, fmt.Errorf("write file purpose: %w", err)
	}
	if err := writeFile(writer, "file", filename, req.MimeType, body); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close file multipart writer: %w", err)
	}

	var response openAIFileResponse
	if err := a.client.filesMultipart(ctx, "/files", multipartBody.Bytes(), writer.FormDataContentType(), &response); err != nil {
		return nil, err
	}
	return response.handle(a.provider, req.Purpose, req.MimeType, int64(len(body))), nil
}

func (a *FilesAdapter) Get(ctx context.Context, providerFileID string) (*modality.ProviderFileHandle, error) {
	providerFileID = strings.TrimSpace(providerFileID)
	if providerFileID == "" {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_file_id", "file_id", "Provider file id is required.")
	}
	var response openAIFileResponse
	if err := a.client.filesJSON(ctx, http.MethodGet, "/files/"+providerFileID, nil, &response); err != nil {
		return nil, err
	}
	return response.handle(a.provider, modality.FilePurposeUserData, "", 0), nil
}

func (a *FilesAdapter) Delete(ctx context.Context, providerFileID string) error {
	providerFileID = strings.TrimSpace(providerFileID)
	if providerFileID == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_file_id", "file_id", "Provider file id is required.")
	}
	var response struct {
		Deleted bool `json:"deleted"`
	}
	return a.client.filesJSON(ctx, http.MethodDelete, "/files/"+providerFileID, nil, &response)
}

func (a *FilesAdapter) Materialize(ctx context.Context, req *modality.FileMaterializeRequest) (*modality.ProviderFileHandle, error) {
	if req == nil {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_file", "file", "File materialization request is required.")
	}
	if len(req.InlineSrc) == 0 {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_file_bytes", "file", "OpenAI file materialization requires file bytes.")
	}
	return a.Upload(ctx, &modality.FileUploadRequest{
		Body:     bytes.NewReader(req.InlineSrc),
		Size:     req.Size,
		Filename: req.Filename,
		MimeType: req.MimeType,
		Purpose:  req.Purpose,
	})
}

type openAIFileResponse struct {
	ID        string `json:"id"`
	Object    string `json:"object"`
	Bytes     int64  `json:"bytes"`
	Filename  string `json:"filename"`
	Purpose   string `json:"purpose"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
}

func (r openAIFileResponse) handle(provider string, fallbackPurpose modality.FilePurpose, fallbackMime string, fallbackSize int64) *modality.ProviderFileHandle {
	purpose := modality.FilePurpose(strings.TrimSpace(r.Purpose))
	if purpose == "" {
		purpose = fallbackPurpose
	}
	size := r.Bytes
	if size == 0 {
		size = fallbackSize
	}
	createdAt := time.Now().UTC()
	if r.CreatedAt > 0 {
		createdAt = time.Unix(r.CreatedAt, 0).UTC()
	}
	var expiresAt *time.Time
	if r.ExpiresAt > 0 {
		value := time.Unix(r.ExpiresAt, 0).UTC()
		expiresAt = &value
	}
	return &modality.ProviderFileHandle{
		Provider:       provider,
		ProviderFileID: r.ID,
		Purpose:        purpose,
		SizeBytes:      size,
		MimeType:       fallbackMime,
		ExpiresAt:      expiresAt,
		CreatedAt:      createdAt,
	}
}

func openAIFilePurpose(purpose modality.FilePurpose) string {
	switch purpose {
	case modality.FilePurposeBatch:
		return "batch"
	case modality.FilePurposeAssistants:
		return "assistants"
	case modality.FilePurposeVision:
		return "vision"
	case modality.FilePurposeUserData:
		return "user_data"
	default:
		return "user_data"
	}
}

func (c *Client) filesMultipart(ctx context.Context, path string, payload []byte, contentType string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build openai files request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return translateTransportError(err, "OpenAI")
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= http.StatusBadRequest {
		return c.apiError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "OpenAI returned an invalid file response.")
	}
	return nil
}

func (c *Client) filesJSON(ctx context.Context, method string, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal openai files request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build openai files request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return translateTransportError(err, "OpenAI")
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= http.StatusBadRequest {
		return c.apiError(resp)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "OpenAI returned an invalid file response.")
		}
	}
	return nil
}
