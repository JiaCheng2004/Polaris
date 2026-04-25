package client

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Option func(*Client) error

func WithAPIKey(apiKey string) Option {
	return func(client *Client) error {
		client.apiKey = strings.TrimSpace(apiKey)
		return nil
	}
}

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
