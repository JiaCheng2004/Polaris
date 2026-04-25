package bytedance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestVoiceCatalogAdapterListVoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if r.URL.Query().Get("Action") != bytedanceListVoicesAction {
			t.Fatalf("unexpected action %q", r.URL.Query().Get("Action"))
		}
		if r.URL.Query().Get("Version") != bytedanceListVoicesVersion {
			t.Fatalf("unexpected version %q", r.URL.Query().Get("Version"))
		}
		if got := r.Header.Get("X-Date"); got == "" {
			t.Fatalf("expected X-Date header")
		}
		if got := r.Header.Get("X-Content-Sha256"); got == "" {
			t.Fatalf("expected X-Content-Sha256 header")
		}
		auth := r.Header.Get("Authorization")
		if !strings.Contains(auth, "Credential=test-ak/") || !strings.Contains(auth, "/cn-beijing/speech_saas_prod/request") {
			t.Fatalf("unexpected authorization header %q", auth)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if len(payload) != 0 {
			t.Fatalf("expected empty request body, got %#v", payload)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"ResponseMetadata": {
				"RequestId": "request-1",
				"Action": "ListBigModelTTSTimbres",
				"Region": "cn-beijing",
				"Service": "speech_saas_prod",
				"Version": "2025-05-20"
			},
			"Result": {
				"Timbres": [
					{
						"SpeakerID": "zh_female_vv_uranus_bigtts",
						"TimbreInfos": [
							{
								"SpeakerName": "Uranus",
								"Gender": "female",
								"Age": "adult",
								"Categories": [{"Category":"Narration"}],
								"Emotions": [{
									"Emotion": "General",
									"EmotionType": "general",
									"DemoURL": "https://example.com/demo.mp3",
									"DemoText": "Hello from Uranus."
								}]
							}
						]
					},
					{
						"SpeakerID": "unused_voice",
						"TimbreInfos": []
					}
				]
			}
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-sk",
		ControlBaseURL:  server.URL,
		Timeout:         time.Second,
	})
	adapter := NewVoiceCatalogAdapter(client)

	resp, err := adapter.ListVoices(context.Background(), &modality.VoiceCatalogRequest{
		Provider:           "bytedance",
		Model:              "bytedance/doubao-tts-2.0",
		Scope:              "provider",
		Type:               "builtin",
		ConfiguredVoiceIDs: []string{"zh_female_vv_uranus_bigtts"},
	})
	if err != nil {
		t.Fatalf("ListVoices() error = %v", err)
	}
	if resp.Scope != "provider" || resp.Provider != "bytedance" || len(resp.Data) != 1 {
		t.Fatalf("unexpected response %#v", resp)
	}
	if resp.Data[0].ID != "zh_female_vv_uranus_bigtts" || resp.Data[0].PreviewURL != "https://example.com/demo.mp3" {
		t.Fatalf("unexpected voice payload %#v", resp.Data[0])
	}
}

func TestVoiceCatalogAdapterRejectsCustomType(t *testing.T) {
	client := NewClient(config.ProviderConfig{
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-sk",
		ControlBaseURL:  "https://example.com",
		Timeout:         time.Second,
	})
	adapter := NewVoiceCatalogAdapter(client)

	if _, err := adapter.ListVoices(context.Background(), &modality.VoiceCatalogRequest{Type: "custom"}); err == nil {
		t.Fatalf("expected error")
	}
}
