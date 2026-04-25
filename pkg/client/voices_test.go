package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListVoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if r.URL.Path != "/v1/voices" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("scope"); got != "provider" {
			t.Fatalf("unexpected scope %q", got)
		}
		if got := r.URL.Query().Get("provider"); got != "bytedance" {
			t.Fatalf("unexpected provider %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "2" {
			t.Fatalf("unexpected limit %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object":"list",
			"scope":"provider",
			"provider":"bytedance",
			"data":[{"id":"zh_female_vv_uranus_bigtts","name":"Uranus","type":"builtin"}]
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, WithAPIKey("secret"))
	resp, err := client.ListVoices(context.Background(), &VoiceListRequest{
		Provider: "bytedance",
		Scope:    "provider",
		Limit:    2,
	})
	if err != nil {
		t.Fatalf("ListVoices() error = %v", err)
	}
	if resp.Scope != "provider" || resp.Provider != "bytedance" || len(resp.Data) != 1 {
		t.Fatalf("unexpected response %#v", resp)
	}
	if resp.Data[0].ID != "zh_female_vv_uranus_bigtts" || resp.Data[0].Name != "Uranus" {
		t.Fatalf("unexpected voice payload %#v", resp.Data[0])
	}
}

func TestListVoicesDecodesConfigScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(VoiceList{
			Object: "list",
			Scope:  "config",
			Data: []VoiceItem{
				{ID: "alloy", Provider: "openai", Type: "configured", Models: []string{"openai/tts-1"}},
			},
		}); err != nil {
			t.Fatalf("Encode() error = %v", err)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	resp, err := client.ListVoices(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListVoices() error = %v", err)
	}
	if resp.Scope != "config" || len(resp.Data) != 1 || resp.Data[0].ID != "alloy" {
		t.Fatalf("unexpected response %#v", resp)
	}
}

func TestArchiveAndUnarchiveVoice(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1:
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected archive method %s", r.Method)
			}
			if r.URL.Path != "/v1/voices/custom-voice/archive" {
				t.Fatalf("unexpected archive path %s", r.URL.Path)
			}
			if got := r.URL.Query().Get("provider"); got != "bytedance" {
				t.Fatalf("unexpected archive provider %q", got)
			}
			if got := r.URL.Query().Get("model"); got != "bytedance/doubao-tts-2.0" {
				t.Fatalf("unexpected archive model %q", got)
			}
			w.WriteHeader(http.StatusNoContent)
		case 2:
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected unarchive method %s", r.Method)
			}
			if r.URL.Path != "/v1/voices/custom-voice/unarchive" {
				t.Fatalf("unexpected unarchive path %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected extra request %d", calls)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	req := &VoiceListRequest{Provider: "bytedance", Model: "bytedance/doubao-tts-2.0"}
	if err := client.ArchiveVoice(context.Background(), "custom-voice", req); err != nil {
		t.Fatalf("ArchiveVoice() error = %v", err)
	}
	if err := client.UnarchiveVoice(context.Background(), "custom-voice", req); err != nil {
		t.Fatalf("UnarchiveVoice() error = %v", err)
	}
}
