package ollama

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
	httpClient   *http.Client
	maxAttempts  int
	initialDelay time.Duration
}

func NewClient(cfg config.ProviderConfig) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	maxAttempts := cfg.Retry.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	initialDelay := cfg.Retry.InitialDelay
	if initialDelay <= 0 {
		initialDelay = 500 * time.Millisecond
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	core := transport.New(transport.Options{
		BaseURL:      baseURL,
		ProviderName: "Ollama",
		ProviderSlug: "ollama",
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
		httpClient:   core.HTTPClient(),
		maxAttempts:  maxAttempts,
		initialDelay: initialDelay,
	}
}

func (c *Client) JSON(ctx context.Context, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal ollama request: %w", err)
	}

	resp, err := c.do(ctx, path, payload)
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
		return apierror.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Ollama returned an invalid JSON response.")
	}
	return nil
}

func (c *Client) Stream(ctx context.Context, path string, body any) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal ollama stream request: %w", err)
	}

	resp, err := c.do(ctx, path, payload)
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

// do issues the request through the shared transport core. The endpoint is
// resolved to an absolute URL (with the /api prefix) and passed verbatim.
func (c *Client) do(ctx context.Context, path string, payload []byte) (*http.Response, error) {
	return c.core.Do(ctx, transport.Request{
		Method:      http.MethodPost,
		Path:        c.endpoint(path),
		Body:        payload,
		ContentType: "application/json",
		Accept:      "application/json",
	})
}

func (c *Client) endpoint(path string) string {
	base := strings.TrimRight(c.baseURL, "/")
	if strings.HasSuffix(base, "/api") {
		return base + path
	}
	return base + "/api" + path
}

func (c *Client) apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	type ollamaErrorEnvelope struct {
		Error string `json:"error"`
	}

	var parsed ollamaErrorEnvelope
	_ = json.Unmarshal(body, &parsed)

	switch resp.StatusCode {
	case http.StatusNotFound:
		return apierror.NewError(http.StatusBadGateway, "provider_error", "provider_model_unavailable", "", strings.TrimSpace(parsed.Error))
	}

	return apierror.ProviderAPIError("Ollama", resp.StatusCode, apierror.ProviderErrorDetails{
		Message: parsed.Error,
		Body:    string(body),
	})
}
