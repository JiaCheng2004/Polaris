package bedrock

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
	awsauth "github.com/JiaCheng2004/Polaris/internal/provider/common/auth"
	"github.com/JiaCheng2004/Polaris/internal/transport"
)

const bedrockService = "bedrock"

type Client struct {
	core         *transport.Client
	baseURL      string
	region       string
	httpClient   *http.Client
	maxAttempts  int
	initialDelay time.Duration
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

	accessKeyID := strings.TrimSpace(cfg.AccessKeyID)
	accessKeySecret := strings.TrimSpace(cfg.AccessKeySecret)
	sessionToken := strings.TrimSpace(cfg.SessionToken)

	core := transport.New(transport.Options{
		BaseURL:      baseURL,
		ProviderName: "Amazon Bedrock",
		ProviderSlug: "bedrock",
		// SigV4 is re-signed per attempt with a fresh timestamp; the AuthFunc
		// receives the exact request body to hash. It runs after Content-Type
		// and Accept are set so those signed headers are covered.
		Auth: func(req *http.Request, body []byte) error {
			if err := awsauth.SignAWSRequest(req, body, bedrockService, region, accessKeyID, accessKeySecret, sessionToken, time.Now()); err != nil {
				return fmt.Errorf("sign amazon bedrock request: %w", err)
			}
			return nil
		},
		Timeout: timeout,
		Retry: transport.RetryPolicy{
			MaxAttempts:       maxAttempts,
			InitialDelay:      initialDelay,
			RespectRetryAfter: true,
		},
	})

	return &Client{
		core:         core,
		baseURL:      baseURL,
		region:       region,
		httpClient:   core.HTTPClient(),
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
		return apierror.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Amazon Bedrock returned an invalid JSON response.")
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
	return c.core.Do(ctx, transport.Request{
		Method:      http.MethodPost,
		Path:        path,
		Body:        payload,
		ContentType: "application/json",
		Accept:      accept,
	})
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

	return apierror.ProviderAPIError("Amazon Bedrock", resp.StatusCode, apierror.ProviderErrorDetails{
		Message: message,
		Body:    string(body),
		Code:    code,
		Type:    parsed.Type,
	})
}
