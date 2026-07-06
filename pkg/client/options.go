package client

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Option configures a [Client] at construction time. Pass options to [New].
type Option func(*Client) error

// WithAPIKey sets the bearer API key sent on every request.
func WithAPIKey(apiKey string) Option {
	return func(client *Client) error {
		client.apiKey = strings.TrimSpace(apiKey)
		return nil
	}
}

// WithTimeout sets the per-request timeout on the client's HTTP transport. The
// timeout must be greater than zero.
func WithTimeout(timeout time.Duration) Option {
	return func(client *Client) error {
		if timeout <= 0 {
			return fmt.Errorf("timeout must be greater than zero")
		}
		if client.httpClient == nil {
			client.httpClient = &http.Client{}
		}
		client.httpClient.Timeout = timeout
		return nil
	}
}

// WithHTTPClient replaces the underlying [http.Client]. If the supplied client
// has no timeout, any timeout already configured on the Polaris client is
// preserved. This is the hook for custom transports, proxies, or TLS settings.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(client *Client) error {
		if httpClient == nil {
			return fmt.Errorf("httpClient must not be nil")
		}
		if httpClient.Timeout == 0 && client.httpClient != nil && client.httpClient.Timeout > 0 {
			httpClient.Timeout = client.httpClient.Timeout
		}
		client.httpClient = httpClient
		return nil
	}
}
