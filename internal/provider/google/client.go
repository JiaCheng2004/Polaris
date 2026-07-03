package google

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
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}

	maxAttempts := cfg.Retry.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	initialDelay := cfg.Retry.InitialDelay
	if initialDelay <= 0 {
		initialDelay = 200 * time.Millisecond
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = time.Minute
	}

	core := transport.New(transport.Options{
		BaseURL:      baseURL,
		ProviderName: "Google",
		ProviderSlug: "google",
		Auth:         transport.APIKeyHeaderAuth("x-goog-api-key", cfg.APIKey),
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
		apiKey:       cfg.APIKey,
		httpClient:   core.HTTPClient(),
		maxAttempts:  maxAttempts,
		initialDelay: initialDelay,
	}
}

func (c *Client) JSON(ctx context.Context, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal google request: %w", err)
	}

	resp, err := c.do(ctx, path, payload, "application/json")
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		return c.apiError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return apierror.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Google returned an invalid JSON response.")
	}
	return nil
}

func (c *Client) Stream(ctx context.Context, path string, body any) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal google stream request: %w", err)
	}

	resp, err := c.do(ctx, path, payload, "text/event-stream")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer func() {
			_ = resp.Body.Close()
		}()
		return nil, c.apiError(resp)
	}
	return resp, nil
}

// do issues the request through the shared transport core, preserving Google's
// x-goog-api-key auth and per-request Accept (JSON vs SSE).
func (c *Client) do(ctx context.Context, path string, payload []byte, accept string) (*http.Response, error) {
	return c.core.Do(ctx, transport.Request{
		Method:      http.MethodPost,
		Path:        path,
		Body:        payload,
		ContentType: "application/json",
		Accept:      accept,
	})
}

func (c *Client) apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	type googleErrorEnvelope struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}

	var parsed googleErrorEnvelope
	_ = json.Unmarshal(body, &parsed)

	return apierror.ProviderAPIError("Google", resp.StatusCode, apierror.ProviderErrorDetails{
		Message: parsed.Error.Message,
		Body:    string(body),
		Status:  parsed.Error.Status,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
