package minimax

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
)

const defaultBaseURL = "https://api.minimax.io"

type Client struct {
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
	return &Client{
		baseURL: baseURL,
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: telemetry.NewProviderTransport("minimax", nil),
		},
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
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "MiniMax returned an invalid JSON response.")
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
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal minimax request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("build minimax request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if contentType != "" && body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, httputil.ProviderTransportError(err, "MiniMax")
	}
	return resp, nil
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

	return httputil.ProviderAPIError("MiniMax", resp.StatusCode, httputil.ProviderErrorDetails{
		Message: parsed.BaseResp.StatusMsg,
		Body:    string(body),
		Code:    fmt.Sprintf("%d", parsed.BaseResp.StatusCode),
	})
}
