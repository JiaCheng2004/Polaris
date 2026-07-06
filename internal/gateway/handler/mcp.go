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
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/mcp"
	"github.com/JiaCheng2004/Polaris/internal/obs"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/tooling"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
)

type MCPHandler struct {
	runtime *gwruntime.Holder
	store   store.Store
	tools   *tooling.Registry
	metrics *metrics.Recorder
	client  *http.Client
	server  *mcp.Server
}

func NewMCPHandler(runtime *gwruntime.Holder, appStore store.Store, tools *tooling.Registry, recorder *metrics.Recorder) *MCPHandler {
	return &MCPHandler{
		runtime: runtime,
		store:   appStore,
		tools:   tools,
		metrics: recorder,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		server: mcp.NewServer(""),
	}
}

func (h *MCPHandler) Serve(c *gin.Context) {
	ctx, span := obs.StartInternalSpan(c.Request.Context(), "mcp.broker")
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
		snapshot := middleware.RuntimeSnapshot(c, h.runtime)
		if snapshot == nil || snapshot.Config == nil {
			status = "error"
			httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "runtime_unavailable", "", "Runtime configuration is unavailable."))
			return
		}
		if !snapshot.Config.Tools.Enabled {
			status = "disabled"
			httputil.WriteError(c, httputil.NewError(http.StatusNotFound, "invalid_request_error", "tools_disabled", "", "Local tool execution is disabled."))
			return
		}
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

// ServeAggregate exposes every binding the caller's scopes allow through a
// single MCP endpoint. Tools are namespaced `{binding_id}:{tool}`.
func (h *MCPHandler) ServeAggregate(c *gin.Context) {
	ctx, span := obs.StartInternalSpan(c.Request.Context(), "mcp.aggregate")
	defer span.End()
	c.Request = c.Request.WithContext(ctx)
	middleware.SetRequestOutcome(c, middleware.RequestOutcome{InterfaceFamily: "mcp", MCPBinding: "_aggregate"})

	if c.Request.Method == http.MethodGet {
		c.JSON(http.StatusOK, gin.H{
			"transport":        "streamable_http",
			"protocol_version": mcp.ProtocolVersionLatest,
			"capabilities":     gin.H{"tools": true},
			"aggregate":        true,
		})
		return
	}

	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil || snapshot.Config == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "runtime_unavailable", "", "Runtime configuration is unavailable."))
		return
	}
	auth := middleware.GetAuthContext(c)
	bindings, err := h.store.ListMCPBindings(ctx)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}

	agg := mcp.NewAggregate(30 * time.Second)
	for _, binding := range bindings {
		if !binding.Enabled || !middleware.StringScopeAllowed(auth.AllowedMCPBindings, auth.PolicyMCPBindings, binding.ID) {
			continue
		}
		switch binding.Kind {
		case store.MCPBindingKindLocalToolset:
			if !snapshot.Config.Tools.Enabled || !middleware.StringScopeAllowed(auth.AllowedToolsets, auth.PolicyToolsets, binding.ToolsetID) {
				continue
			}
			toolset, err := h.store.GetToolset(ctx, binding.ToolsetID)
			if err != nil {
				continue
			}
			agg.Add(binding.ID, &localToolSource{store: h.store, tools: h.tools, toolset: *toolset, metrics: h.metrics})
		case store.MCPBindingKindUpstreamProxy:
			agg.Add(binding.ID, h.upstreamClientFor(binding))
		}
	}

	if h.metrics != nil {
		h.metrics.IncMCPRequest("_aggregate", "ok")
	}
	h.server.Handle(c.Writer, c.Request, agg)
	c.Abort()
}

func (h *MCPHandler) proxyUpstream(c *gin.Context, binding store.MCPBinding) error {
	ctx, span := obs.StartInternalSpan(c.Request.Context(), "mcp.proxy",
		attribute.String("polaris.mcp_binding_id", binding.ID),
		attribute.String("polaris.mcp_binding_kind", string(binding.Kind)),
	)
	defer span.End()

	targetURL, err := url.Parse(strings.TrimRight(binding.UpstreamURL, "/") + c.Param("path"))
	if err != nil {
		obs.RecordSpanError(span, err)
		return httputil.NewError(http.StatusBadGateway, "provider_error", "mcp_proxy_url_invalid", "binding_id", "Configured MCP upstream URL is invalid.")
	}
	targetURL.RawQuery = c.Request.URL.RawQuery

	var body io.Reader
	if c.Request.Body != nil {
		payload, err := io.ReadAll(c.Request.Body)
		if err != nil {
			obs.RecordSpanError(span, err)
			if httputil.IsRequestBodyTooLarge(err) {
				return httputil.RequestBodyTooLargeError(0)
			}
			return httputil.NewError(http.StatusBadGateway, "provider_error", "mcp_proxy_read_failed", "", "Unable to read MCP request body.")
		}
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, c.Request.Method, targetURL.String(), body)
	if err != nil {
		obs.RecordSpanError(span, err)
		return httputil.NewError(http.StatusBadGateway, "provider_error", "mcp_proxy_build_failed", "", "Unable to build MCP upstream request.")
	}
	copyMCPProxyHeaders(req.Header, c.Request.Header)
	for key, value := range parseStringMap(binding.HeadersJSON) {
		req.Header.Set(key, value)
	}
	obs.InjectHTTPHeaders(ctx, req.Header)

	resp, err := h.client.Do(req)
	if err != nil {
		obs.RecordSpanError(span, err)
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
	ctx, span := obs.StartInternalSpan(c.Request.Context(), "mcp.local_toolset",
		attribute.String("polaris.mcp_binding_id", binding.ID),
		attribute.String("polaris.mcp_binding_kind", string(binding.Kind)),
		attribute.String("polaris.toolset_id", binding.ToolsetID),
	)
	defer span.End()
	c.Request = c.Request.WithContext(ctx)

	if c.Request.Method == http.MethodGet {
		c.JSON(http.StatusOK, localMetadata(binding))
		return nil
	}
	if c.Request.Method != http.MethodPost {
		return httputil.NewError(http.StatusMethodNotAllowed, "invalid_request_error", "method_not_allowed", "", "Local MCP toolsets support GET and POST.")
	}

	toolset, err := h.store.GetToolset(c.Request.Context(), binding.ToolsetID)
	if err != nil {
		obs.RecordSpanError(span, err)
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unknown_toolset", "binding_id", "Referenced toolset was not found.")
	}

	source := &localToolSource{store: h.store, tools: h.tools, toolset: *toolset, metrics: h.metrics}
	h.server.Handle(c.Writer, c.Request, source)
	c.Abort()
	return nil
}

func parseStringMap(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	out := map[string]string{}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func copyMCPProxyHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		if !mcpProxyHeaderAllowed(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func mcpProxyHeaderAllowed(key string) bool {
	switch http.CanonicalHeaderKey(key) {
	case "Accept", "Content-Type", "Last-Event-Id", "Mcp-Protocol-Version", "Mcp-Session-Id":
		return true
	default:
		return false
	}
}

var _ = context.Background
