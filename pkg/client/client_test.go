package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewRejectsEmptyBaseURL(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatalf("expected error for empty baseURL")
	}
}

func TestNewAppliesOptions(t *testing.T) {
	httpClient := &http.Client{}

	client, err := New("http://localhost:8080", WithAPIKey("secret"), WithTimeout(2*time.Second), WithHTTPClient(httpClient))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if client.apiKey != "secret" {
		t.Fatalf("expected api key to be set, got %q", client.apiKey)
	}
	if client.httpClient != httpClient {
		t.Fatalf("expected injected httpClient")
	}
	if client.httpClient.Timeout != 2*time.Second {
		t.Fatalf("expected timeout to be updated, got %s", client.httpClient.Timeout)
	}
}

func TestDoJSONDecodesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"blocked","type":"permission_error","code":"model_not_allowed","param":"model"}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, WithAPIKey("secret"))
	err := client.doJSON(context.Background(), http.MethodGet, "/v1/models", nil, nil, &ModelList{})
	if err == nil {
		t.Fatalf("expected error")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusForbidden || apiErr.Code != "model_not_allowed" {
		t.Fatalf("unexpected API error %#v", apiErr)
	}
}
