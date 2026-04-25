package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultTimeout = time.Minute

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func New(baseURL string, opts ...Option) (*Client, error) {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return nil, fmt.Errorf("baseURL is required")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parse baseURL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("baseURL must include scheme and host")
	}

	client := &Client{
		baseURL: strings.TrimRight(trimmed, "/"),
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}

	for _, opt := range opts {
		if err := opt(client); err != nil {
			return nil, err
		}
	}
	if client.httpClient == nil {
		client.httpClient = &http.Client{Timeout: defaultTimeout}
	}

	return client, nil
}

func (c *Client) doJSON(ctx context.Context, method string, path string, query url.Values, body any, out any) error {
	resp, err := c.do(ctx, method, path, query, body, "application/json", "application/json")
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		return decodeAPIErrorResponse(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode JSON response: %w", err)
	}
	return nil
}

func (c *Client) doBinary(ctx context.Context, method string, path string, query url.Values, body any) ([]byte, string, error) {
	resp, err := c.do(ctx, method, path, query, body, "application/json", "*/*")
	if err != nil {
		return nil, "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, "", decodeAPIErrorResponse(resp)
	}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read binary response: %w", err)
	}
	return payload, resp.Header.Get("Content-Type"), nil
}

func (c *Client) do(ctx context.Context, method string, path string, query url.Values, body any, contentType string, accept string) (*http.Response, error) {
	var reader io.Reader
	switch value := body.(type) {
	case nil:
	case io.Reader:
		reader = value
	default:
		payload, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("marshal JSON body: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := c.newRequest(ctx, method, path, query, reader)
	if err != nil {
		return nil, err
	}
	if contentType != "" && body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s failed: %w", method, path, err)
	}
	return resp, nil
}

func (c *Client) newRequest(ctx context.Context, method string, path string, query url.Values, body io.Reader) (*http.Request, error) {
	urlString := strings.TrimRight(c.baseURL, "/") + path
	if len(query) > 0 {
		urlString += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, urlString, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	return req, nil
}
