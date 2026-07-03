package anthropiccompat

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

// Client is the shared Anthropic-compatible transport (Anthropic Messages wire
// format), used by the native Anthropic adapter and the token-plan providers
// (zaitoken, minimaxtoken). It wraps the unified transport.Client and adds a
// per-request anthropic-beta header mechanism.
type Client struct {
	core         *transport.Client
	baseURL      string
	apiKey       string
	httpClient   *http.Client
	maxAttempts  int
	initialDelay time.Duration
}

// NewClient builds an Anthropic-compatible client. Authentication flows through
// staticHeaders (x-api-key + anthropic-version, or Authorization: Bearer for
// token-plan providers), matching each caller's scheme.
func NewClient(providerSlug, providerName string, cfg config.ProviderConfig, defaultBaseURL string, staticHeaders map[string]string) *Client {
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
		timeout = 2 * time.Minute
	}

	core := transport.New(transport.Options{
		BaseURL:       baseURL,
		ProviderName:  providerName,
		ProviderSlug:  providerSlug,
		StaticHeaders: staticHeaders,
		Timeout:       timeout,
		Retry: transport.RetryPolicy{
			MaxAttempts:       maxAttempts,
			InitialDelay:      initialDelay,
			RespectRetryAfter: true,
		},
		ErrorTranslator: translateAnthropicError,
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

// ProviderName returns the display name used in error messages.
func (c *Client) ProviderName() string { return c.core.ProviderName() }

// Core exposes the underlying transport.Client.
func (c *Client) Core() *transport.Client { return c.core }

// JSON POSTs body and decodes a success response into out. The response return
// is retained for signature compatibility and is always nil.
func (c *Client) JSON(ctx context.Context, path string, body any, out any) (*http.Response, error) {
	return c.JSONWithBetas(ctx, path, body, out, nil)
}

// JSONWithBetas is JSON with an anthropic-beta header carrying the given beta
// feature flags.
func (c *Client) JSONWithBetas(ctx context.Context, path string, body any, out any, betas []string) (*http.Response, error) {
	if err := c.core.JSON(ctx, http.MethodPost, path, body, out, betaOption(betas)); err != nil {
		return nil, err
	}
	return nil, nil
}

// Stream POSTs body and returns the raw response for SSE reading.
func (c *Client) Stream(ctx context.Context, path string, body any) (*http.Response, error) {
	return c.StreamWithBetas(ctx, path, body, nil)
}

// StreamWithBetas is Stream with an anthropic-beta header.
func (c *Client) StreamWithBetas(ctx context.Context, path string, body any, betas []string) (*http.Response, error) {
	return c.core.Stream(ctx, http.MethodPost, path, body, transport.WithAccept("application/json"), betaOption(betas))
}

func betaOption(betas []string) transport.ReqOption {
	if len(betas) == 0 {
		return func(*transport.Request) {}
	}
	return transport.WithHeader("anthropic-beta", strings.Join(betas, ","))
}

func translateAnthropicError(providerName string, status int, body []byte) *apierror.APIError {
	type anthropicErrorEnvelope struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
		Type    string `json:"type"`
	}

	var parsed anthropicErrorEnvelope
	_ = json.Unmarshal(body, &parsed)

	message := strings.TrimSpace(parsed.Error.Message)
	if message == "" {
		message = strings.TrimSpace(parsed.Message)
	}
	errorType := strings.TrimSpace(parsed.Error.Type)
	if errorType == "" {
		errorType = strings.TrimSpace(parsed.Type)
	}

	return apierror.ProviderAPIError(providerName, status, apierror.ProviderErrorDetails{
		Message: message,
		Body:    string(body),
		Type:    errorType,
	})
}

// APIError parses an Anthropic-compatible error body into a canonical APIError,
// for adapters that issue their own requests.
func (c *Client) APIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	return translateAnthropicError(c.core.ProviderName(), resp.StatusCode, body)
}
