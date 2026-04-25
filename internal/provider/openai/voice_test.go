package openai

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestVoiceAdapterTextToSpeech(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("unexpected Authorization header %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "tts-1" {
			t.Fatalf("expected stripped provider model, got %#v", payload["model"])
		}
		if payload["voice"] != "nova" {
			t.Fatalf("unexpected voice %#v", payload["voice"])
		}
		if payload["response_format"] != "wav" {
			t.Fatalf("unexpected response_format %#v", payload["response_format"])
		}

		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFFtest"))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewVoiceAdapter(client, "openai/tts-1")

	response, err := adapter.TextToSpeech(context.Background(), &modality.TTSRequest{
		Model:          "openai/tts-1",
		Input:          "Hello from Polaris",
		Voice:          "nova",
		ResponseFormat: "wav",
	})
	if err != nil {
		t.Fatalf("TextToSpeech() error = %v", err)
	}
	if got := string(response.Data); got != "RIFFtest" {
		t.Fatalf("unexpected audio body %q", got)
	}
	if response.ContentType != "audio/wav" {
		t.Fatalf("unexpected content type %q", response.ContentType)
	}
}

func TestVoiceAdapterSpeechToTextVerboseJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("unexpected Authorization header %q", got)
		}

		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("ParseMediaType() error = %v", err)
		}
		if mediaType != "multipart/form-data" {
			t.Fatalf("unexpected media type %q", mediaType)
		}

		reader := multipart.NewReader(r.Body, params["boundary"])
		fields := map[string][]string{}
		files := map[string][]byte{}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("NextPart() error = %v", err)
			}

			data, err := io.ReadAll(part)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if part.FileName() != "" {
				files[part.FormName()] = data
				continue
			}
			fields[part.FormName()] = append(fields[part.FormName()], string(data))
		}

		if got := fields["model"]; len(got) != 1 || got[0] != "whisper-1" {
			t.Fatalf("unexpected model field %#v", got)
		}
		if got := fields["response_format"]; len(got) != 1 || got[0] != "verbose_json" {
			t.Fatalf("unexpected response_format field %#v", got)
		}
		if got := fields["timestamp_granularities[]"]; len(got) != 1 || got[0] != "segment" {
			t.Fatalf("unexpected timestamp_granularities field %#v", got)
		}
		if got := fields["language"]; len(got) != 1 || got[0] != "en" {
			t.Fatalf("unexpected language field %#v", got)
		}
		if got := fields["temperature"]; len(got) != 1 || got[0] != "0.2" {
			t.Fatalf("unexpected temperature field %#v", got)
		}
		if got := string(files["file"]); got != "wav-bytes" {
			t.Fatalf("unexpected file payload %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"text":"Hello from speech",
			"language":"en",
			"duration":3.42,
			"segments":[{"id":0,"start":0.0,"end":3.42,"text":"Hello from speech"}]
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewVoiceAdapter(client, "openai/whisper-1")
	temperature := 0.2

	response, err := adapter.SpeechToText(context.Background(), &modality.STTRequest{
		Model:          "openai/whisper-1",
		File:           []byte("wav-bytes"),
		Filename:       "sample.wav",
		ContentType:    "audio/wav",
		Language:       "en",
		ResponseFormat: "json",
		Temperature:    &temperature,
	})
	if err != nil {
		t.Fatalf("SpeechToText() error = %v", err)
	}
	if response.Text != "Hello from speech" || response.Language != "en" || response.Duration != 3.42 {
		t.Fatalf("unexpected transcript response %#v", response)
	}
	if len(response.Segments) != 1 || response.Segments[0].Text != "Hello from speech" {
		t.Fatalf("unexpected transcript segments %#v", response.Segments)
	}
}
