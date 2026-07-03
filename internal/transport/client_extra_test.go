package transport

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/apierror"
)

func TestReqOptionEdgeCases(t *testing.T) {
	var r Request
	WithHeaders(nil)(&r) // empty map is a no-op
	if r.Headers != nil {
		t.Fatalf("WithHeaders(nil) should not allocate: %v", r.Headers)
	}
	WithHeader("A", "1")(&r)
	WithHeaders(map[string]string{"B": "2"})(&r)
	if r.Headers["A"] != "1" || r.Headers["B"] != "2" {
		t.Fatalf("headers = %v", r.Headers)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "x", "y"); got != "x" {
		t.Fatalf("firstNonEmpty = %q", got)
	}
	if got := firstNonEmpty("", "   "); got != "" {
		t.Fatalf("firstNonEmpty(all empty) = %q", got)
	}
}

func TestAccessors(t *testing.T) {
	c := New(Options{BaseURL: "https://api.example.com/", ProviderName: "Acme", Retry: RetryPolicy{MaxAttempts: 1}})
	if c.BaseURL() != "https://api.example.com" {
		t.Fatalf("BaseURL = %q", c.BaseURL())
	}
	if c.ProviderName() != "Acme" {
		t.Fatalf("ProviderName = %q", c.ProviderName())
	}
	if c.HTTPClient() == nil {
		t.Fatal("HTTPClient nil")
	}
}

func TestWithAcceptAndStaticHeaders(t *testing.T) {
	var (
		gotAccept string
		gotStatic string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		gotStatic = r.Header.Get("X-Static")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(Options{
		BaseURL:       srv.URL,
		ProviderName:  "Acme",
		Retry:         RetryPolicy{MaxAttempts: 1},
		StaticHeaders: map[string]string{"X-Static": "yes"},
		HTTPClient:    &http.Client{Timeout: 5 * time.Second},
	})
	if err := c.JSON(context.Background(), http.MethodPost, "/", map[string]int{}, nil, WithAccept("text/event-stream"), WithHeaders(map[string]string{"X-Extra": "1"})); err != nil {
		t.Fatalf("JSON() = %v", err)
	}
	if gotAccept != "text/event-stream" {
		t.Fatalf("Accept = %q", gotAccept)
	}
	if gotStatic != "yes" {
		t.Fatalf("static header = %q", gotStatic)
	}
}

type flakyRoundTripper struct {
	failuresLeft int32
	base         http.RoundTripper
}

func (f *flakyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if atomic.AddInt32(&f.failuresLeft, -1) >= 0 {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("simulated transport failure")}
	}
	return f.base.RoundTrip(req)
}

func TestRetriesTransportErrorThenSucceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(Options{
		BaseURL:      srv.URL,
		ProviderName: "Acme",
		Retry:        RetryPolicy{MaxAttempts: 2, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond},
		HTTPClient:   &http.Client{Timeout: 5 * time.Second, Transport: &flakyRoundTripper{failuresLeft: 1, base: http.DefaultTransport}},
	})
	if err := c.JSON(context.Background(), http.MethodPost, "/", map[string]int{}, nil); err != nil {
		t.Fatalf("JSON() = %v (transport error should have been retried)", err)
	}
}

func TestStreamErrorStatusTranslated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := testClient(t, srv.URL, RetryPolicy{MaxAttempts: 1})
	_, err := c.Stream(context.Background(), http.MethodPost, "/", map[string]bool{"stream": true})
	var apiErr *apierror.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadRequest {
		t.Fatalf("Stream() err = %v, want 400 APIError", err)
	}
}

func TestRawSuccessAndErrorStatus(t *testing.T) {
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte("raw-ok"))
	}))
	defer srv.Close()

	c := testClient(t, srv.URL, RetryPolicy{MaxAttempts: 1})
	resp, err := c.Raw(context.Background(), http.MethodGet, "/asset", nil, "")
	if err != nil {
		t.Fatalf("Raw() = %v", err)
	}
	data, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(data) != "raw-ok" {
		t.Fatalf("raw body = %q", data)
	}

	fail.Store(true)
	if _, err := c.Raw(context.Background(), http.MethodGet, "/asset", nil, ""); err == nil {
		t.Fatal("Raw() error = nil, want 403")
	}
}

func TestJSONInvalidResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{not json`))
	}))
	defer srv.Close()

	c := testClient(t, srv.URL, RetryPolicy{MaxAttempts: 1})
	var out struct {
		X int `json:"x"`
	}
	err := c.JSON(context.Background(), http.MethodPost, "/", map[string]int{}, &out)
	var apiErr *apierror.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "provider_invalid_response" {
		t.Fatalf("JSON() err = %v, want provider_invalid_response", err)
	}
}

func TestJSONNoContentSkipsDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := testClient(t, srv.URL, RetryPolicy{MaxAttempts: 1})
	var out struct {
		X int `json:"x"`
	}
	if err := c.JSON(context.Background(), http.MethodDelete, "/x", nil, &out); err != nil {
		t.Fatalf("JSON() = %v", err)
	}
}

func TestAbsoluteURLPathBypassesBaseURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/absolute" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(Options{BaseURL: "https://ignored.example.com", ProviderName: "Acme", Retry: RetryPolicy{MaxAttempts: 1}, HTTPClient: &http.Client{Timeout: 5 * time.Second}})
	if err := c.JSON(context.Background(), http.MethodPost, srv.URL+"/absolute", map[string]int{}, nil); err != nil {
		t.Fatalf("JSON() = %v", err)
	}
}
