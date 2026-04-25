package bytedance

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

const defaultBaseURL = "https://ark.cn-beijing.volces.com/api/v3"
const defaultControlBaseURL = "https://open.volcengineapi.com"

type Client struct {
	baseURL         string
	controlBaseURL  string
	apiKey          string
	accessKeyID     string
	accessKeySecret string
	appID           string
	speechAPIKey    string
	speechToken     string
	projectName     string
	httpClient      *http.Client
	maxAttempts     int
	initialDelay    time.Duration
}

func NewClient(cfg config.ProviderConfig) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	controlBaseURL := strings.TrimRight(cfg.ControlBaseURL, "/")
	if controlBaseURL == "" {
		controlBaseURL = defaultControlBaseURL
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

	return &Client{
		baseURL:         baseURL,
		controlBaseURL:  controlBaseURL,
		apiKey:          cfg.APIKey,
		accessKeyID:     cfg.AccessKeyID,
		accessKeySecret: cfg.AccessKeySecret,
		appID:           cfg.AppID,
		speechAPIKey:    cfg.SpeechAPIKey,
		speechToken:     cfg.SpeechAccessToken,
		projectName:     firstNonEmpty(strings.TrimSpace(cfg.ProjectName), "default"),
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: telemetry.NewProviderTransport("bytedance", nil),
		},
		maxAttempts:  maxAttempts,
		initialDelay: initialDelay,
	}
}

func (c *Client) JSON(ctx context.Context, endpoint string, path string, body any, out any) error {
	return c.JSONRequest(ctx, http.MethodPost, endpoint, path, body, out)
}

func (c *Client) JSONRequest(ctx context.Context, method string, endpoint string, path string, body any, out any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal bytedance request: %w", err)
		}
	}

	resp, err := c.doRequest(ctx, method, endpoint, path, payload)
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
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid JSON response.")
	}
	return nil
}

func (c *Client) RawRequest(ctx context.Context, method string, endpoint string, path string, body any) (*http.Response, error) {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal bytedance request: %w", err)
		}
	}

	resp, err := c.doRequest(ctx, method, endpoint, path, payload)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) Stream(ctx context.Context, endpoint string, path string, body any) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal bytedance stream request: %w", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, endpoint, path, payload)
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

func (c *Client) doRequest(ctx context.Context, method string, endpoint string, path string, payload []byte) (*http.Response, error) {
	attempts := c.maxAttempts
	if attempts <= 0 {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		var body io.Reader
		if payload != nil {
			body = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, joinURL(c.resolveBaseURL(endpoint), path), body)
		if err != nil {
			return nil, fmt.Errorf("build bytedance request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < attempts && retrypkg.RetryableTransportError(err) {
				if sleepErr := retrypkg.SleepWithContext(ctx, retrypkg.BackoffDelay(c.initialDelay, attempt)); sleepErr == nil {
					continue
				}
			}
			return nil, retrypkg.TranslateTransportError(err, "ByteDance")
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

	return nil, retrypkg.TranslateTransportError(lastErr, "ByteDance")
}

func (c *Client) resolveBaseURL(endpoint string) string {
	if trimmed := strings.TrimSpace(endpoint); trimmed != "" {
		return strings.TrimRight(trimmed, "/")
	}
	return c.baseURL
}

func joinURL(baseURL string, path string) string {
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
}

func (c *Client) apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	type bytedanceErrorEnvelope struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
			Param   string `json:"param"`
		} `json:"error"`
	}

	var parsed bytedanceErrorEnvelope
	_ = json.Unmarshal(body, &parsed)

	return httputil.ProviderAPIError("ByteDance", resp.StatusCode, httputil.ProviderErrorDetails{
		Message: parsed.Error.Message,
		Body:    string(body),
		Code:    parsed.Error.Code,
		Param:   parsed.Error.Param,
		Type:    parsed.Error.Type,
	})
}

func translateTransportError(err error, providerName string) error {
	return retrypkg.TranslateTransportError(err, providerName)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
