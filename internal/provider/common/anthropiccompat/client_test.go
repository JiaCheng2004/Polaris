package anthropiccompat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
)

func TestClientInjectsStaticHeaders(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("x-api-key"), "sk-test"; got != want {
			t.Fatalf("x-api-key = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("anthropic-version"), "2023-06-01"; got != want {
			t.Fatalf("anthropic-version = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewClient("test", "Test", config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL,
		Timeout: time.Second,
	}, server.URL, map[string]string{
		"x-api-key":         "sk-test",
		"anthropic-version": "2023-06-01",
	})

	var out map[string]bool
	if _, err := client.JSON(context.Background(), "/v1/messages", map[string]string{"model": "m"}, &out); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if !out["ok"] {
		t.Fatalf("unexpected response %#v", out)
	}
}

func TestClientMapsProviderErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		status     int
		wantStatus int
		wantType   string
		wantCode   string
	}{
		{name: "auth", status: http.StatusUnauthorized, wantStatus: http.StatusBadGateway, wantType: "provider_error", wantCode: "provider_auth_failed"},
		{name: "rate limit", status: http.StatusTooManyRequests, wantStatus: http.StatusTooManyRequests, wantType: "rate_limit_error", wantCode: "rate_limit_error"},
		{name: "server", status: http.StatusInternalServerError, wantStatus: http.StatusBadGateway, wantType: "provider_error", wantCode: "provider_server_error"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"type": "error",
					"error": map[string]any{
						"type":    tc.wantCode,
						"message": "provider failed",
					},
				})
			}))
			defer server.Close()

			client := NewClient("test", "Test", config.ProviderConfig{
				APIKey:  "sk-test",
				BaseURL: server.URL,
				Timeout: time.Second,
			}, server.URL, nil)

			var out map[string]any
			_, err := client.JSON(context.Background(), "/v1/messages", map[string]string{"model": "m"}, &out)
			if err == nil {
				t.Fatal("expected error")
			}
			var apiErr *httputil.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected APIError, got %T", err)
			}
			if apiErr.Status != tc.wantStatus || apiErr.Type != tc.wantType || apiErr.Code != tc.wantCode {
				t.Fatalf("APIError = %#v", apiErr)
			}
		})
	}
}
