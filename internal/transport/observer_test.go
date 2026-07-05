package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestAddObserverFiresWithSlug(t *testing.T) {
	ResetObservers()
	defer ResetObservers()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	var mu sync.Mutex
	var seen []AttemptInfo
	AddObserver(func(info AttemptInfo) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, info)
	})

	c := New(Options{BaseURL: srv.URL, ProviderName: "OpenAI", ProviderSlug: "openai"})
	if err := c.JSON(context.Background(), http.MethodPost, "/v1/x", map[string]string{"a": "b"}, nil); err != nil {
		t.Fatalf("JSON: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 {
		t.Fatalf("observer fired %d times, want 1", len(seen))
	}
	if seen[0].Slug != "openai" {
		t.Fatalf("observed slug = %q, want openai", seen[0].Slug)
	}
	if seen[0].Provider != "OpenAI" || seen[0].Status != http.StatusOK {
		t.Fatalf("observed %+v", seen[0])
	}
}

func TestSlugDefaultsToLowerProviderName(t *testing.T) {
	ResetObservers()
	defer ResetObservers()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	var got string
	AddObserver(func(info AttemptInfo) { got = info.Slug })
	c := New(Options{BaseURL: srv.URL, ProviderName: "MiniMax"})
	_ = c.JSON(context.Background(), http.MethodPost, "/x", nil, nil)
	if got != "minimax" {
		t.Fatalf("default slug = %q, want minimax", got)
	}
}

func TestObserverAndClientHooksBothFire(t *testing.T) {
	ResetObservers()
	defer ResetObservers()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	observerFired, hookFired := false, false
	AddObserver(func(AttemptInfo) { observerFired = true })
	c := New(Options{
		BaseURL:      srv.URL,
		ProviderName: "x",
		Hooks:        []AttemptHook{func(AttemptInfo) { hookFired = true }},
	})
	_ = c.JSON(context.Background(), http.MethodPost, "/x", nil, nil)
	if !observerFired || !hookFired {
		t.Fatalf("observerFired=%v hookFired=%v, want both true", observerFired, hookFired)
	}
}
