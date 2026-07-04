package openaicompat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/apierror"
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/transport"
)

// Client is the shared OpenAI-compatible transport used by every provider whose
// wire format matches OpenAI's /chat/completions and /embeddings surfaces. It
// wraps the unified transport.Client, preserving the accessor and JSON/Stream
// API that provider adapters depend on.
type Client struct {
	core         *transport.Client
	baseURL      string
	apiKey       string
	httpClient   *http.Client
	maxAttempts  int
	initialDelay time.Duration
}

// NewClient builds an OpenAI-compatible client. providerSlug is the low-cardinality
// span/metric label; providerName is the display name in error messages.
func NewClient(providerSlug string, providerName string, cfg config.ProviderConfig, defaultBaseURL string, staticHeaders map[string]string) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(defaultBaseURL, "/")
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
		BaseURL:       baseURL,
		ProviderName:  providerName,
		ProviderSlug:  providerSlug,
		Auth:          transport.BearerAuth(cfg.APIKey),
		StaticHeaders: staticHeaders,
		Timeout:       timeout,
		Retry: transport.RetryPolicy{
			MaxAttempts:       maxAttempts,
			InitialDelay:      initialDelay,
			RespectRetryAfter: true,
		},
		ErrorTranslator: translateOpenAIError,
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

func (c *Client) BaseURL() string { return c.baseURL }

func (c *Client) APIKey() string { return c.apiKey }

func (c *Client) HTTPClient() *http.Client { return c.httpClient }

func (c *Client) MaxAttempts() int { return c.maxAttempts }

func (c *Client) InitialDelay() time.Duration { return c.initialDelay }

// ProviderName returns the display name used in error and log messages.
func (c *Client) ProviderName() string { return c.core.ProviderName() }

// Core exposes the underlying transport.Client for adapters that need bespoke
// request shapes (query params, alternate paths).
func (c *Client) Core() *transport.Client { return c.core }

// JSON POSTs body as JSON and decodes a success response into out.
func (c *Client) JSON(ctx context.Context, path string, body any, out any) error {
	return c.core.JSON(ctx, http.MethodPost, path, body, out)
}

// Stream POSTs body and returns the raw response for SSE reading. The Accept
// header is application/json, matching the historical behavior of this client.
func (c *Client) Stream(ctx context.Context, path string, body any) (*http.Response, error) {
	return c.core.Stream(ctx, http.MethodPost, path, body, transport.WithAccept("application/json"))
}

// APIError parses an OpenAI-compatible error response body into a canonical
// APIError. Retained for adapters (e.g. multipart image edits) that issue their
// own requests and handle the error status directly.
func (c *Client) APIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	return translateOpenAIError(c.core.ProviderName(), resp.StatusCode, body)
}

func translateOpenAIError(providerName string, status int, body []byte) *apierror.APIError {
	type errorEnvelope struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
			Param   string `json:"param"`
		} `json:"error"`
		Message string `json:"message"`
		Code    string `json:"code"`
	}

	var parsed errorEnvelope
	_ = json.Unmarshal(body, &parsed)

	message := strings.TrimSpace(parsed.Error.Message)
	if message == "" {
		message = strings.TrimSpace(parsed.Message)
	}
	code := strings.TrimSpace(parsed.Error.Code)
	if code == "" {
		code = strings.TrimSpace(parsed.Code)
	}

	return apierror.ProviderAPIError(providerName, status, apierror.ProviderErrorDetails{
		Message: message,
		Body:    string(body),
		Code:    code,
		Param:   parsed.Error.Param,
		Type:    parsed.Error.Type,
	})
}
