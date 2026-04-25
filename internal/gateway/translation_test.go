package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestTranslationEndpointByteDance(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Api-Key"); got != "speech-key" {
			t.Fatalf("unexpected X-Api-Key %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 20000000,
			"message": "ok",
			"data": {
				"translation_list": [
					{
						"translation": "Hola, Polaris.",
						"detected_source_language": "en",
						"usage": {"prompt_tokens": 11, "completion_tokens": 5, "total_tokens": 16}
					}
				]
			}
		}`))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Providers = map[string]config.ProviderConfig{
		"bytedance": {
			SpeechAPIKey: "speech-key",
			Timeout:      time.Second,
			Models: map[string]config.ModelConfig{
				"doubao-translation-2.0": {
					Modality: modality.ModalityTranslation,
					Endpoint: upstream.URL,
				},
			},
		},
	}
	cfg.Routing.Aliases = map[string]string{
		"bytedance-translation": "bytedance/doubao-translation-2.0",
	}

	engine := newTestEngine(t, cfg)
	req := httptest.NewRequest(http.MethodPost, "/v1/translations", bytes.NewBufferString(`{
		"model":"bytedance-translation",
		"input":"Hello, Polaris.",
		"target_language":"es"
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Model        string `json:"model"`
		Translations []struct {
			Text string `json:"text"`
		} `json:"translations"`
		Usage struct {
			PromptTokens     int    `json:"prompt_tokens"`
			CompletionTokens int    `json:"completion_tokens"`
			TotalTokens      int    `json:"total_tokens"`
			Source           string `json:"source"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Model != "bytedance/doubao-translation-2.0" || len(response.Translations) != 1 || response.Translations[0].Text != "Hola, Polaris." {
		t.Fatalf("unexpected response %#v", response)
	}
	if response.Usage.TotalTokens != 16 || response.Usage.Source != "provider_reported" {
		t.Fatalf("unexpected usage %#v", response.Usage)
	}
}

func TestTranslationEndpointRejectsTooManyInputs(t *testing.T) {
	cfg := testConfig(t)
	cfg.Providers = map[string]config.ProviderConfig{
		"bytedance": {
			SpeechAPIKey: "speech-key",
			Timeout:      time.Second,
			Models: map[string]config.ModelConfig{
				"doubao-translation-2.0": {
					Modality: modality.ModalityTranslation,
				},
			},
		},
	}
	cfg.Routing.Aliases = map[string]string{}
	engine := newTestEngine(t, cfg)

	inputs := make([]string, 17)
	for i := range inputs {
		inputs[i] = "item"
	}
	body, err := json.Marshal(map[string]any{
		"model":           "bytedance/doubao-translation-2.0",
		"input":           inputs,
		"target_language": "es",
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/translations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		payload, _ := io.ReadAll(rec.Body)
		t.Fatalf("expected 400, got %d body=%s", rec.Code, string(payload))
	}
}
