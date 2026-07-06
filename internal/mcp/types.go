// Package mcp implements a stateless, streamable-HTTP Model Context Protocol
// server and upstream client. It is transport- and storage-agnostic: the gateway
// adapts its auth/store/registry into the ToolSource interface and drives the
// Server, so this package stays below the gateway layer. All protocol constants
// live in this file so reconciling a future spec revision is a one-file change.
package mcp

import (
	"context"
	"encoding/json"
)

// Protocol revisions Polaris speaks. Latest is preferred on negotiation; Legacy
// is accepted from older clients and used as the upstream-client fallback.
const (
	ProtocolVersionLatest = "2025-06-18"
	ProtocolVersionLegacy = "2025-03-26"
)

// SupportedProtocolVersions is the negotiation set, most-preferred first.
var SupportedProtocolVersions = []string{ProtocolVersionLatest, ProtocolVersionLegacy}

// ProtocolVersionSupported reports whether v is a version Polaris speaks.
func ProtocolVersionSupported(v string) bool {
	for _, s := range SupportedProtocolVersions {
		if s == v {
			return true
		}
	}
	return false
}

// JSON-RPC method names (v1 surface: tools only).
const (
	MethodInitialize  = "initialize"
	MethodPing        = "ping"
	MethodToolsList   = "tools/list"
	MethodToolsCall   = "tools/call"
	MethodInitialized = "notifications/initialized"
	MethodCancelled   = "notifications/cancelled"
)

// JSON-RPC 2.0 reserved error codes.
const (
	ErrCodeParse          = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternal       = -32603
)

// ProtocolVersionHeader carries the negotiated version on every post-initialize
// request in streamable HTTP.
const ProtocolVersionHeader = "MCP-Protocol-Version"

// ServerName is the MCP serverInfo.name Polaris advertises.
const ServerName = "polaris"

// Tool is one callable tool in a tools/list result.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

// ContentBlock is one element of a tool result's content array.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// TextContent builds a single text content block.
func TextContent(text string) []ContentBlock { return []ContentBlock{{Type: "text", Text: text}} }

// ToolResult is the result of a tools/call.
type ToolResult struct {
	Content           []ContentBlock `json:"content"`
	StructuredContent any            `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError"`
}

// ToolSource is what the Server dispatches against: a set of listable, callable
// tools. Local toolsets, upstream MCP servers, and aggregates all implement it.
type ToolSource interface {
	ListTools(ctx context.Context, cursor string) (tools []Tool, nextCursor string, err error)
	CallTool(ctx context.Context, name string, arguments json.RawMessage) (ToolResult, error)
}
