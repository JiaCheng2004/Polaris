package client

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateSpeech(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFFtest"))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	response, err := client.CreateSpeech(context.Background(), &SpeechRequest{
		Model:          "openai/tts-1",
		Input:          "Hello",
		Voice:          "nova",
		ResponseFormat: "wav",
	})
	if err != nil {
		t.Fatalf("CreateSpeech() error = %v", err)
	}
	if response.ContentType != "audio/wav" || string(response.Data) != "RIFFtest" {
		t.Fatalf("unexpected response %#v", response)
	}
}

func TestCreateTranscriptionJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("ParseMediaType() error = %v", err)
		}
		if mediaType != "multipart/form-data" {
			t.Fatalf("unexpected media type %q", mediaType)
		}

		reader := multipart.NewReader(r.Body, params["boundary"])
		fields := map[string]string{}
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
			fields[part.FormName()] = string(data)
		}

		if fields["model"] != "openai/whisper-1" || fields["response_format"] != "json" {
			t.Fatalf("unexpected fields %#v", fields)
		}
		if fields["routing"] == "" || fields["routing"] == "{}" {
			t.Fatalf("unexpected routing field %#v", fields["routing"])
		}
		if string(files["file"]) != "wav-bytes" {
			t.Fatalf("unexpected file payload %q", files["file"])
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

	client := newTestClient(t, server.URL)
	response, err := client.CreateTranscription(context.Background(), &TranscriptionRequest{
		Model:          "openai/whisper-1",
		Routing:        &RoutingOptions{Providers: []string{"openai"}},
		File:           []byte("wav-bytes"),
		Filename:       "sample.wav",
		ContentType:    "audio/wav",
		Language:       "en",
		ResponseFormat: "json",
	})
	if err != nil {
		t.Fatalf("CreateTranscription() error = %v", err)
	}
	if response.Text != "Hello from speech" || response.Language != "en" || response.Duration != 3.42 {
		t.Fatalf("unexpected response %#v", response)
	}
}

func TestCreateTranscriptionTextFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("hello world"))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	response, err := client.CreateTranscription(context.Background(), &TranscriptionRequest{
		Model:          "openai/whisper-1",
		File:           []byte("wav-bytes"),
		Filename:       "sample.wav",
		ResponseFormat: "text",
	})
	if err != nil {
		t.Fatalf("CreateTranscription() error = %v", err)
	}
	if response.Text != "hello world" || string(response.Raw) != "hello world" {
		t.Fatalf("unexpected response %#v", response)
	}
}
