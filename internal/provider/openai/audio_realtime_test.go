package openai

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gorilla/websocket"
)

func TestRealtimeAudioSessionUsesNativeOpenAIProtocol(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/realtime" {
			t.Fatalf("unexpected realtime path %s", r.URL.Path)
		}
		if r.URL.Query().Get("model") != "gpt-realtime" {
			t.Fatalf("unexpected realtime model %q", r.URL.Query().Get("model"))
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-openai" {
			t.Fatalf("unexpected Authorization header %q", got)
		}
		if got := r.Header.Get("OpenAI-Beta"); got != "realtime=v1" {
			t.Fatalf("unexpected OpenAI-Beta header %q", got)
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade websocket: %v", err)
		}
		defer func() {
			_ = conn.Close()
		}()

		var sessionUpdate map[string]any
		if err := conn.ReadJSON(&sessionUpdate); err != nil {
			t.Fatalf("read session.update: %v", err)
		}
		if sessionUpdate["type"] != "session.update" {
			t.Fatalf("unexpected first event %#v", sessionUpdate)
		}
		session := sessionUpdate["session"].(map[string]any)
		if session["voice"] != "nova" {
			t.Fatalf("unexpected session voice %#v", session["voice"])
		}
		if session["input_audio_format"] != "pcm16" || session["output_audio_format"] != "pcm16" {
			t.Fatalf("unexpected audio formats %#v", session)
		}
		if err := conn.WriteJSON(map[string]any{"type": "session.updated"}); err != nil {
			t.Fatalf("write session.updated: %v", err)
		}

		var itemCreate map[string]any
		if err := conn.ReadJSON(&itemCreate); err != nil {
			t.Fatalf("read conversation.item.create: %v", err)
		}
		if itemCreate["type"] != "conversation.item.create" {
			t.Fatalf("unexpected item create payload %#v", itemCreate)
		}

		var responseCreate map[string]any
		if err := conn.ReadJSON(&responseCreate); err != nil {
			t.Fatalf("read response.create: %v", err)
		}
		if responseCreate["type"] != "response.create" {
			t.Fatalf("unexpected response create payload %#v", responseCreate)
		}
		response := responseCreate["response"].(map[string]any)
		if response["conversation"] != "auto" {
			t.Fatalf("unexpected response payload %#v", response)
		}
		if modalities := response["modalities"].([]any); len(modalities) != 2 || modalities[0] != "audio" || modalities[1] != "text" {
			t.Fatalf("unexpected response modalities %#v", response["modalities"])
		}
		if response["output_audio_format"] != "pcm16" {
			t.Fatalf("unexpected response audio format %#v", response["output_audio_format"])
		}

		audio24k := pcmSamplesToBase64([]int16{100, 200, 300, 400, 500, 600})
		for _, payload := range []map[string]any{
			{"type": "response.created", "response": map[string]any{"id": "resp_123"}},
			{"type": "response.output_text.delta", "delta": "OPENAI_AUDIO_OK"},
			{"type": "response.output_audio.delta", "delta": audio24k},
			{"type": "response.output_audio.done"},
			{"type": "response.done", "response": map[string]any{"id": "resp_123", "usage": map[string]any{"input_tokens": 4, "output_tokens": 3, "total_tokens": 7}}},
		} {
			if err := conn.WriteJSON(payload); err != nil {
				t.Fatalf("write provider event %#v: %v", payload, err)
			}
		}
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-openai",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewAudioAdapter(client, "openai/gpt-4o-audio", config.ModelConfig{
		Modality:   modality.ModalityAudio,
		SessionTTL: 10 * time.Minute,
		RealtimeSession: config.AudioRealtimeConfig{
			Transport: "openai_realtime",
			Model:     "gpt-realtime",
		},
	})

	session, err := adapter.Connect(context.Background(), &modality.AudioSessionConfig{
		Model:             "openai/gpt-4o-audio",
		Voice:             "nova",
		Instructions:      "Reply with OPENAI_AUDIO_OK only.",
		InputAudioFormat:  modality.AudioFormatPCM16,
		OutputAudioFormat: modality.AudioFormatPCM16,
		SampleRateHz:      16000,
		TurnDetection:     &modality.TurnDetectionConfig{Mode: modality.TurnDetectionManual},
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer func() {
		_ = session.Close()
	}()

	if err := session.Send(modality.AudioClientEvent{Type: modality.AudioClientEventInputText, Text: "Reply with OPENAI_AUDIO_OK only."}); err != nil {
		t.Fatalf("Send(input_text) error = %v", err)
	}
	if err := session.Send(modality.AudioClientEvent{Type: modality.AudioClientEventResponseCreate}); err != nil {
		t.Fatalf("Send(response.create) error = %v", err)
	}

	var (
		seenSessionUpdated bool
		seenTranscript     bool
		seenAudio          bool
		seenCompleted      bool
	)
	for !seenCompleted {
		select {
		case event := <-session.Events():
			switch event.Type {
			case modality.AudioServerEventSessionUpdated:
				seenSessionUpdated = true
			case modality.AudioServerEventResponseTextDelta:
				if event.Text != "OPENAI_AUDIO_OK" {
					t.Fatalf("unexpected text delta %#v", event)
				}
				seenTranscript = true
			case modality.AudioServerEventResponseAudioDelta:
				decoded, err := base64.StdEncoding.DecodeString(event.Audio)
				if err != nil {
					t.Fatalf("decode response audio delta: %v", err)
				}
				if len(decoded) != 8 {
					t.Fatalf("expected 16kHz audio delta to downsample to 4 samples (8 bytes), got %d", len(decoded))
				}
				seenAudio = true
			case modality.AudioServerEventResponseCompleted:
				if event.Usage == nil || event.Usage.TotalTokens != 7 {
					t.Fatalf("unexpected completion event %#v", event)
				}
				seenCompleted = true
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for realtime audio events")
		}
	}
	if !seenSessionUpdated || !seenTranscript || !seenAudio {
		t.Fatalf("missing expected realtime events updated=%v transcript=%v audio=%v", seenSessionUpdated, seenTranscript, seenAudio)
	}
}

func TestRealtimeAudioSessionResamplesManualAudioInput(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade websocket: %v", err)
		}
		defer func() {
			_ = conn.Close()
		}()

		var payload map[string]any
		if err := conn.ReadJSON(&payload); err != nil {
			t.Fatalf("read session.update: %v", err)
		}
		if err := conn.WriteJSON(map[string]any{"type": "session.updated"}); err != nil {
			t.Fatalf("write session.updated: %v", err)
		}

		if err := conn.ReadJSON(&payload); err != nil {
			t.Fatalf("read input_audio_buffer.append: %v", err)
		}
		if payload["type"] != "input_audio_buffer.append" {
			t.Fatalf("unexpected append payload %#v", payload)
		}
		decoded, err := base64.StdEncoding.DecodeString(payload["audio"].(string))
		if err != nil {
			t.Fatalf("decode appended audio: %v", err)
		}
		if len(decoded) != 12 {
			t.Fatalf("expected 24kHz upsampled audio to be 12 bytes, got %d", len(decoded))
		}

		if err := conn.ReadJSON(&payload); err != nil {
			t.Fatalf("read input_audio_buffer.commit: %v", err)
		}
		if payload["type"] != "input_audio_buffer.commit" {
			t.Fatalf("unexpected commit payload %#v", payload)
		}
		if err := conn.ReadJSON(&payload); err != nil {
			t.Fatalf("read response.create: %v", err)
		}
		if payload["type"] != "response.create" {
			t.Fatalf("unexpected response.create payload %#v", payload)
		}
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-openai",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewAudioAdapter(client, "openai/gpt-4o-audio", config.ModelConfig{
		Modality: modality.ModalityAudio,
		RealtimeSession: config.AudioRealtimeConfig{
			Transport: "openai_realtime",
			Model:     "gpt-realtime",
		},
	})
	session, err := adapter.Connect(context.Background(), &modality.AudioSessionConfig{
		Model:             "openai/gpt-4o-audio",
		InputAudioFormat:  modality.AudioFormatPCM16,
		OutputAudioFormat: modality.AudioFormatPCM16,
		SampleRateHz:      16000,
		TurnDetection:     &modality.TurnDetectionConfig{Mode: modality.TurnDetectionManual},
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer func() {
		_ = session.Close()
	}()

	if err := session.Send(modality.AudioClientEvent{
		Type:  modality.AudioClientEventInputAudioAppend,
		Audio: pcmSamplesToBase64([]int16{100, 200, 300, 400}),
	}); err != nil {
		t.Fatalf("Send(input_audio.append) error = %v", err)
	}
	if err := session.Send(modality.AudioClientEvent{Type: modality.AudioClientEventResponseCreate}); err != nil {
		t.Fatalf("Send(response.create) error = %v", err)
	}
}

func pcmSamplesToBase64(samples []int16) string {
	bytes := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(bytes[i*2:], uint16(sample))
	}
	return base64.StdEncoding.EncodeToString(bytes)
}
