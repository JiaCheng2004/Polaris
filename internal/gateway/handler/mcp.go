package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/metrics"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/gateway/telemetry"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/tooling"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
)

type MCPHandler struct {
	store   store.Store
	tools   *tooling.Registry
	metrics *metrics.Recorder
	client  *http.Client
}

func NewMCPHandler(appStore store.Store, tools *tooling.Registry, recorder *metrics.Recorder) *MCPHandler {
	return &MCPHandler{
		store:   appStore,
		tools:   tools,
		metrics: recorder,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (h *MCPHandler) Serve(c *gin.Context) {
	ctx, span := telemetry.StartInternalSpan(c.Request.Context(), "mcp.broker")
	defer span.End()
	c.Request = c.Request.WithContext(ctx)

	auth := middleware.GetAuthContext(c)
	bindingID := strings.TrimSpace(c.Param("binding_id"))
	outcome := middleware.RequestOutcome{
		InterfaceFamily: "mcp",
		MCPBinding:      bindingID,
	}
	middleware.SetRequestOutcome(c, outcome)
	if bindingID == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_binding_id", "binding_id", "MCP binding id is required."))
		return
	}
	span.SetAttributes(attribute.String("polaris.mcp_binding_id", bindingID))
	if !middleware.StringScopeAllowed(auth.AllowedMCPBindings, auth.PolicyMCPBindings, bindingID) {
		httputil.WriteError(c, httputil.NewError(http.StatusForbidden, "permission_error", "mcp_binding_not_allowed", "binding_id", "API key is not permitted to use this MCP binding."))
		return
	}
	binding, err := h.store.GetMCPBinding(c.Request.Context(), bindingID)
	if err != nil {
		if err == store.ErrNotFound {
			httputil.WriteError(c, httputil.NewError(http.StatusNotFound, "invalid_request_error", "binding_not_found", "binding_id", "MCP binding was not found."))
			return
		}
		httputil.WriteError(c, err)
		return
	}
	if !binding.Enabled {
		httputil.WriteError(c, httputil.NewError(http.StatusForbidden, "permission_error", "binding_disabled", "binding_id", "MCP binding is disabled."))
		return
	}
	span.SetAttributes(
		attribute.String("polaris.mcp_binding_kind", string(binding.Kind)),
		attribute.String("polaris.toolset_id", binding.ToolsetID),
	)
	outcome.Toolset = binding.ToolsetID
	middleware.SetRequestOutcome(c, outcome)

	status := "ok"
	defer func() {
		if h.metrics != nil {
			h.metrics.IncMCPRequest(binding.ID, status)
		}
	}()

	switch binding.Kind {
	case store.MCPBindingKindUpstreamProxy:
		if err := h.proxyUpstream(c, *binding); err != nil {
			status = "error"
			httputil.WriteError(c, err)
		}
	case store.MCPBindingKindLocalToolset:
		if !middleware.StringScopeAllowed(auth.AllowedToolsets, auth.PolicyToolsets, binding.ToolsetID) {
			status = "forbidden"
			httputil.WriteError(c, httputil.NewError(http.StatusForbidden, "permission_error", "toolset_not_allowed", "binding_id", "API key is not permitted to use this toolset."))
			return
		}
		if err := h.serveLocalToolset(c, *binding); err != nil {
			status = "error"
			httputil.WriteError(c, err)
		}
	default:
		status = "error"
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_binding_kind", "binding_id", "MCP binding kind is not supported by this runtime."))
	}
}

func (h *MCPHandler) proxyUpstream(c *gin.Context, binding store.MCPBinding) error {
	ctx, span := telemetry.StartInternalSpan(c.Request.Context(), "mcp.proxy",
		attribute.String("polaris.mcp_binding_id", binding.ID),
		attribute.String("polaris.mcp_binding_kind", string(binding.Kind)),
	)
	defer span.End()

	targetURL, err := url.Parse(strings.TrimRight(binding.UpstreamURL, "/") + c.Param("path"))
	if err != nil {
		telemetry.RecordSpanError(span, err)
		return httputil.NewError(http.StatusBadGateway, "provider_error", "mcp_proxy_url_invalid", "binding_id", "Configured MCP upstream URL is invalid.")
	}
	targetURL.RawQuery = c.Request.URL.RawQuery

	var body io.Reader
	if c.Request.Body != nil {
		payload, err := io.ReadAll(c.Request.Body)
		if err != nil {
			telemetry.RecordSpanError(span, err)
			return httputil.NewError(http.StatusBadGateway, "provider_error", "mcp_proxy_read_failed", "", "Unable to read MCP request body.")
		}
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, c.Request.Method, targetURL.String(), body)
	if err != nil {
		telemetry.RecordSpanError(span, err)
		return httputil.NewError(http.StatusBadGateway, "provider_error", "mcp_proxy_build_failed", "", "Unable to build MCP upstream request.")
	}
	copyHeaders(req.Header, c.Request.Header)
	for key, value := range parseStringMap(binding.HeadersJSON) {
		req.Header.Set(key, value)
	}
	telemetry.InjectHTTPHeaders(ctx, req.Header)

	resp, err := h.client.Do(req)
	if err != nil {
		telemetry.RecordSpanError(span, err)
		return httputil.NewError(http.StatusBadGateway, "provider_error", "mcp_proxy_failed", "", "Unable to reach the configured MCP upstream.")
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))

	for key, values := range resp.Header {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
	return nil
}

func (h *MCPHandler) serveLocalToolset(c *gin.Context, binding store.MCPBinding) error {
	ctx, span := telemetry.StartInternalSpan(c.Request.Context(), "mcp.local_toolset",
		attribute.String("polaris.mcp_binding_id", binding.ID),
		attribute.String("polaris.mcp_binding_kind", string(binding.Kind)),
		attribute.String("polaris.toolset_id", binding.ToolsetID),
	)
	defer span.End()
	c.Request = c.Request.WithContext(ctx)

	if c.Request.Method == http.MethodGet {
		c.JSON(http.StatusOK, gin.H{
			"binding_id":   binding.ID,
			"kind":         binding.Kind,
			"toolset_id":   binding.ToolsetID,
			"transport":    "streamable_http",
			"capabilities": gin.H{"tools": true},
		})
		return nil
	}
	if c.Request.Method != http.MethodPost {
		return httputil.NewError(http.StatusMethodNotAllowed, "invalid_request_error", "method_not_allowed", "", "Local MCP toolsets support GET and POST.")
	}

	var req mcpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON-RPC.")
	}
	toolset, err := h.store.GetToolset(c.Request.Context(), binding.ToolsetID)
	if err != nil {
		telemetry.RecordSpanError(span, err)
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unknown_toolset", "binding_id", "Referenced toolset was not found.")
	}

	switch req.Method {
	case "initialize":
		c.JSON(http.StatusOK, mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: gin.H{
				"protocolVersion": "2025-03-26",
				"capabilities": gin.H{
					"tools": gin.H{"listChanged": false},
				},
				"serverInfo": gin.H{
					"name":    "polaris",
					"version": "control-plane-preview",
				},
			},
		})
		return nil
	case "ping":
		c.JSON(http.StatusOK, mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: gin.H{}})
		return nil
	case "tools/list":
		tools, err := h.resolveToolsetTools(c.Request.Context(), *toolset)
		if err != nil {
			return err
		}
		items := make([]gin.H, 0, len(tools))
		for _, tool := range tools {
			items = append(items, gin.H{
				"name":        tool.Name,
				"description": tool.Description,
				"inputSchema": rawJSON(tool.InputSchema),
			})
		}
		c.JSON(http.StatusOK, mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: gin.H{"tools": items}})
		return nil
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_tool_call", "params", "tools/call params must include name and arguments.")
		}
		tool, err := h.lookupToolByName(c.Request.Context(), *toolset, strings.TrimSpace(params.Name))
		if err != nil {
			return err
		}
		result, err := h.tools.Execute(c.Request.Context(), tool.Implementation, params.Arguments)
		if err != nil {
			if h.metrics != nil {
				h.metrics.IncToolInvocation(tool.Name, "error")
			}
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "tool_execution_failed", "params", err.Error())
		}
		if h.metrics != nil {
			h.metrics.IncToolInvocation(tool.Name, "ok")
		}
		c.JSON(http.StatusOK, mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: gin.H{
				"content": []gin.H{
					{"type": "text", "text": result.Text},
				},
				"structuredContent": result.Structured,
				"isError":           false,
			},
		})
		return nil
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_mcp_method", "method", "MCP method is not supported by this Polaris runtime.")
	}
}

func (h *MCPHandler) resolveToolsetTools(ctx context.Context, toolset store.Toolset) ([]store.ToolDefinition, error) {
	tools := make([]store.ToolDefinition, 0, len(toolset.ToolIDs))
	for _, toolID := range toolset.ToolIDs {
		tool, err := h.store.GetToolDefinition(ctx, toolID)
		if err != nil {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unknown_tool", "tool_ids", "Toolset references an unknown tool definition.")
		}
		tools = append(tools, *tool)
	}
	return tools, nil
}

func (h *MCPHandler) lookupToolByName(ctx context.Context, toolset store.Toolset, name string) (*store.ToolDefinition, error) {
	tools, err := h.resolveToolsetTools(ctx, toolset)
	if err != nil {
		return nil, err
	}
	for _, tool := range tools {
		if strings.EqualFold(tool.Name, name) {
			return &tool, nil
		}
	}
	return nil, httputil.NewError(http.StatusNotFound, "invalid_request_error", "tool_not_found", "name", "Tool is not part of this toolset.")
}

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   any             `json:"error,omitempty"`
}

func parseStringMap(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	out := map[string]string{}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func rawJSON(value string) any {
	if strings.TrimSpace(value) == "" {
		return gin.H{}
	}
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return gin.H{}
	}
	return decoded
}

func copyHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		if strings.EqualFold(key, "Host") {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

var _ = context.Background
