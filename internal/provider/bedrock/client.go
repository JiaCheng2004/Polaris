package bedrock

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
	awsauth "github.com/JiaCheng2004/Polaris/internal/provider/common/auth"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/openaicompat"
)

const bedrockService = "bedrock"

type Client struct {
	baseURL         string
	region          string
	accessKeyID     string
	accessKeySecret string
	sessionToken    string
	httpClient      *http.Client
	maxAttempts     int
	initialDelay    time.Duration
}

func NewClient(cfg config.ProviderConfig) *Client {
	region := strings.TrimSpace(cfg.Location)
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com", region)
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = time.Minute
	}
	maxAttempts := cfg.Retry.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	initialDelay := cfg.Retry.InitialDelay
	if initialDelay <= 0 {
		initialDelay = 200 * time.Millisecond
	}

	return &Client{
		baseURL:         baseURL,
		region:          region,
		accessKeyID:     strings.TrimSpace(cfg.AccessKeyID),
		accessKeySecret: strings.TrimSpace(cfg.AccessKeySecret),
		sessionToken:    strings.TrimSpace(cfg.SessionToken),
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: telemetry.NewProviderTransport("bedrock", nil),
		},
		maxAttempts:  maxAttempts,
		initialDelay: initialDelay,
	}
}

func (c *Client) JSON(ctx context.Context, path string, body any, out any) error {
	resp, err := c.do(ctx, path, body, "application/json")
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
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Amazon Bedrock returned an invalid JSON response.")
	}
	return nil
}

func (c *Client) Stream(ctx context.Context, path string, body any) (*http.Response, error) {
	resp, err := c.do(ctx, path, body, "application/vnd.amazon.eventstream")
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

func (c *Client) do(ctx context.Context, path string, body any, accept string) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal amazon bedrock request: %w", err)
	}

	attempts := c.maxAttempts
	if attempts <= 0 {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("build amazon bedrock request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		if err := awsauth.SignAWSRequest(req, payload, bedrockService, c.region, c.accessKeyID, c.accessKeySecret, c.sessionToken, time.Now()); err != nil {
			return nil, fmt.Errorf("sign amazon bedrock request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < attempts && openaicompat.RetryableTransportError(err) {
				if sleepErr := openaicompat.SleepWithContext(ctx, openaicompat.BackoffDelay(c.initialDelay, attempt)); sleepErr == nil {
					continue
				}
			}
			return nil, httputil.ProviderTransportError(err, "Amazon Bedrock")
		}

		if openaicompat.RetryableStatus(resp.StatusCode) && attempt < attempts {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if sleepErr := openaicompat.SleepWithContext(ctx, openaicompat.BackoffDelay(c.initialDelay, attempt)); sleepErr == nil {
				continue
			}
		}

		return resp, nil
	}

	return nil, httputil.ProviderTransportError(lastErr, "Amazon Bedrock")
}

func (c *Client) apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 128*1024))

	type bedrockErrorEnvelope struct {
		Message string `json:"message"`
		Type    string `json:"__type"`
		Code    string `json:"code"`
	}

	var parsed bedrockErrorEnvelope
	_ = json.Unmarshal(body, &parsed)

	message := strings.TrimSpace(parsed.Message)
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	if message == "" {
		message = "Amazon Bedrock returned an error."
	}

	code := strings.TrimSpace(parsed.Code)
	if code == "" {
		code = strings.TrimSpace(parsed.Type)
	}

	return httputil.ProviderAPIError("Amazon Bedrock", resp.StatusCode, httputil.ProviderErrorDetails{
		Message: message,
		Body:    string(body),
		Code:    code,
		Type:    parsed.Type,
	})
}
