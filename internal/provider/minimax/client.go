package minimax

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

const defaultBaseURL = "https://api.minimax.io"

type Client struct {
	core       *transport.Client
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
	core := transport.New(transport.Options{
		BaseURL:      baseURL,
		ProviderName: "MiniMax",
		ProviderSlug: "minimax",
		Auth:         transport.BearerAuth(cfg.APIKey),
		Timeout:      timeout,
		// MiniMax generation submits are not idempotent: keep single-attempt.
		Retry: transport.RetryPolicy{MaxAttempts: 1},
	})
	return &Client{
		core:       core,
		baseURL:    baseURL,
		apiKey:     cfg.APIKey,
		httpClient: core.HTTPClient(),
	}
}

func (c *Client) JSON(ctx context.Context, method string, path string, body any, out any) (*http.Response, error) {
	resp, err := c.do(ctx, method, path, body, "application/json", "application/json")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer func() {
			_ = resp.Body.Close()
		}()
		return nil, c.apiError(resp)
	}
	if out == nil {
		return resp, nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		_ = resp.Body.Close()
		return nil, apierror.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "MiniMax returned an invalid JSON response.")
	}
	_ = resp.Body.Close()
	return resp, nil
}

func (c *Client) Raw(ctx context.Context, method string, path string, body any, accept string) (*http.Response, error) {
	resp, err := c.do(ctx, method, path, body, "application/json", accept)
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

func (c *Client) do(ctx context.Context, method string, path string, body any, contentType string, accept string) (*http.Response, error) {
	var payload []byte
	if body != nil {
		marshaled, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal minimax request: %w", err)
		}
		payload = marshaled
	}
	ct := ""
	if contentType != "" && body != nil {
		ct = contentType
	}
	return c.core.Do(ctx, transport.Request{
		Method:      method,
		Path:        path,
		Body:        payload,
		ContentType: ct,
		Accept:      accept,
	})
}

func (c *Client) apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	type minimaxErrorEnvelope struct {
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
	}

	var parsed minimaxErrorEnvelope
	_ = json.Unmarshal(body, &parsed)

	return apierror.ProviderAPIError("MiniMax", resp.StatusCode, apierror.ProviderErrorDetails{
		Message: parsed.BaseResp.StatusMsg,
		Body:    string(body),
		Code:    fmt.Sprintf("%d", parsed.BaseResp.StatusCode),
	})
}
