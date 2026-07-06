package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const (
	defaultMaxResultBytes = 1 << 20 // 1 MiB tool-result cap → truncation error
	maxRequestBytes       = 4 << 20 // 4 MiB request cap
)

// Server dispatches MCP JSON-RPC over streamable HTTP against a ToolSource. It is
// stateless: no session id, no initialize handshake requirement. A POST is
// answered as a single JSON body, or — when the client sends
// `Accept: text/event-stream` — as SSE frames on the POST response.
type Server struct {
	serverVersion  string
	maxResultBytes int
}

// NewServer builds a Server advertising serverVersion in serverInfo.
func NewServer(serverVersion string) *Server {
	if serverVersion == "" {
		serverVersion = "dev"
	}
	return &Server{serverVersion: serverVersion, maxResultBytes: defaultMaxResultBytes}
}

// Handle processes a POST JSON-RPC request (single or batch) against source. GET
// and other verbs return 405 — the gateway handler owns any GET metadata path.
func (s *Server) Handle(w http.ResponseWriter, r *http.Request, source ToolSource) {
	if r.Method != http.MethodPost {
		writeHTTPError(w, http.StatusMethodNotAllowed, "only POST is supported on this MCP endpoint")
		return
	}
	if v := strings.TrimSpace(r.Header.Get(ProtocolVersionHeader)); v != "" && !ProtocolVersionSupported(v) {
		writeHTTPError(w, http.StatusBadRequest, "unsupported MCP-Protocol-Version: "+v)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes))
	if err != nil {
		s.write(w, r, false, []Response{newFailure(nil, NewError(ErrCodeParse, "unable to read request body", nil))})
		return
	}
	requests, isBatch, err := DecodeRequests(body)
	if err != nil {
		s.write(w, r, false, []Response{newFailure(nil, NewError(ErrCodeParse, "parse error: "+err.Error(), nil))})
		return
	}

	responses := make([]Response, 0, len(requests))
	for i := range requests {
		if requests[i].IsNotification() {
			continue // notifications (incl. notifications/cancelled) get no response
		}
		responses = append(responses, s.dispatch(r.Context(), requests[i], source))
	}
	if len(responses) == 0 {
		w.WriteHeader(http.StatusAccepted) // all-notifications payload
		return
	}
	s.write(w, r, isBatch, responses)
}

func (s *Server) dispatch(ctx context.Context, req Request, source ToolSource) Response {
	switch req.Method {
	case MethodInitialize:
		return newResult(req.ID, s.initializeResult(req))
	case MethodPing:
		return newResult(req.ID, struct{}{})
	case MethodToolsList:
		tools, next, err := source.ListTools(ctx, parseCursor(req.Params))
		if err != nil {
			return newFailure(req.ID, NewError(ErrCodeInternal, err.Error(), nil))
		}
		if tools == nil {
			tools = []Tool{}
		}
		result := map[string]any{"tools": tools}
		if next != "" {
			result["nextCursor"] = next
		}
		return newResult(req.ID, result)
	case MethodToolsCall:
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil || strings.TrimSpace(params.Name) == "" {
			return newFailure(req.ID, NewError(ErrCodeInvalidParams, "tools/call requires a tool name", nil))
		}
		result, err := source.CallTool(ctx, strings.TrimSpace(params.Name), params.Arguments)
		if err != nil {
			return newFailure(req.ID, NewError(ErrCodeInvalidParams, err.Error(), nil))
		}
		if s.maxResultBytes > 0 {
			if encoded, mErr := json.Marshal(result); mErr == nil && len(encoded) > s.maxResultBytes {
				result = ToolResult{Content: TextContent("tool result exceeded the size limit"), IsError: true}
			}
		}
		return newResult(req.ID, result)
	default:
		return newFailure(req.ID, NewError(ErrCodeMethodNotFound, "method not found: "+req.Method, nil))
	}
}

func (s *Server) initializeResult(req Request) map[string]any {
	version := ProtocolVersionLatest
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(req.Params) > 0 {
		_ = json.Unmarshal(req.Params, &params)
	}
	if params.ProtocolVersion != "" && ProtocolVersionSupported(params.ProtocolVersion) {
		version = params.ProtocolVersion
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      map[string]any{"name": ServerName, "version": s.serverVersion},
	}
}

func parseCursor(params json.RawMessage) string {
	if len(params) == 0 {
		return ""
	}
	var p struct {
		Cursor string `json:"cursor"`
	}
	_ = json.Unmarshal(params, &p)
	return p.Cursor
}

func wantsSSE(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

func (s *Server) write(w http.ResponseWriter, r *http.Request, isBatch bool, responses []Response) {
	if wantsSSE(r) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, resp := range responses {
			writeSSEFrame(w, resp)
			if flusher != nil {
				flusher.Flush()
			}
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if isBatch {
		_ = json.NewEncoder(w).Encode(responses)
		return
	}
	_ = json.NewEncoder(w).Encode(responses[0])
}

func writeSSEFrame(w io.Writer, resp Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_, _ = w.Write([]byte("event: message\ndata: "))
	_, _ = w.Write(data)
	_, _ = w.Write([]byte("\n\n"))
}

func writeHTTPError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": message}})
}
