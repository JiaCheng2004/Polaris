package googlevertex

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
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

type Client struct {
	baseURL        string
	projectID      string
	location       string
	httpClient     *http.Client
	tokenSource    oauth2.TokenSource
	tokenSourceErr error
}

func NewClient(cfg config.ProviderConfig) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = fmt.Sprintf("https://%s-aiplatform.googleapis.com", strings.TrimSpace(cfg.Location))
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = time.Minute
	}

	tokenSource, err := google.DefaultTokenSource(context.Background(), cloudPlatformScope)

	return &Client{
		baseURL:        baseURL,
		projectID:      strings.TrimSpace(cfg.ProjectID),
		location:       strings.TrimSpace(cfg.Location),
		httpClient:     &http.Client{Timeout: timeout, Transport: telemetry.NewProviderTransport("google-vertex", nil)},
		tokenSource:    tokenSource,
		tokenSourceErr: err,
	}
}

func (c *Client) endpoint(model string) string {
	return fmt.Sprintf("projects/%s/locations/%s/publishers/google/models/%s", c.projectID, c.location, model)
}

func (c *Client) JSON(ctx context.Context, method string, path string, body any, out any) error {
	resp, err := c.do(ctx, method, path, body, "application/json")
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
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Google Vertex returned an invalid JSON response.")
	}
	return nil
}

func (c *Client) do(ctx context.Context, method string, path string, body any, accept string) (*http.Response, error) {
	if c.tokenSourceErr != nil {
		return nil, httputil.ProviderAuthError("Google Vertex", "Google Vertex ADC credentials are not available.")
	}
	token, err := c.tokenSource.Token()
	if err != nil {
		return nil, httputil.ProviderAuthError("Google Vertex", "Google Vertex access token request failed.")
	}

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal google vertex request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("build google vertex request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, httputil.ProviderTransportError(err, "Google Vertex")
	}
	return resp, nil
}

func (c *Client) apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	type googleErrorEnvelope struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
			Code    int    `json:"code"`
		} `json:"error"`
	}

	var parsed googleErrorEnvelope
	_ = json.Unmarshal(body, &parsed)

	message := strings.TrimSpace(parsed.Error.Message)
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	if message == "" {
		message = "Google Vertex returned an error."
	}

	return httputil.ProviderAPIError("Google Vertex", resp.StatusCode, httputil.ProviderErrorDetails{
		Message: message,
		Body:    string(body),
		Status:  parsed.Error.Status,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
