package anthropiccompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/telemetry"
	retrypkg "github.com/JiaCheng2004/Polaris/internal/provider/common/retry"
)

type Client struct {
	providerSlug  string
	providerName  string
	baseURL       string
	apiKey        string
	httpClient    *http.Client
	maxAttempts   int
	initialDelay  time.Duration
	staticHeaders map[string]string
}

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

	headersCopy := make(map[string]string, len(staticHeaders))
	for key, value := range staticHeaders {
		headersCopy[key] = value
	}

	return &Client{
		providerSlug: providerSlug,
		providerName: providerName,
		baseURL:      baseURL,
		apiKey:       cfg.APIKey,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: telemetry.NewProviderTransport(providerSlug, nil),
		},
		maxAttempts:   maxAttempts,
		initialDelay:  initialDelay,
		staticHeaders: headersCopy,
	}
}

func (c *Client) BaseURL() string {
	return c.baseURL
}

func (c *Client) APIKey() string {
	return c.apiKey
}

func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}

func (c *Client) MaxAttempts() int {
	return c.maxAttempts
}

func (c *Client) InitialDelay() time.Duration {
	return c.initialDelay
}

func (c *Client) JSON(ctx context.Context, path string, body any, out any) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal %s request: %w", strings.ToLower(c.providerName), err)
	}

	resp, err := c.do(ctx, path, payload)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		return resp, c.APIError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return resp, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", fmt.Sprintf("%s returned an invalid JSON response.", c.providerName))
	}
	return resp, nil
}

func (c *Client) Stream(ctx context.Context, path string, body any) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal %s stream request: %w", strings.ToLower(c.providerName), err)
	}

	resp, err := c.do(ctx, path, payload)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer func() {
			_ = resp.Body.Close()
		}()
		return nil, c.APIError(resp)
	}
	return resp, nil
}

func (c *Client) APIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

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

	return httputil.ProviderAPIError(c.providerName, resp.StatusCode, httputil.ProviderErrorDetails{
		Message: message,
		Body:    string(body),
		Type:    errorType,
	})
}

func (c *Client) do(ctx context.Context, path string, payload []byte) (*http.Response, error) {
	attempts := c.maxAttempts
	if attempts <= 0 {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("build %s request: %w", strings.ToLower(c.providerName), err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		for key, value := range c.staticHeaders {
			req.Header.Set(key, value)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < attempts && retrypkg.RetryableTransportError(err) {
				if sleepErr := retrypkg.SleepWithContext(ctx, retrypkg.BackoffDelay(c.initialDelay, attempt)); sleepErr == nil {
					continue
				}
			}
			return nil, retrypkg.TranslateTransportError(err, c.providerName)
		}

		if retrypkg.RetryableStatus(resp.StatusCode) && attempt < attempts {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if sleepErr := retrypkg.SleepWithContext(ctx, retrypkg.BackoffDelay(c.initialDelay, attempt)); sleepErr == nil {
				continue
			}
		}

		return resp, nil
	}

	return nil, retrypkg.TranslateTransportError(lastErr, c.providerName)
}
