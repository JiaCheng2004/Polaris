package googlevertex

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
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

type Client struct {
	core           *transport.Client
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

	tokenSource, tokenSourceErr := google.DefaultTokenSource(context.Background(), cloudPlatformScope)

	return newClient(baseURL, timeout, strings.TrimSpace(cfg.ProjectID), strings.TrimSpace(cfg.Location), tokenSource, tokenSourceErr)
}

// newClient wires a Client and its transport core; shared by NewClient and tests
// (which inject a static token source and mock base URL).
func newClient(baseURL string, timeout time.Duration, projectID, location string, tokenSource oauth2.TokenSource, tokenSourceErr error) *Client {
	c := &Client{
		baseURL:        baseURL,
		projectID:      projectID,
		location:       location,
		tokenSource:    tokenSource,
		tokenSourceErr: tokenSourceErr,
	}
	c.core = transport.New(transport.Options{
		BaseURL:      baseURL,
		ProviderName: "Google Vertex",
		ProviderSlug: "google-vertex",
		Auth:         c.authorize,
		Timeout:      timeout,
		// Vertex generation submits are not idempotent: keep single-attempt.
		Retry: transport.RetryPolicy{MaxAttempts: 1},
	})
	c.httpClient = c.core.HTTPClient()
	return c
}

// authorize fetches (and refreshes) the Vertex ADC token per attempt.
func (c *Client) authorize(req *http.Request, _ []byte) error {
	if c.tokenSourceErr != nil {
		return apierror.ProviderAuthError("Google Vertex", "Google Vertex ADC credentials are not available.")
	}
	token, err := c.tokenSource.Token()
	if err != nil {
		return apierror.ProviderAuthError("Google Vertex", "Google Vertex access token request failed.")
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	return nil
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
		return apierror.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Google Vertex returned an invalid JSON response.")
	}
	return nil
}

func (c *Client) do(ctx context.Context, method string, path string, body any, accept string) (*http.Response, error) {
	var payload []byte
	if body != nil {
		marshaled, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal google vertex request: %w", err)
		}
		payload = marshaled
	}
	ct := ""
	if body != nil {
		ct = "application/json"
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

	return apierror.ProviderAPIError("Google Vertex", resp.StatusCode, apierror.ProviderErrorDetails{
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
