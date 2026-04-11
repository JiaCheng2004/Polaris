package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
)

type Client struct {
	baseURL      string
	apiKey       string
	httpClient   *http.Client
	maxAttempts  int
	initialDelay time.Duration
}

func NewClient(cfg config.ProviderConfig) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
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

	return &Client{
		baseURL: baseURL,
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		maxAttempts:  maxAttempts,
		initialDelay: initialDelay,
	}
}

func (c *Client) JSON(ctx context.Context, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal openai request: %w", err)
	}

	resp, err := c.do(ctx, path, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return c.apiError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "OpenAI returned an invalid JSON response.")
	}
	return nil
}

func (c *Client) Stream(ctx context.Context, path string, body any) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal openai stream request: %w", err)
	}

	resp, err := c.do(ctx, path, payload)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer resp.Body.Close()
		return nil, c.apiError(resp)
	}
	return resp, nil
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
			return nil, fmt.Errorf("build openai request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < attempts && retryableTransportError(err) {
				if sleepErr := sleepWithContext(ctx, backoffDelay(c.initialDelay, attempt)); sleepErr == nil {
					continue
				}
			}
			return nil, translateTransportError(err, "OpenAI")
		}

		if retryableStatus(resp.StatusCode) && attempt < attempts {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if sleepErr := sleepWithContext(ctx, backoffDelay(c.initialDelay, attempt)); sleepErr == nil {
				continue
			}
		}

		return resp, nil
	}

	return nil, translateTransportError(lastErr, "OpenAI")
}

func (c *Client) apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	type openAIErrorEnvelope struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
			Param   string `json:"param"`
		} `json:"error"`
	}

	var parsed openAIErrorEnvelope
	_ = json.Unmarshal(body, &parsed)

	message := strings.TrimSpace(parsed.Error.Message)
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	if message == "" {
		message = "OpenAI returned an error."
	}

	switch resp.StatusCode {
	case http.StatusBadRequest:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", firstNonEmpty(parsed.Error.Code, "provider_bad_request"), parsed.Error.Param, message)
	case http.StatusTooManyRequests:
		return httputil.NewError(http.StatusTooManyRequests, "rate_limit_error", firstNonEmpty(parsed.Error.Code, "provider_rate_limit"), "", message)
	case http.StatusUnauthorized, http.StatusForbidden:
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_auth_failed", "", message)
	default:
		if resp.StatusCode >= http.StatusInternalServerError {
			return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_server_error", "", message)
		}
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_error", "", message)
	}
}

func retryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func retryableTransportError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func translateTransportError(err error, providerName string) error {
	if err == nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_transport_error", "", providerName+" request failed.")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return httputil.NewError(http.StatusGatewayTimeout, "timeout_error", "provider_timeout", "", providerName+" timed out.")
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return httputil.NewError(http.StatusGatewayTimeout, "timeout_error", "provider_timeout", "", providerName+" timed out.")
	}
	return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_transport_error", "", providerName+" request failed.")
}

func backoffDelay(initial time.Duration, attempt int) time.Duration {
	if attempt <= 1 {
		return initial
	}
	delay := initial
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay > 5*time.Second {
			return 5 * time.Second
		}
	}
	return delay
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
