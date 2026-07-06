package gateway

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

// updateGolden regenerates the committed wire-compat fixtures:
//
//	go test ./internal/gateway -run TestGoldenWireContracts -update-golden
//
// The golden suite is the wire-compatibility safety net: it replays a fixed set
// of requests through the full engine against deterministic provider mocks and
// asserts the client-visible response is byte-identical to the committed
// fixture. It guards the surfaces most sensitive to internal change — provider
// request/response translation, SSE framing, and the OpenAI-compatible error
// envelope — so any accidental wire change fails loudly. Route/method parity for
// the full endpoint set is enforced separately by tests/contract.
var updateGolden = flag.Bool("update-golden", false, "regenerate golden wire-compat fixtures")

type goldenCase struct {
	name       string
	method     string
	path       string
	body       string
	headers    map[string]string
	kind       string // json | sse
	wantStatus int
}

func TestGoldenWireContracts(t *testing.T) {
	openai := httptest.NewServer(goldenOpenAIMock(t))
	defer openai.Close()
	anthropicSrv := httptest.NewServer(goldenAnthropicMock(t))
	defer anthropicSrv.Close()

	cfg := goldenConfig(openai.URL+"/v1", anthropicSrv.URL)
	engine := newTestEngine(t, cfg)

	for _, tc := range goldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			var bodyReader *strings.Reader
			if tc.body != "" {
				bodyReader = strings.NewReader(tc.body)
			} else {
				bodyReader = strings.NewReader("")
			}
			req := httptest.NewRequest(tc.method, tc.path, bodyReader)
			req.Header.Set("Content-Type", "application/json")
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d\nbody: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}

			got := normalizeGolden(t, tc.kind, rec.Body.Bytes())
			path := filepath.Join("testdata", "golden", tc.name+goldenExt(tc.kind))
			if *updateGolden {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("mkdir golden dir: %v", err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatalf("write golden %s: %v", path, err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v (run with -update-golden to create)", path, err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("golden mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", tc.name, got, want)
			}
		})
	}
}

func goldenCases() []goldenCase {
	return []goldenCase{
		{
			name:       "chat_completion_openai",
			method:     http.MethodPost,
			path:       "/v1/chat/completions",
			body:       `{"model":"openai/gpt-4o","messages":[{"role":"user","content":"ping"}]}`,
			kind:       "json",
			wantStatus: http.StatusOK,
		},
		{
			name:       "chat_completion_via_alias",
			method:     http.MethodPost,
			path:       "/v1/chat/completions",
			body:       `{"model":"default-chat","messages":[{"role":"user","content":"ping"}]}`,
			kind:       "json",
			wantStatus: http.StatusOK,
		},
		{
			name:       "chat_completion_stream_openai",
			method:     http.MethodPost,
			path:       "/v1/chat/completions",
			body:       `{"model":"openai/gpt-4o","stream":true,"messages":[{"role":"user","content":"ping"}]}`,
			kind:       "sse",
			wantStatus: http.StatusOK,
		},
		{
			name:       "embeddings_openai",
			method:     http.MethodPost,
			path:       "/v1/embeddings",
			body:       `{"model":"openai/text-embedding-3-small","input":"hello"}`,
			kind:       "json",
			wantStatus: http.StatusOK,
		},
		{
			name:       "messages_native_anthropic",
			method:     http.MethodPost,
			path:       "/v1/messages",
			body:       `{"model":"anthropic/claude-sonnet-4-6","max_tokens":16,"messages":[{"role":"user","content":"ping"}]}`,
			kind:       "json",
			wantStatus: http.StatusOK,
		},
		{
			name:       "chat_via_anthropic_translation",
			method:     http.MethodPost,
			path:       "/v1/chat/completions",
			body:       `{"model":"anthropic/claude-sonnet-4-6","messages":[{"role":"user","content":"ping"}]}`,
			kind:       "json",
			wantStatus: http.StatusOK,
		},
		{
			name:       "error_missing_model",
			method:     http.MethodPost,
			path:       "/v1/chat/completions",
			body:       `{"messages":[{"role":"user","content":"ping"}]}`,
			kind:       "json",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "error_unknown_model",
			method:     http.MethodPost,
			path:       "/v1/chat/completions",
			body:       `{"model":"openai/does-not-exist","messages":[{"role":"user","content":"ping"}]}`,
			kind:       "json",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "error_unknown_alias",
			method:     http.MethodPost,
			path:       "/v1/chat/completions",
			body:       `{"model":"no-such-alias","messages":[{"role":"user","content":"ping"}]}`,
			kind:       "json",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "error_invalid_json",
			method:     http.MethodPost,
			path:       "/v1/chat/completions",
			body:       `{not json`,
			kind:       "json",
			wantStatus: http.StatusBadRequest,
		},
	}
}

func goldenConfig(openaiBaseURL, anthropicBaseURL string) *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Host:            "127.0.0.1",
			Port:            8080,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			ShutdownTimeout: 5 * time.Second,
			MaxBodyBytes:    config.DefaultMaxBodyBytes,
		},
		Auth: config.AuthConfig{Mode: config.AuthModeNone},
		Cache: config.CacheConfig{
			Driver:    "memory",
			RateLimit: config.RateLimitConfig{Enabled: false},
		},
		Providers: map[string]config.ProviderConfig{
			"openai": {
				APIKey:  "sk-openai",
				BaseURL: openaiBaseURL,
				Timeout: time.Minute,
				Models: map[string]config.ModelConfig{
					"gpt-4o": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityJSONMode},
					},
					"text-embedding-3-small": {
						Modality:   modality.ModalityEmbed,
						Dimensions: 1536,
					},
				},
			},
			"anthropic": {
				APIKey:  "sk-anthropic",
				BaseURL: anthropicBaseURL,
				Timeout: time.Minute,
				Models: map[string]config.ModelConfig{
					"claude-sonnet-4-6": {
						Modality:        modality.ModalityChat,
						Capabilities:    []modality.Capability{modality.CapabilityStreaming, modality.CapabilityFunctionCalling},
						MaxOutputTokens: 16000,
					},
				},
			},
		},
		Routing: config.RoutingConfig{
			Aliases: map[string]string{"default-chat": "openai/gpt-4o"},
		},
		Observability: config.ObservabilityConfig{
			Metrics: config.MetricsConfig{Enabled: true, Path: "/metrics"},
			Logging: config.LoggingConfig{Level: "error", Format: "json"},
		},
	}
}

func goldenOpenAIMock(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			var body struct {
				Stream bool `json:"stream"`
			}
			raw, _ := readAllReset(r)
			_ = json.Unmarshal(raw, &body)
			if body.Stream {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-golden\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n"))
				_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-golden\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"pong\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n"))
				_, _ = w.Write([]byte("data: [DONE]\n\n"))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"chatcmpl-golden","object":"chat.completion","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
		case "/v1/embeddings":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","model":"text-embedding-3-small","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2,0.3]}],"usage":{"prompt_tokens":1,"total_tokens":1}}`))
		default:
			t.Errorf("unexpected openai mock request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func goldenAnthropicMock(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected anthropic mock request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_golden","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"pong"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}
}

func readAllReset(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(r.Body)
	return buf.Bytes(), err
}

func goldenExt(kind string) string {
	if kind == "sse" {
		return ".sse"
	}
	return ".json"
}

// normalizeGolden makes the client-visible payload deterministic and diffable.
// Provider mocks return fixed ids/timestamps, so the only normalization needed
// is canonical JSON (stable key order + indentation) per document, applied per
// SSE data frame for streams.
func normalizeGolden(t *testing.T, kind string, body []byte) []byte {
	t.Helper()
	if kind == "sse" {
		var out bytes.Buffer
		for _, line := range strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n") {
			line = strings.TrimRight(line, " \t")
			if !strings.HasPrefix(line, "data:") {
				if line == "" {
					out.WriteByte('\n')
				}
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "[DONE]" {
				out.WriteString("data: [DONE]\n")
				continue
			}
			out.WriteString("data: ")
			out.Write(canonicalJSONGolden(t, []byte(payload)))
			out.WriteByte('\n')
		}
		return out.Bytes()
	}
	return append(canonicalJSONGolden(t, body), '\n')
}

func canonicalJSONGolden(t *testing.T, raw []byte) []byte {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		// Non-JSON body (should not happen for these cases) — return verbatim.
		return bytes.TrimRight(raw, "\n")
	}
	out, err := marshalSorted(v)
	if err != nil {
		t.Fatalf("canonicalize golden json: %v", err)
	}
	return out
}

func marshalSorted(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(sortAny(v)); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// goldenVolatileKeys are response fields whose value is inherently non-repeatable
// (unix timestamps some adapters synthesize with time.Now, e.g. the Anthropic
// chat translation at internal/provider/anthropic/chat.go). They are redacted to
// a stable sentinel so the fixture stays deterministic while still asserting the
// field is present. Everything else is compared exactly.
var goldenVolatileKeys = map[string]struct{}{
	"created":      {},
	"created_at":   {},
	"completed_at": {},
	"expires_at":   {},
}

// sortAny recursively normalizes decoded JSON: redacts volatile keys and lets
// json.Encoder emit deterministic key order.
func sortAny(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for k := range typed {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		ordered := make(map[string]any, len(typed))
		for _, k := range keys {
			if _, volatile := goldenVolatileKeys[k]; volatile {
				ordered[k] = "<redacted>"
				continue
			}
			ordered[k] = sortAny(typed[k])
		}
		return ordered
	case []any:
		for i := range typed {
			typed[i] = sortAny(typed[i])
		}
		return typed
	default:
		return v
	}
}
