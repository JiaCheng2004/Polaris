package anthropic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func newBatchTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	client := NewClient(config.ProviderConfig{APIKey: "test-key", BaseURL: server.URL})
	return client, server
}

// TestBatchGetUsesGET is the B1 regression: batch retrieval must be a GET, not a
// POST (the previous shared JSON helper always POSTed, yielding 405s in prod).
func TestBatchGetUsesGET(t *testing.T) {
	var (
		mu     sync.Mutex
		method string
		path   string
		body   string
	)
	client, server := newBatchTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		if r.ContentLength > 0 {
			_, _ = r.Body.Read(buf)
		}
		mu.Lock()
		method, path, body = r.Method, r.URL.Path, string(buf)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"id":"batch_123","processing_status":"in_progress"}`))
	})
	defer server.Close()

	adapter := NewBatchAdapter(client, "anthropic")
	if _, err := adapter.Get(context.Background(), "batch_123"); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if method != http.MethodGet {
		t.Fatalf("batch Get method = %s, want GET", method)
	}
	if path != "/v1/messages/batches/batch_123" {
		t.Fatalf("batch Get path = %s", path)
	}
	if strings.TrimSpace(body) != "" {
		t.Fatalf("batch Get should send no body, got %q", body)
	}
}

func TestBatchCreateAndCancelUsePOST(t *testing.T) {
	var (
		mu      sync.Mutex
		methods = map[string]string{}
	)
	client, server := newBatchTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		methods[r.URL.Path] = r.Method
		mu.Unlock()
		_, _ = w.Write([]byte(`{"id":"batch_123","processing_status":"in_progress"}`))
	})
	defer server.Close()

	adapter := NewBatchAdapter(client, "anthropic")
	if _, err := adapter.Create(context.Background(), &modality.BatchRequest{InputFileID: "file_1"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := adapter.Cancel(context.Background(), "batch_123"); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if methods["/v1/messages/batches"] != http.MethodPost {
		t.Fatalf("batch Create method = %s, want POST", methods["/v1/messages/batches"])
	}
	if methods["/v1/messages/batches/batch_123/cancel"] != http.MethodPost {
		t.Fatalf("batch Cancel method = %s, want POST", methods["/v1/messages/batches/batch_123/cancel"])
	}
}
