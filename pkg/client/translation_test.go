package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateTranslation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if r.URL.Path != "/v1/translations" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var req TranslationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "bytedance-translation" || req.TargetLanguage != "de" {
			t.Fatalf("unexpected request %#v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object":"translation.list",
			"model":"bytedance/doubao-translation-2.0",
			"translations":[{"index":0,"text":"Hallo, Polaris.","detected_source_language":"en"}],
			"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13,"source":"provider_reported"}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, WithAPIKey("secret"))
	resp, err := client.CreateTranslation(context.Background(), &TranslationRequest{
		Model:          "bytedance-translation",
		Input:          NewSingleTranslationInput("Hello, Polaris."),
		TargetLanguage: "de",
	})
	if err != nil {
		t.Fatalf("CreateTranslation() error = %v", err)
	}
	if resp.Model != "bytedance/doubao-translation-2.0" || len(resp.Translations) != 1 {
		t.Fatalf("unexpected response %#v", resp)
	}
	if resp.Translations[0].Text != "Hallo, Polaris." || resp.Usage.TotalTokens != 13 {
		t.Fatalf("unexpected translation payload %#v", resp)
	}
}
