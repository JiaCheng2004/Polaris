package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// UploadFile uploads a local file to the gateway's file store.
func (c *Client) UploadFile(ctx context.Context, req *FileUploadRequest) (*FileObject, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	if len(req.File) == 0 {
		return nil, fmt.Errorf("file is required")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writeMultipartFile(writer, "file", defaultFilename(req.Filename, "file"), defaultContentType(firstNonEmpty(req.ContentType, req.MimeType), req.File), req.File); err != nil {
		return nil, err
	}
	if err := writeOptionalMultipartField(writer, "filename", req.Filename); err != nil {
		return nil, err
	}
	if err := writeOptionalMultipartField(writer, "mime_type", req.MimeType); err != nil {
		return nil, err
	}
	if err := writeOptionalMultipartField(writer, "purpose", req.Purpose); err != nil {
		return nil, err
	}
	if err := writeOptionalMultipartField(writer, "expires_at", req.ExpiresAt); err != nil {
		return nil, err
	}
	if len(req.Metadata) > 0 {
		payload, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("marshal metadata: %w", err)
		}
		if err := writer.WriteField("metadata", string(payload)); err != nil {
			return nil, fmt.Errorf("write metadata field: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart body: %w", err)
	}

	resp, err := c.do(ctx, http.MethodPost, "/v1/files", nil, bytes.NewReader(body.Bytes()), writer.FormDataContentType(), "application/json")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, decodeAPIErrorResponse(resp)
	}
	var out FileObject
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode file upload response: %w", err)
	}
	return &out, nil
}

// UploadFileURL registers a file with the gateway by its source URL.
func (c *Client) UploadFileURL(ctx context.Context, req *FileURLUploadRequest) (*FileObject, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	var out FileObject
	if err := c.doJSON(ctx, http.MethodPost, "/v1/files", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListFiles lists files.
func (c *Client) ListFiles(ctx context.Context, params *ListFilesParams) (*FileList, error) {
	query := url.Values{}
	if params != nil {
		if params.Limit > 0 {
			query.Set("limit", strconv.Itoa(params.Limit))
		}
		if strings.TrimSpace(params.After) != "" {
			query.Set("after", params.After)
		}
		if strings.TrimSpace(params.Purpose) != "" {
			query.Set("purpose", params.Purpose)
		}
	}
	var out FileList
	if err := c.doJSON(ctx, http.MethodGet, "/v1/files", query, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetFile retrieves the file.
func (c *Client) GetFile(ctx context.Context, fileID string) (*FileObject, error) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return nil, fmt.Errorf("fileID is required")
	}
	var out FileObject
	if err := c.doJSON(ctx, http.MethodGet, "/v1/files/"+url.PathEscape(fileID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteFile deletes the file.
func (c *Client) DeleteFile(ctx context.Context, fileID string) error {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return fmt.Errorf("fileID is required")
	}
	return c.doJSON(ctx, http.MethodDelete, "/v1/files/"+url.PathEscape(fileID), nil, nil, nil)
}

// MaterializeFile materializes the file.
func (c *Client) MaterializeFile(ctx context.Context, fileID string, provider string) (*FileMaterialization, error) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return nil, fmt.Errorf("fileID is required")
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return nil, fmt.Errorf("provider is required")
	}
	query := url.Values{"provider": []string{provider}}
	var out FileMaterialization
	if err := c.doJSON(ctx, http.MethodPost, "/v1/files/"+url.PathEscape(fileID)+"/materialize", query, map[string]any{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DownloadFileContent downloads the file content.
func (c *Client) DownloadFileContent(ctx context.Context, contentURL string) ([]byte, string, error) {
	contentURL = strings.TrimSpace(contentURL)
	if contentURL == "" {
		return nil, "", fmt.Errorf("contentURL is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, contentURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build request: %w", err)
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("GET file content failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, "", decodeAPIErrorResponse(resp)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read file content: %w", err)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

func writeOptionalMultipartField(writer *multipart.Writer, name string, value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if err := writer.WriteField(name, value); err != nil {
		return fmt.Errorf("write %s field: %w", name, err)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
