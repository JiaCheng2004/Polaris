package modality

import (
	"encoding/json"
	"testing"
)

func TestAudioSessionConfigJSONRoundTrip(t *testing.T) {
	config := AudioSessionConfig{
		Model:             "openai/realtime-voice",
		Voice:             "alloy",
		Instructions:      "Keep responses short.",
		InputAudioFormat:  AudioFormatPCM16,
		OutputAudioFormat: AudioFormatPCM16,
		SampleRateHz:      16000,
		TurnDetection: &TurnDetectionConfig{
			Mode:            TurnDetectionServerVAD,
			SilenceMS:       600,
			PrefixPaddingMS: 200,
		},
	}

	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded AudioSessionConfig
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded.Model != config.Model || decoded.TurnDetection == nil || decoded.TurnDetection.Mode != TurnDetectionServerVAD {
		t.Fatalf("unexpected decoded config = %#v", decoded)
	}
}

func TestAudioClientEventJSONRoundTrip(t *testing.T) {
	event := AudioClientEvent{
		Type:    AudioClientEventInputAudioAppend,
		EventID: "evt_client_123",
		Audio:   "UklGRiQAAABXQVZF",
	}

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded AudioClientEvent
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded.Type != AudioClientEventInputAudioAppend || decoded.Audio != event.Audio || decoded.EventID != event.EventID {
		t.Fatalf("unexpected decoded event = %#v", decoded)
	}
}

func TestAudioServerEventJSONRoundTrip(t *testing.T) {
	event := AudioServerEvent{
		Type:    AudioServerEventResponseCompleted,
		EventID: "evt_server_456",
		Session: &AudioSessionDescriptor{
			ID:           "audsess_123",
			Object:       "audio.session",
			Model:        "openai/realtime-voice",
			ExpiresAt:    1712699999,
			WebSocketURL: "wss://gateway.example/v1/audio/sessions/audsess_123/ws",
			ClientSecret: "audsec_123",
		},
		ResponseID: "resp_123",
		Transcript: "Hello from Polaris.",
		Usage: &AudioUsage{
			InputAudioSeconds:  1.25,
			OutputAudioSeconds: 0.8,
			InputTextTokens:    12,
			OutputTextTokens:   18,
			TotalTokens:        30,
		},
	}

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded AudioServerEvent
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded.Type != AudioServerEventResponseCompleted || decoded.Session == nil || decoded.Session.Object != "audio.session" || decoded.Usage == nil || decoded.Usage.TotalTokens != 30 {
		t.Fatalf("unexpected decoded event = %#v", decoded)
	}
}
