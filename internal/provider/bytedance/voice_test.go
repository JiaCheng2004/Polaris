package bytedance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestVoiceAdapterTextToSpeech(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/tts/unidirectional/sse" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Api-Key"); got != "speech-key" {
			t.Fatalf("unexpected X-Api-Key header %q", got)
		}
		if got := r.Header.Get("X-Api-Resource-Id"); got != bytedanceTTSResource2ID {
			t.Fatalf("unexpected X-Api-Resource-Id header %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		user := payload["user"].(map[string]any)
		if user["uid"] == "" {
			t.Fatalf("expected uid")
		}

		reqParams := payload["req_params"].(map[string]any)
		if reqParams["speaker"] != "zh_female_vv_uranus_bigtts" {
			t.Fatalf("unexpected speaker %#v", reqParams["speaker"])
		}
		audio := reqParams["audio_params"].(map[string]any)
		if audio["format"] != "ogg_opus" {
			t.Fatalf("unexpected format %#v", audio["format"])
		}
		if audio["sample_rate"] != float64(24000) {
			t.Fatalf("unexpected sample_rate %#v", audio["sample_rate"])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: 352\ndata: {\"code\":0,\"message\":\"\",\"data\":\"AQID\"}\n\n"))
		_, _ = w.Write([]byte("event: 152\ndata: {\"code\":20000000,\"message\":\"OK\",\"data\":null}\n\n"))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		SpeechAPIKey: "speech-key",
		BaseURL:      "https://unused.example.com/api/v3",
		Timeout:      time.Second,
	})
	adapter := NewVoiceAdapter(client, "bytedance/doubao-tts-2.0", server.URL+"/api/v3/tts/unidirectional/sse")

	response, err := adapter.TextToSpeech(context.Background(), &modality.TTSRequest{
		Model:          "bytedance/doubao-tts-2.0",
		Input:          "ByteDance voice",
		Voice:          "zh_female_vv_uranus_bigtts",
		ResponseFormat: "opus",
	})
	if err != nil {
		t.Fatalf("TextToSpeech() error = %v", err)
	}
	if response.ContentType != "audio/ogg" {
		t.Fatalf("unexpected content type %q", response.ContentType)
	}
	if got := response.Data; len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("unexpected decoded audio %#v", got)
	}
}

func TestVoiceAdapterMapsProviderCodeErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":55000000,"message":"resource ID is mismatched with speaker related resource"}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		SpeechAPIKey: "speech-key",
		BaseURL:      "https://unused.example.com/api/v3",
		Timeout:      time.Second,
	})
	adapter := NewVoiceAdapter(client, "bytedance/doubao-tts-2.0", server.URL)

	_, err := adapter.TextToSpeech(context.Background(), &modality.TTSRequest{
		Model: "bytedance/doubao-tts-2.0",
		Input: "ByteDance voice",
		Voice: "zh_female_vv_uranus_bigtts",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	var apiErr *httputil.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Status != http.StatusBadGateway || apiErr.Code != "provider_auth_failed" {
		t.Fatalf("unexpected api error %#v", apiErr)
	}
}

func TestVoiceAdapterSpeechToText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/auc/bigmodel/recognize/flash" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Api-Key"); got != "speech-key" {
			t.Fatalf("unexpected X-Api-Key header %q", got)
		}
		if got := r.Header.Get("X-Api-Resource-Id"); got != bytedanceSTTResourceID {
			t.Fatalf("unexpected X-Api-Resource-Id header %q", got)
		}
		if got := r.Header.Get("X-Api-Sequence"); got != "-1" {
			t.Fatalf("unexpected X-Api-Sequence header %q", got)
		}
		if got := r.Header.Get("X-Api-Request-Id"); got == "" {
			t.Fatalf("expected X-Api-Request-Id header")
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		user := payload["user"].(map[string]any)
		request := payload["request"].(map[string]any)
		audio := payload["audio"].(map[string]any)
		if user["uid"] != r.Header.Get("X-Api-Request-Id") {
			t.Fatalf("expected uid to match request id, got %#v", user["uid"])
		}
		if request["model_name"] != "bigmodel" {
			t.Fatalf("unexpected model_name %#v", request["model_name"])
		}
		if audio["data"] != base64.StdEncoding.EncodeToString([]byte("wav-bytes")) {
			t.Fatalf("unexpected base64 audio payload %#v", audio["data"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Api-Status-Code", bytedanceSTTSuccessCode)
		_, _ = w.Write([]byte(`{
			"audio_info":{"duration":2500},
			"result":{
				"text":"Hello ByteDance",
				"utterances":[
					{"start_time":0,"end_time":1200,"text":"Hello"},
					{"start_time":1200,"end_time":2500,"text":"ByteDance"}
				]
			}
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		SpeechAPIKey: "speech-key",
		BaseURL:      "https://unused.example.com/api/v3",
		Timeout:      time.Second,
	})
	adapter := NewVoiceAdapter(client, "bytedance/doubao-asr-2.0", server.URL+"/api/v3/auc/bigmodel/recognize/flash")

	response, err := adapter.SpeechToText(context.Background(), &modality.STTRequest{
		Model:          "bytedance/doubao-asr-2.0",
		File:           []byte("wav-bytes"),
		Filename:       "sample.wav",
		ContentType:    "audio/wav",
		Language:       "zh",
		ResponseFormat: "json",
	})
	if err != nil {
		t.Fatalf("SpeechToText() error = %v", err)
	}
	if response.Text != "Hello ByteDance" || response.Language != "zh" || response.Duration != 2.5 {
		t.Fatalf("unexpected transcript response %#v", response)
	}
	if response.ContentType != "application/json" {
		t.Fatalf("unexpected content type %q", response.ContentType)
	}
	if len(response.Segments) != 2 || response.Segments[0].Text != "Hello" || response.Segments[1].Text != "ByteDance" {
		t.Fatalf("unexpected transcript segments %#v", response.Segments)
	}
}

func TestVoiceAdapterSpeechToTextMapsProviderErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Api-Status-Code", "45000151")
		w.Header().Set("X-Api-Message", "unsupported format")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		SpeechAPIKey: "speech-key",
		BaseURL:      "https://unused.example.com/api/v3",
		Timeout:      time.Second,
	})
	adapter := NewVoiceAdapter(client, "bytedance/doubao-asr-2.0", server.URL)

	_, err := adapter.SpeechToText(context.Background(), &modality.STTRequest{
		Model:          "bytedance/doubao-asr-2.0",
		File:           []byte("wav-bytes"),
		Filename:       "sample.wav",
		ContentType:    "audio/wav",
		ResponseFormat: "json",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	var apiErr *httputil.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Status != http.StatusBadRequest || apiErr.Code != "unsupported_audio_format" {
		t.Fatalf("unexpected api error %#v", apiErr)
	}
}
