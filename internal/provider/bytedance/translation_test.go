package bytedance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestTranslationAdapterTranslate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if got := r.Header.Get("X-Api-Key"); got != "speech-key" {
			t.Fatalf("unexpected X-Api-Key %q", got)
		}
		if got := r.Header.Get("X-Api-Resource-Id"); got != bytedanceTranslationResourceID {
			t.Fatalf("unexpected X-Api-Resource-Id %q", got)
		}
		if got := r.Header.Get("X-Api-Request-Id"); got == "" {
			t.Fatalf("expected X-Api-Request-Id header")
		}

		var payload translationRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.TargetLanguage != "fr" || payload.SourceLanguage != "en" {
			t.Fatalf("unexpected language selection %#v", payload)
		}
		if len(payload.TextList) != 2 || payload.TextList[0] != "Hello, Polaris." {
			t.Fatalf("unexpected text list %#v", payload.TextList)
		}
		if payload.Corpus == nil || payload.Corpus.GlossaryList["Polaris"] != "Polaris" {
			t.Fatalf("unexpected glossary %#v", payload.Corpus)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 20000000,
			"message": "ok",
			"data": {
				"translation_list": [
					{
						"translation": "Bonjour, Polaris.",
						"detected_source_language": "en",
						"usage": {"prompt_tokens": 12, "completion_tokens": 4, "total_tokens": 16}
					},
					{
						"translation": "La passerelle est en ligne.",
						"usage": {"prompt_tokens": 15, "completion_tokens": 6, "total_tokens": 21}
					}
				]
			}
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		SpeechAPIKey: "speech-key",
		Timeout:      time.Second,
	})
	adapter := NewTranslationAdapter(client, "bytedance/doubao-translation-2.0", server.URL)

	resp, err := adapter.Translate(context.Background(), &modality.TranslationRequest{
		Model:          "bytedance/doubao-translation-2.0",
		Input:          modality.NewMultiTranslationInput("Hello, Polaris.", "The gateway is online."),
		TargetLanguage: "fr",
		SourceLanguage: "en",
		Glossary: map[string]string{
			"Polaris": "Polaris",
		},
	})
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if resp.Object != "translation.list" || resp.Model != "bytedance/doubao-translation-2.0" {
		t.Fatalf("unexpected translation response metadata %#v", resp)
	}
	if len(resp.Translations) != 2 || resp.Translations[0].Text != "Bonjour, Polaris." {
		t.Fatalf("unexpected translations %#v", resp.Translations)
	}
	if resp.Usage.PromptTokens != 27 || resp.Usage.CompletionTokens != 10 || resp.Usage.TotalTokens != 37 {
		t.Fatalf("unexpected usage %#v", resp.Usage)
	}
	if resp.Usage.Source != modality.TokenCountSourceProviderReported {
		t.Fatalf("unexpected usage source %#v", resp.Usage)
	}
}
