package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestVoicesEndpointConfigScope(t *testing.T) {
	cfg := testConfig(t)
	cfg.Providers = map[string]config.ProviderConfig{
		"openai": {
			APIKey:  "sk-openai",
			BaseURL: "https://api.openai.com/v1",
			Timeout: time.Second,
			Models: map[string]config.ModelConfig{
				"tts-1": {
					Modality: modality.ModalityVoice,
					Capabilities: []modality.Capability{
						modality.CapabilityTTS,
					},
					Voices: []string{"alloy", "nova"},
				},
			},
		},
		"bytedance": {
			SpeechAPIKey: "speech-key",
			Timeout:      time.Second,
			Models: map[string]config.ModelConfig{
				"doubao-tts-2.0": {
					Modality: modality.ModalityVoice,
					Capabilities: []modality.Capability{
						modality.CapabilityTTS,
					},
					Voices: []string{"zh_female_vv_uranus_bigtts", "nova"},
				},
			},
		},
	}
	cfg.Routing.Aliases = map[string]string{}

	engine := newTestEngine(t, cfg)
	req := httptest.NewRequest(http.MethodGet, "/v1/voices?scope=config", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Scope string `json:"scope"`
		Data  []struct {
			ID       string   `json:"id"`
			Provider string   `json:"provider"`
			Type     string   `json:"type"`
			Models   []string `json:"models"`
		} `json:"data"`
	}
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Scope != "config" || len(resp.Data) != 4 {
		t.Fatalf("unexpected response %#v", resp)
	}
	if resp.Data[0].Type != "configured" {
		t.Fatalf("unexpected configured voice payload %#v", resp.Data[0])
	}
}

func TestVoicesEndpointProviderScopeByteDance(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Action") != "ListBigModelTTSTimbres" {
			t.Fatalf("unexpected action %q", r.URL.Query().Get("Action"))
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
					}
				]
			}
		}`))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Providers = map[string]config.ProviderConfig{
		"bytedance": {
			AccessKeyID:     "test-ak",
			AccessKeySecret: "test-sk",
			ControlBaseURL:  upstream.URL,
			SpeechAPIKey:    "speech-key",
			Timeout:         time.Second,
			Models: map[string]config.ModelConfig{
				"doubao-tts-2.0": {
					Modality: modality.ModalityVoice,
					Capabilities: []modality.Capability{
						modality.CapabilityTTS,
					},
					Voices: []string{"zh_female_vv_uranus_bigtts"},
				},
			},
		},
	}
	cfg.Routing.Aliases = map[string]string{
		"bytedance-tts": "bytedance/doubao-tts-2.0",
	}

	engine := newTestEngine(t, cfg)
	req := httptest.NewRequest(http.MethodGet, "/v1/voices?scope=provider&model=bytedance-tts", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Scope    string `json:"scope"`
		Provider string `json:"provider"`
		Data     []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			PreviewURL  string `json:"preview_url"`
			PreviewText string `json:"preview_text"`
		} `json:"data"`
	}
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Scope != "provider" || resp.Provider != "bytedance" || len(resp.Data) != 1 {
		t.Fatalf("unexpected response %#v", resp)
	}
	if resp.Data[0].ID != "zh_female_vv_uranus_bigtts" || resp.Data[0].Name != "Uranus" {
		t.Fatalf("unexpected provider voice payload %#v", resp.Data[0])
	}
}

func TestVoicesArchiveAndUnarchiveByteDanceProviderVoice(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Action") != "ListBigModelTTSTimbres" {
			t.Fatalf("unexpected action %q", r.URL.Query().Get("Action"))
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
					}
				]
			}
		}`))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Providers = map[string]config.ProviderConfig{
		"bytedance": {
			AccessKeyID:     "test-ak",
			AccessKeySecret: "test-sk",
			ControlBaseURL:  upstream.URL,
			SpeechAPIKey:    "speech-key",
			Timeout:         time.Second,
			Models: map[string]config.ModelConfig{
				"doubao-tts-2.0": {
					Modality: modality.ModalityVoice,
					Capabilities: []modality.Capability{
						modality.CapabilityTTS,
					},
					Voices: []string{"zh_female_vv_uranus_bigtts"},
				},
			},
		},
	}
	cfg.Routing.Aliases = map[string]string{
		"bytedance-tts": "bytedance/doubao-tts-2.0",
	}

	engine := newTestEngine(t, cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/voices/zh_female_vv_uranus_bigtts/archive?provider=bytedance&model=bytedance-tts&type=builtin", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 from archive, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/voices?scope=provider&model=bytedance-tts", nil)
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from list, got %d body=%s", rec.Code, rec.Body.String())
	}
	var listResp struct {
		Data []modality.VoiceCatalogItem `json:"data"`
	}
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.Data) != 0 {
		t.Fatalf("expected archived voice to be hidden, got %#v", listResp.Data)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/voices/zh_female_vv_uranus_bigtts?provider=bytedance&model=bytedance-tts&type=builtin", nil)
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for archived voice get, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/voices?scope=provider&model=bytedance-tts&include_archived=true", nil)
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from archived list, got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&listResp); err != nil {
		t.Fatalf("decode archived list response: %v", err)
	}
	if len(listResp.Data) != 1 || listResp.Data[0].Metadata["archived"] != true {
		t.Fatalf("expected archived metadata in list response, got %#v", listResp.Data)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/voices/zh_female_vv_uranus_bigtts?provider=bytedance&model=bytedance-tts&type=builtin&include_archived=true", nil)
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for archived voice get, got %d body=%s", rec.Code, rec.Body.String())
	}
	var item modality.VoiceCatalogItem
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&item); err != nil {
		t.Fatalf("decode archived voice response: %v", err)
	}
	if item.Metadata["archived"] != true {
		t.Fatalf("expected archived metadata, got %#v", item)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/voices/zh_female_vv_uranus_bigtts/unarchive?provider=bytedance&model=bytedance-tts", nil)
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 from unarchive, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/voices?scope=provider&model=bytedance-tts", nil)
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from final list, got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&listResp); err != nil {
		t.Fatalf("decode final list response: %v", err)
	}
	if len(listResp.Data) != 1 || listResp.Data[0].ID != "zh_female_vv_uranus_bigtts" {
		t.Fatalf("expected voice to be visible again, got %#v", listResp.Data)
	}
}
