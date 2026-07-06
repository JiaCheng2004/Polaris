package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type stubSource struct {
	tools []Tool
	calls map[string]ToolResult
	err   error
}

func (s *stubSource) ListTools(context.Context, string) ([]Tool, string, error) {
	if s.err != nil {
		return nil, "", s.err
	}
	return s.tools, "", nil
}

func (s *stubSource) CallTool(_ context.Context, name string, _ json.RawMessage) (ToolResult, error) {
	if r, ok := s.calls[name]; ok {
		return r, nil
	}
	return ToolResult{}, fmt.Errorf("no such tool: %s", name)
}

func post(srv *Server, source ToolSource, accept, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	w := httptest.NewRecorder()
	srv.Handle(w, req, source)
	return w
}

func decodeResp(t *testing.T, w *httptest.ResponseRecorder) Response {
	t.Helper()
	var r Response
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, w.Body.String())
	}
	return r
}

func TestServerInitializeNegotiates(t *testing.T) {
	srv := NewServer("1.0.0")
	w := post(srv, &stubSource{}, "", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"protocolVersion":"2025-06-18"`) || !strings.Contains(body, `"name":"polaris"`) || !strings.Contains(body, `"version":"1.0.0"`) {
		t.Fatalf("initialize result missing fields: %s", body)
	}
	// Unsupported requested version → server falls back to its latest.
	w = post(srv, &stubSource{}, "", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`)
	if !strings.Contains(w.Body.String(), `"protocolVersion":"`+ProtocolVersionLatest+`"`) {
		t.Fatalf("expected fallback to latest: %s", w.Body.String())
	}
}

func TestServerToolsListAndCall(t *testing.T) {
	srv := NewServer("1.0.0")
	source := &stubSource{
		tools: []Tool{{Name: "echo", Description: "echoes", InputSchema: json.RawMessage(`{"type":"object"}`)}},
		calls: map[string]ToolResult{"echo": {Content: TextContent("hello"), IsError: false}},
	}
	w := post(srv, source, "", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if !strings.Contains(w.Body.String(), `"name":"echo"`) {
		t.Fatalf("tools/list missing echo: %s", w.Body.String())
	}
	w = post(srv, source, "", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"text":"hi"}}}`)
	if !strings.Contains(w.Body.String(), `"text":"hello"`) || !strings.Contains(w.Body.String(), `"isError":false`) {
		t.Fatalf("tools/call result wrong: %s", w.Body.String())
	}
}

func TestServerPingAndMethodNotFound(t *testing.T) {
	srv := NewServer("1.0.0")
	if w := post(srv, &stubSource{}, "", `{"jsonrpc":"2.0","id":4,"method":"ping"}`); decodeResp(t, w).Error != nil {
		t.Fatal("ping should succeed")
	}
	w := post(srv, &stubSource{}, "", `{"jsonrpc":"2.0","id":5,"method":"resources/list"}`)
	r := decodeResp(t, w)
	if r.Error == nil || r.Error.Code != ErrCodeMethodNotFound {
		t.Fatalf("expected method-not-found, got %+v", r.Error)
	}
}

func TestServerNotificationGetsNoResponse(t *testing.T) {
	srv := NewServer("1.0.0")
	w := post(srv, &stubSource{}, "", `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if w.Code != http.StatusAccepted || w.Body.Len() != 0 {
		t.Fatalf("notification should yield 202 no-body, got %d body=%q", w.Code, w.Body.String())
	}
}

func TestServerSSEResponse(t *testing.T) {
	srv := NewServer("1.0.0")
	source := &stubSource{calls: map[string]ToolResult{"echo": {Content: TextContent("hi"), IsError: false}}}
	w := post(srv, source, "text/event-stream", `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"echo"}}`)
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected SSE content-type, got %q", ct)
	}
	if !strings.Contains(w.Body.String(), "event: message\ndata: ") || !strings.Contains(w.Body.String(), `"text":"hi"`) {
		t.Fatalf("SSE frame malformed: %q", w.Body.String())
	}
}

func TestServerBatch(t *testing.T) {
	srv := NewServer("1.0.0")
	w := post(srv, &stubSource{}, "", `[{"jsonrpc":"2.0","id":1,"method":"ping"},{"jsonrpc":"2.0","id":2,"method":"ping"}]`)
	var arr []Response
	if err := json.Unmarshal(w.Body.Bytes(), &arr); err != nil || len(arr) != 2 {
		t.Fatalf("batch response wrong: %v %s", err, w.Body.String())
	}
}

func TestServerRejectsBadVersionHeaderAndGet(t *testing.T) {
	srv := NewServer("1.0.0")
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set(ProtocolVersionHeader, "1999-01-01")
	w := httptest.NewRecorder()
	srv.Handle(w, req, &stubSource{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad version header should 400, got %d", w.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/mcp", nil)
	w = httptest.NewRecorder()
	srv.Handle(w, req, &stubSource{})
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET should 405, got %d", w.Code)
	}
}

func TestClientRoundTripAgainstServer(t *testing.T) {
	srv := NewServer("1.0.0")
	source := &stubSource{
		tools: []Tool{{Name: "add", Description: "adds"}},
		calls: map[string]ToolResult{"add": {Content: TextContent("5"), IsError: false}},
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.Handle(w, r, source)
	}))
	defer upstream.Close()

	client := NewClient(upstream.URL, nil, upstream.Client())
	tools, _, err := client.ListTools(context.Background(), "")
	if err != nil || len(tools) != 1 || tools[0].Name != "add" {
		t.Fatalf("client ListTools = %v, %v", tools, err)
	}
	res, err := client.CallTool(context.Background(), "add", json.RawMessage(`{"a":2,"b":3}`))
	if err != nil || len(res.Content) == 0 || res.Content[0].Text != "5" {
		t.Fatalf("client CallTool = %+v, %v", res, err)
	}
}

func TestServerOversizedResultTruncates(t *testing.T) {
	srv := NewServer("1.0.0")
	huge := strings.Repeat("x", (1<<20)+16)
	source := &stubSource{calls: map[string]ToolResult{"big": {Content: TextContent(huge), IsError: false}}}
	w := post(srv, source, "", `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"big"}}`)
	body := w.Body.String()
	if !strings.Contains(body, "exceeded the size limit") || !strings.Contains(body, `"isError":true`) {
		t.Fatalf("oversized result should truncate to an error, got %d bytes", len(body))
	}
}

func TestClientLegacyFallback(t *testing.T) {
	var sawLegacy bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(ProtocolVersionHeader) == ProtocolVersionLatest {
			w.WriteHeader(http.StatusBadRequest) // pretend we only speak legacy
			return
		}
		sawLegacy = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"legacy_tool"}]}}`))
	}))
	defer upstream.Close()

	client := NewClient(upstream.URL, nil, upstream.Client())
	tools, _, err := client.ListTools(context.Background(), "")
	if err != nil || !sawLegacy || len(tools) != 1 || tools[0].Name != "legacy_tool" {
		t.Fatalf("legacy fallback failed: tools=%v legacy=%v err=%v", tools, sawLegacy, err)
	}
}

func FuzzServerHandle(f *testing.F) {
	for _, seed := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"ping"}`,
		`[{"jsonrpc":"2.0","method":"notifications/initialized"}]`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"x"}}`,
		`not json at all`,
		`[]`,
		``,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, body string) {
		srv := NewServer("1.0.0")
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		w := httptest.NewRecorder()
		srv.Handle(w, req, &stubSource{calls: map[string]ToolResult{"x": {Content: TextContent("ok")}}}) // must never panic
	})
}

func TestAggregateNamespacingAndRouting(t *testing.T) {
	local := &stubSource{
		tools: []Tool{{Name: "echo"}},
		calls: map[string]ToolResult{"echo": {Content: TextContent("local"), IsError: false}},
	}
	upstream := &stubSource{
		tools: []Tool{{Name: "search"}},
		calls: map[string]ToolResult{"search": {Content: TextContent("remote"), IsError: false}},
	}
	failing := &stubSource{err: fmt.Errorf("down")}

	agg := NewAggregate(time.Minute)
	agg.Add("", local)
	agg.Add("web", upstream)
	agg.Add("dead", failing)

	tools, _, err := agg.ListTools(context.Background(), "")
	if err != nil {
		t.Fatalf("aggregate list: %v", err)
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Name] = true
	}
	if !names["echo"] || !names["web:search"] {
		t.Fatalf("aggregate names = %v (want echo + web:search, failing source skipped)", names)
	}
	// Route by namespace.
	if r, _ := agg.CallTool(context.Background(), "echo", nil); r.Content[0].Text != "local" {
		t.Fatal("bare name should route to local")
	}
	if r, _ := agg.CallTool(context.Background(), "web:search", nil); r.Content[0].Text != "remote" {
		t.Fatal("namespaced name should route to upstream")
	}
	if _, err := agg.CallTool(context.Background(), "nope:x", nil); err == nil {
		t.Fatal("unknown namespace should error")
	}
}
