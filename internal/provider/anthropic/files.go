package anthropic

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

	"github.com/JiaCheng2004/Polaris/internal/apierror"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

const anthropicFilesBeta = "files-api-2025-04-14"

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
		return nil, fmt.Errorf("read anthropic file body: %w", err)
	}
	filename := strings.TrimSpace(req.Filename)
	if filename == "" {
		filename = "file"
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeMultipartFilename(filename)))
	if strings.TrimSpace(req.MimeType) != "" {
		header.Set("Content-Type", strings.TrimSpace(req.MimeType))
	}
	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, fmt.Errorf("create anthropic file part: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return nil, fmt.Errorf("write anthropic file part: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close anthropic multipart writer: %w", err)
	}

	var response anthropicFileResponse
	if err := a.multipart(ctx, "/v1/files", body.Bytes(), writer.FormDataContentType(), &response); err != nil {
		return nil, err
	}
	return response.handle(a.provider, req.Purpose, req.MimeType, int64(len(data))), nil
}

func (a *FilesAdapter) Get(ctx context.Context, providerFileID string) (*modality.ProviderFileHandle, error) {
	providerFileID = strings.TrimSpace(providerFileID)
	if providerFileID == "" {
		return nil, apierror.NewError(http.StatusBadRequest, "invalid_request_error", "missing_file_id", "file_id", "Provider file id is required.")
	}
	var response anthropicFileResponse
	if err := a.jsonRequest(ctx, http.MethodGet, "/v1/files/"+providerFileID, nil, &response); err != nil {
		return nil, err
	}
	return response.handle(a.provider, modality.FilePurposeUserData, "", 0), nil
}

func (a *FilesAdapter) Delete(ctx context.Context, providerFileID string) error {
	providerFileID = strings.TrimSpace(providerFileID)
	if providerFileID == "" {
		return apierror.NewError(http.StatusBadRequest, "invalid_request_error", "missing_file_id", "file_id", "Provider file id is required.")
	}
	return a.jsonRequest(ctx, http.MethodDelete, "/v1/files/"+providerFileID, nil, nil)
}

func (a *FilesAdapter) Materialize(ctx context.Context, req *modality.FileMaterializeRequest) (*modality.ProviderFileHandle, error) {
	if req == nil || len(req.InlineSrc) == 0 {
		return nil, apierror.NewError(http.StatusBadRequest, "invalid_request_error", "missing_file_bytes", "file", "Anthropic file materialization requires file bytes.")
	}
	return a.Upload(ctx, &modality.FileUploadRequest{
		Body:     bytes.NewReader(req.InlineSrc),
		Size:     req.Size,
		Filename: req.Filename,
		MimeType: req.MimeType,
		Purpose:  req.Purpose,
	})
}

type anthropicFileResponse struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Filename  string `json:"filename"`
	SizeBytes int64  `json:"size_bytes"`
	MimeType  string `json:"mime_type"`
	CreatedAt string `json:"created_at"`
}

func (r anthropicFileResponse) handle(provider string, fallbackPurpose modality.FilePurpose, fallbackMime string, fallbackSize int64) *modality.ProviderFileHandle {
	createdAt := time.Now().UTC()
	if parsed, err := time.Parse(time.RFC3339, r.CreatedAt); err == nil {
		createdAt = parsed.UTC()
	}
	size := r.SizeBytes
	if size == 0 {
		size = fallbackSize
	}
	return &modality.ProviderFileHandle{
		Provider:       provider,
		ProviderFileID: r.ID,
		Purpose:        fallbackPurpose,
		SizeBytes:      size,
		MimeType:       firstNonEmpty(r.MimeType, fallbackMime),
		CreatedAt:      createdAt,
	}
}

func (a *FilesAdapter) multipart(ctx context.Context, path string, payload []byte, contentType string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.client.BaseURL()+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build anthropic files request: %w", err)
	}
	req.Header.Set("x-api-key", a.client.APIKey())
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("anthropic-beta", anthropicFilesBeta)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.HTTPClient().Do(req)
	if err != nil {
		return apierror.ProviderTransportError(err, "Anthropic")
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= http.StatusBadRequest {
		return a.client.APIError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return apierror.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Anthropic returned an invalid file response.")
	}
	return nil
}

func (a *FilesAdapter) jsonRequest(ctx context.Context, method string, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal anthropic files request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.client.BaseURL()+path, reader)
	if err != nil {
		return fmt.Errorf("build anthropic files request: %w", err)
	}
	req.Header.Set("x-api-key", a.client.APIKey())
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("anthropic-beta", anthropicFilesBeta)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := a.client.HTTPClient().Do(req)
	if err != nil {
		return apierror.ProviderTransportError(err, "Anthropic")
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= http.StatusBadRequest {
		return a.client.APIError(resp)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return apierror.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Anthropic returned an invalid file response.")
		}
	}
	return nil
}

func escapeMultipartFilename(filename string) string {
	filename = strings.ReplaceAll(filename, `\`, `\\`)
	filename = strings.ReplaceAll(filename, `"`, `\"`)
	return filename
}
