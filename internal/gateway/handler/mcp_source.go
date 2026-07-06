package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/metrics"
	"github.com/JiaCheng2004/Polaris/internal/mcp"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/tooling"
)

// localToolSource adapts a store-backed toolset + the local tool registry to the
// mcp.ToolSource interface.
type localToolSource struct {
	store   store.Store
	tools   *tooling.Registry
	toolset store.Toolset
	metrics *metrics.Recorder
}

func (s *localToolSource) ListTools(ctx context.Context, _ string) ([]mcp.Tool, string, error) {
	out := make([]mcp.Tool, 0, len(s.toolset.ToolIDs))
	for _, id := range s.toolset.ToolIDs {
		tool, err := s.store.GetToolDefinition(ctx, id)
		if err != nil {
			return nil, "", fmt.Errorf("toolset references an unknown tool")
		}
		out = append(out, mcp.Tool{Name: tool.Name, Description: tool.Description, InputSchema: rawSchema(tool.InputSchema)})
	}
	return out, "", nil
}

func (s *localToolSource) CallTool(ctx context.Context, name string, args json.RawMessage) (mcp.ToolResult, error) {
	var target *store.ToolDefinition
	for _, id := range s.toolset.ToolIDs {
		tool, err := s.store.GetToolDefinition(ctx, id)
		if err != nil {
			continue
		}
		if strings.EqualFold(tool.Name, name) {
			target = tool
			break
		}
	}
	if target == nil {
		return mcp.ToolResult{}, fmt.Errorf("tool is not part of this toolset: %s", name)
	}
	result, err := s.tools.Execute(ctx, target.Implementation, args)
	if err != nil {
		if s.metrics != nil {
			s.metrics.IncToolInvocation(target.Name, "error")
		}
		return mcp.ToolResult{}, err
	}
	if s.metrics != nil {
		s.metrics.IncToolInvocation(target.Name, "ok")
	}
	return mcp.ToolResult{Content: mcp.TextContent(result.Text), StructuredContent: result.Structured, IsError: false}, nil
}

func rawSchema(s string) json.RawMessage {
	if strings.TrimSpace(s) == "" {
		return json.RawMessage("{}")
	}
	if !json.Valid([]byte(s)) {
		return json.RawMessage("{}")
	}
	return json.RawMessage(s)
}

// upstreamClientFor builds an mcp.Client for an upstream_proxy binding, injecting
// the binding's configured headers.
func (h *MCPHandler) upstreamClientFor(binding store.MCPBinding) *mcp.Client {
	return mcp.NewClient(binding.UpstreamURL, parseStringMap(binding.HeadersJSON), h.client)
}

// localMetadata is the GET metadata document for a binding (backward-compatible
// with the pre-rewrite shape).
func localMetadata(binding store.MCPBinding) map[string]any {
	return map[string]any{
		"binding_id":       binding.ID,
		"kind":             binding.Kind,
		"toolset_id":       binding.ToolsetID,
		"transport":        "streamable_http",
		"protocol_version": mcp.ProtocolVersionLatest,
		"capabilities":     map[string]any{"tools": true},
	}
}
