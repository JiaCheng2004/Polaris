package bytedance

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

const defaultBaseURL = "https://ark.cn-beijing.volces.com/api/v3"
const defaultControlBaseURL = "https://open.volcengineapi.com"

type Client struct {
	core            *transport.Client
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

	core := transport.New(transport.Options{
		BaseURL:      baseURL,
		ProviderName: "ByteDance",
		ProviderSlug: "bytedance",
		Auth:         transport.BearerAuth(cfg.APIKey),
		Timeout:      timeout,
		Retry: transport.RetryPolicy{
			MaxAttempts:       maxAttempts,
			InitialDelay:      initialDelay,
			RespectRetryAfter: true,
		},
	})

	return &Client{
		core:            core,
		baseURL:         baseURL,
		controlBaseURL:  controlBaseURL,
		apiKey:          cfg.APIKey,
		accessKeyID:     cfg.AccessKeyID,
		accessKeySecret: cfg.AccessKeySecret,
		appID:           cfg.AppID,
		speechAPIKey:    cfg.SpeechAPIKey,
		speechToken:     cfg.SpeechAccessToken,
		projectName:     firstNonEmpty(strings.TrimSpace(cfg.ProjectName), "default"),
		httpClient:      core.HTTPClient(),
		maxAttempts:     maxAttempts,
		initialDelay:    initialDelay,
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
		return apierror.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid JSON response.")
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

// doRequest routes through the shared transport core (Bearer data-plane auth,
// jittered retry, B2 fix). The full URL is resolved per call because ByteDance
// data-plane and per-endpoint overrides use different base URLs; it is passed
// as an absolute path so the core issues it verbatim.
func (c *Client) doRequest(ctx context.Context, method string, endpoint string, path string, payload []byte) (*http.Response, error) {
	contentType := ""
	if payload != nil {
		contentType = "application/json"
	}
	return c.core.Do(ctx, transport.Request{
		Method:      method,
		Path:        joinURL(c.resolveBaseURL(endpoint), path),
		Body:        payload,
		ContentType: contentType,
		Accept:      "application/json",
	})
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

	return apierror.ProviderAPIError("ByteDance", resp.StatusCode, apierror.ProviderErrorDetails{
		Message: parsed.Error.Message,
		Body:    string(body),
		Code:    parsed.Error.Code,
		Param:   parsed.Error.Param,
		Type:    parsed.Error.Type,
	})
}

func translateTransportError(err error, providerName string) error {
	return apierror.ProviderTransportError(err, providerName)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
