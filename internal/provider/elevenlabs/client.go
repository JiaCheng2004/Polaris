package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/telemetry"
)

const defaultBaseURL = "https://api.elevenlabs.io"

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(cfg config.ProviderConfig) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: telemetry.NewProviderTransport("elevenlabs", nil),
		},
	}
}

func (c *Client) JSON(ctx context.Context, method string, path string, query url.Values, body any, out any) (*http.Response, error) {
	resp, err := c.doJSON(ctx, method, path, query, body)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return resp, nil
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ElevenLabs returned an invalid JSON response.")
	}
	return resp, nil
}

func (c *Client) Raw(ctx context.Context, method string, path string, query url.Values, body any, accept string) (*http.Response, error) {
	return c.do(ctx, method, path, query, bodyReader(body), "application/json", accept)
}

func (c *Client) Multipart(ctx context.Context, method string, path string, query url.Values, payload []byte, contentType string, accept string) (*http.Response, error) {
	return c.do(ctx, method, path, query, bytes.NewReader(payload), contentType, accept)
}

func (c *Client) UploadFile(ctx context.Context, path string, fieldName string, filename string, contentType string, data []byte, fields map[string]string, out any) (*http.Response, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if err := writer.WriteField(key, value); err != nil {
			return nil, fmt.Errorf("write multipart field %s: %w", key, err)
		}
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, filename))
	if strings.TrimSpace(contentType) != "" {
		header.Set("Content-Type", contentType)
	}
	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, fmt.Errorf("create multipart part: %w", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("write multipart part: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}
	resp, err := c.Multipart(ctx, http.MethodPost, path, nil, body.Bytes(), writer.FormDataContentType(), "application/json")
	if err != nil {
		return nil, err
	}
	if out == nil {
		return resp, nil
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ElevenLabs returned an invalid JSON response.")
	}
	return resp, nil
}

func (c *Client) doJSON(ctx context.Context, method string, path string, query url.Values, body any) (*http.Response, error) {
	return c.do(ctx, method, path, query, bodyReader(body), "application/json", "application/json")
}

func (c *Client) do(ctx context.Context, method string, path string, query url.Values, body io.Reader, contentType string, accept string) (*http.Response, error) {
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, fmt.Errorf("build elevenlabs request: %w", err)
	}
	req.Header.Set("xi-api-key", c.apiKey)
	if contentType != "" && body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, httputil.ProviderTransportError(err, "ElevenLabs")
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer func() {
			_ = resp.Body.Close()
		}()
		return nil, c.apiError(resp)
	}
	return resp, nil
}

func (c *Client) apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	type elevenErrorEnvelope struct {
		Detail struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"detail"`
	}

	var parsed elevenErrorEnvelope
	_ = json.Unmarshal(body, &parsed)

	return httputil.ProviderAPIError("ElevenLabs", resp.StatusCode, httputil.ProviderErrorDetails{
		Message: parsed.Detail.Message,
		Body:    string(body),
		Status:  parsed.Detail.Status,
	})
}

func bodyReader(body any) io.Reader {
	if body == nil {
		return nil
	}
	if reader, ok := body.(io.Reader); ok {
		return reader
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return bytes.NewReader(nil)
	}
	return bytes.NewReader(payload)
}
