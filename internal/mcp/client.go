package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is an upstream MCP client speaking streamable HTTP. It implements
// ToolSource by calling a remote server's tools/list and tools/call. On a
// version-mismatch (4xx) it retries once pinned to the legacy protocol version,
// so older upstream servers keep working. Bearer tokens from downstream callers
// are never forwarded here — only the binding's configured headers are sent.
type Client struct {
	url     string
	headers map[string]string
	http    *http.Client
	nextID  int64
}

// NewClient builds an upstream client for url with static headers.
func NewClient(url string, headers map[string]string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &Client{url: strings.TrimRight(url, "/"), headers: headers, http: httpClient}
}

// ListTools fetches the upstream tool catalog (one page).
func (c *Client) ListTools(ctx context.Context, cursor string) ([]Tool, string, error) {
	params := map[string]any{}
	if cursor != "" {
		params["cursor"] = cursor
	}
	raw, err := c.call(ctx, MethodToolsList, params)
	if err != nil {
		return nil, "", err
	}
	var out struct {
		Tools      []Tool `json:"tools"`
		NextCursor string `json:"nextCursor"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, "", fmt.Errorf("mcp upstream: decode tools/list: %w", err)
	}
	return out.Tools, out.NextCursor, nil
}

// CallTool invokes an upstream tool.
func (c *Client) CallTool(ctx context.Context, name string, arguments json.RawMessage) (ToolResult, error) {
	if len(arguments) == 0 {
		arguments = json.RawMessage("{}")
	}
	raw, err := c.call(ctx, MethodToolsCall, map[string]any{"name": name, "arguments": arguments})
	if err != nil {
		return ToolResult{}, err
	}
	var result ToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return ToolResult{}, fmt.Errorf("mcp upstream: decode tools/call: %w", err)
	}
	return result, nil
}

func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	raw, status, err := c.do(ctx, method, params, ProtocolVersionLatest)
	if err == nil {
		return raw, nil
	}
	// Version-mismatch fallback: retry once pinned to the legacy revision.
	if status >= 400 && status < 500 {
		if raw2, _, err2 := c.do(ctx, method, params, ProtocolVersionLegacy); err2 == nil {
			return raw2, nil
		}
	}
	return nil, err
}

func (c *Client) do(ctx context.Context, method string, params any, version string) (json.RawMessage, int, error) {
	c.nextID++
	id, _ := json.Marshal(c.nextID)
	paramBytes, _ := json.Marshal(params)
	reqBody, _ := json.Marshal(Request{JSONRPC: "2.0", ID: id, Method: method, Params: paramBytes})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	httpReq.Header.Set(ProtocolVersionHeader, version)
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxRequestBytes))
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, fmt.Errorf("mcp upstream: status %d", resp.StatusCode)
	}

	payload := body
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		payload = lastSSEData(body)
	}
	var rpc Response
	if err := json.Unmarshal(payload, &rpc); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("mcp upstream: decode response: %w", err)
	}
	if rpc.Error != nil {
		return nil, resp.StatusCode, rpc.Error
	}
	result, _ := json.Marshal(rpc.Result)
	return result, resp.StatusCode, nil
}

// lastSSEData returns the data payload of the final SSE event in body.
func lastSSEData(body []byte) []byte {
	var last []byte
	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimRight(line, "\r")
		if data, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			last = bytes.TrimSpace(data)
		}
	}
	if last == nil {
		return body
	}
	return last
}
