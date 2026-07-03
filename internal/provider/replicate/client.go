package replicate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/apierror"
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/transport"
)

type Client struct {
	core         *transport.Client
	baseURL      string
	apiKey       string
	httpClient   *http.Client
	maxAttempts  int
	initialDelay time.Duration
}

func NewClient(cfg config.ProviderConfig) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.replicate.com/v1"
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	maxAttempts := cfg.Retry.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	initialDelay := cfg.Retry.InitialDelay
	if initialDelay <= 0 {
		initialDelay = 200 * time.Millisecond
	}

	apiKey := strings.TrimSpace(cfg.APIKey)
	core := transport.New(transport.Options{
		BaseURL:      baseURL,
		ProviderName: "Replicate",
		ProviderSlug: "replicate",
		Auth:         transport.BearerAuth(apiKey),
		Timeout:      timeout,
		Retry: transport.RetryPolicy{
			MaxAttempts:       maxAttempts,
			InitialDelay:      initialDelay,
			RespectRetryAfter: true,
		},
	})

	return &Client{
		core:         core,
		baseURL:      baseURL,
		apiKey:       apiKey,
		httpClient:   core.HTTPClient(),
		maxAttempts:  maxAttempts,
		initialDelay: initialDelay,
	}
}

func (c *Client) JSON(ctx context.Context, method string, path string, body any, out any) error {
	resp, err := c.do(ctx, method, path, body, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		return c.apiError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return apierror.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Replicate returned an invalid JSON response.")
	}
	return nil
}

// Download fetches a generated asset URL. It is a single-attempt GET (no retry)
// on the shared transport client, preserving the previous behavior.
func (c *Client) Download(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build replicate download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, apierror.ProviderTransportError(err, "Replicate")
	}
	return resp, nil
}

func (c *Client) do(ctx context.Context, method string, path string, body any, extraHeaders map[string]string) (*http.Response, error) {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal replicate request: %w", err)
		}
	}

	headers := make(map[string]string, len(extraHeaders))
	for key, value := range extraHeaders {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		headers[key] = value
	}

	contentType := ""
	if payload != nil {
		contentType = "application/json"
	}
	return c.core.Do(ctx, transport.Request{
		Method:      method,
		Path:        path,
		Body:        payload,
		ContentType: contentType,
		Accept:      "application/json",
		Headers:     headers,
	})
}

func (c *Client) apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 128*1024))

	type replicateErrorEnvelope struct {
		Detail string `json:"detail"`
		Title  string `json:"title"`
		Error  string `json:"error"`
		Status int    `json:"status"`
		Type   string `json:"type"`
	}

	var parsed replicateErrorEnvelope
	_ = json.Unmarshal(body, &parsed)

	message := strings.TrimSpace(parsed.Detail)
	if message == "" {
		message = strings.TrimSpace(parsed.Error)
	}
	if message == "" {
		message = strings.TrimSpace(parsed.Title)
	}
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	if message == "" {
		message = "Replicate returned an error."
	}

	return apierror.ProviderAPIError("Replicate", resp.StatusCode, apierror.ProviderErrorDetails{
		Message: message,
		Body:    string(body),
		Type:    parsed.Type,
	})
}
