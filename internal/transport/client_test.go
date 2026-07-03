package transport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/apierror"
)

func testClient(t *testing.T, baseURL string, retry RetryPolicy) *Client {
	t.Helper()
	return New(Options{
		BaseURL:      baseURL,
		ProviderName: "TestProvider",
		Retry:        retry,
		HTTPClient:   &http.Client{Timeout: 5 * time.Second},
	})
}

func TestJSONSuccessDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"q":"ping"}` {
			t.Errorf("body = %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answer":"pong"}`))
	}))
	defer srv.Close()

	client := testClient(t, srv.URL, RetryPolicy{MaxAttempts: 1})
	var out struct {
		Answer string `json:"answer"`
	}
	if err := client.JSON(context.Background(), http.MethodPost, "/v1/x", map[string]string{"q": "ping"}, &out); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if out.Answer != "pong" {
		t.Fatalf("answer = %q", out.Answer)
	}
}

func TestJSONErrorTranslatorInvoked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"oops":"bad"}`))
	}))
	defer srv.Close()

	client := New(Options{
		BaseURL:      srv.URL,
		ProviderName: "TestProvider",
		Retry:        RetryPolicy{MaxAttempts: 1},
		HTTPClient:   &http.Client{Timeout: 5 * time.Second},
		ErrorTranslator: func(_ string, status int, body []byte) *apierror.APIError {
			return apierror.NewError(status, "invalid_request_error", "custom_code", "", string(body))
		},
	})
	err := client.JSON(context.Background(), http.MethodPost, "/v1/x", map[string]string{"q": "x"}, nil)
	var apiErr *apierror.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want APIError", err)
	}
	if apiErr.Code != "custom_code" || apiErr.Status != http.StatusBadRequest {
		t.Fatalf("apiErr = %#v", apiErr)
	}
}

func TestRetriesRetryableStatusThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	client := testClient(t, srv.URL, RetryPolicy{MaxAttempts: 3, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond})
	if err := client.JSON(context.Background(), http.MethodPost, "/", map[string]int{"a": 1}, nil); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
}

func TestNoRetryWhenMaxAttemptsOne(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	// MaxAttempts=1 mirrors a non-idempotent provider (minimax/elevenlabs/vertex):
	// the 5xx must surface after exactly one attempt, never a retry.
	client := testClient(t, srv.URL, RetryPolicy{MaxAttempts: 1})
	resp, err := client.Do(context.Background(), Request{Method: http.MethodPost, Path: "/", Body: []byte(`{}`)})
	if err != nil {
		t.Fatalf("Do() unexpected transport error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
}

// TestInterruptedBackoffReturnsErrorNotDrainedResponse is the B2 regression: the
// old per-provider loops, when a backoff sleep was interrupted after a retryable
// status, returned the drained+closed response as if it were a success. Do must
// return an error and no response instead.
func TestInterruptedBackoffReturnsErrorNotDrainedResponse(t *testing.T) {
	restore := backoffSleep
	t.Cleanup(func() { backoffSleep = restore })
	// Simulate an interrupted sleep on the first (and only) backoff.
	backoffSleep = func(context.Context, time.Duration) error { return context.Canceled }

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := testClient(t, srv.URL, RetryPolicy{MaxAttempts: 3, InitialDelay: time.Second})
	resp, err := client.Do(context.Background(), Request{Method: http.MethodPost, Path: "/", Body: []byte(`{}`)})
	if resp != nil {
		_ = resp.Body.Close()
		t.Fatalf("Do() returned a response after interrupted backoff; want nil")
	}
	if err == nil {
		t.Fatal("Do() error = nil after interrupted backoff; want error")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("calls = %d, want 1 (no retry after interrupted sleep)", got)
	}
}

func TestRequestBodyReplayedIdenticallyAcrossRetries(t *testing.T) {
	var (
		mu     sync.Mutex
		bodies []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(data))
		n := len(bodies)
		mu.Unlock()
		if n == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := testClient(t, srv.URL, RetryPolicy{MaxAttempts: 2, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond})
	if err := client.JSON(context.Background(), http.MethodPost, "/", map[string]string{"payload": "value"}, nil); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("attempts = %d, want 2", len(bodies))
	}
	if bodies[0] != bodies[1] || bodies[0] != `{"payload":"value"}` {
		t.Fatalf("replayed bodies differ: %q vs %q", bodies[0], bodies[1])
	}
}

func TestBodyReaderForcesSingleAttempt(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := testClient(t, srv.URL, RetryPolicy{MaxAttempts: 3, InitialDelay: time.Millisecond})
	_, err := client.Raw(context.Background(), http.MethodPost, "/", bytes.NewReader([]byte("stream-body")), "application/octet-stream")
	if err == nil {
		t.Fatal("Raw() error = nil, want error for 500")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("calls = %d, want 1 (BodyReader must not be replayed)", got)
	}
}

func TestAuthFuncSeesBodyAndDecoratesRequest(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("X-Signature")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := New(Options{
		BaseURL:      srv.URL,
		ProviderName: "TestProvider",
		Retry:        RetryPolicy{MaxAttempts: 1},
		HTTPClient:   &http.Client{Timeout: 5 * time.Second},
		Auth: func(req *http.Request, body []byte) error {
			req.Header.Set("X-Signature", "sig:"+string(body))
			return nil
		},
	})
	if err := client.JSON(context.Background(), http.MethodPost, "/", map[string]string{"k": "v"}, nil); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if gotAuth != `sig:{"k":"v"}` {
		t.Fatalf("auth header = %q", gotAuth)
	}
}

func TestHooksCalledPerAttempt(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	var mu sync.Mutex
	var infos []AttemptInfo
	client := New(Options{
		BaseURL:      srv.URL,
		ProviderName: "TestProvider",
		Retry:        RetryPolicy{MaxAttempts: 2, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond},
		HTTPClient:   &http.Client{Timeout: 5 * time.Second},
		Hooks: []AttemptHook{func(info AttemptInfo) {
			mu.Lock()
			infos = append(infos, info)
			mu.Unlock()
		}},
	})
	if err := client.JSON(context.Background(), http.MethodPost, "/", map[string]int{}, nil); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(infos) != 2 {
		t.Fatalf("hook calls = %d, want 2", len(infos))
	}
	if infos[0].Status != http.StatusInternalServerError || infos[1].Status != http.StatusOK {
		t.Fatalf("hook statuses = %d,%d", infos[0].Status, infos[1].Status)
	}
	if infos[0].Provider != "TestProvider" {
		t.Fatalf("hook provider = %q", infos[0].Provider)
	}
}

func TestStreamReturnsBodyForReading(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: hello\n\ndata: [DONE]\n\n"))
	}))
	defer srv.Close()

	client := testClient(t, srv.URL, RetryPolicy{MaxAttempts: 1})
	resp, err := client.Stream(context.Background(), http.MethodPost, "/", map[string]bool{"stream": true})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(data), "hello") {
		t.Fatalf("stream body = %q", data)
	}
}

func TestQueryAndPerRequestHeaders(t *testing.T) {
	var (
		gotQuery  string
		gotHeader string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		gotHeader = r.Header.Get("anthropic-beta")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := testClient(t, srv.URL, RetryPolicy{MaxAttempts: 1})
	q := map[string][]string{"model": {"gpt-4o"}}
	if err := client.JSON(context.Background(), http.MethodGet, "/models", nil, nil, WithQuery(q), WithHeader("anthropic-beta", "files-api-2025-04-14")); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if gotQuery != "model=gpt-4o" {
		t.Fatalf("query = %q", gotQuery)
	}
	if gotHeader != "files-api-2025-04-14" {
		t.Fatalf("beta header = %q", gotHeader)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		value string
		want  time.Duration
		ok    bool
	}{
		{"empty", "", 0, false},
		{"seconds", "5", 5 * time.Second, true},
		{"zero", "0", 0, true},
		{"negative", "-3", 0, false},
		{"garbage", "soon", 0, false},
		{"http-date", now.Add(10 * time.Second).Format(http.TimeFormat), 10 * time.Second, true},
		{"past-date", now.Add(-time.Hour).Format(http.TimeFormat), 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseRetryAfter(tc.value, now)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("delay = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBackoffWithinBounds(t *testing.T) {
	p := RetryPolicy{InitialDelay: 100 * time.Millisecond, MaxDelay: time.Second}
	for attempt := 1; attempt <= 8; attempt++ {
		for i := 0; i < 200; i++ {
			d := p.backoff(attempt)
			if d < 0 || d > time.Second {
				t.Fatalf("attempt %d backoff %v out of [0,1s]", attempt, d)
			}
		}
	}
}

func TestTransportErrorTranslated(t *testing.T) {
	// No server listening -> connection refused -> transport error, no retry.
	client := testClient(t, "http://127.0.0.1:1", RetryPolicy{MaxAttempts: 1})
	err := client.JSON(context.Background(), http.MethodPost, "/", map[string]int{}, nil)
	var apiErr *apierror.APIError
	if !errors.As(err, &apiErr) || apiErr.Type != "provider_error" {
		t.Fatalf("error = %v, want provider_error APIError", err)
	}
}
