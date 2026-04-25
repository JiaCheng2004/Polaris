package handler

import (
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
)

func TestAudioSessionTokenRoundTrip(t *testing.T) {
	snapshot := tokenTestSnapshot()
	model := provider.Model{
		ID:       "bytedance/doubao-audio",
		Provider: "bytedance",
	}
	cfg := modality.AudioSessionConfig{
		Model:             model.ID,
		Voice:             "zh_female_vv_jupiter_bigtts",
		InputAudioFormat:  modality.AudioFormatPCM16,
		OutputAudioFormat: modality.AudioFormatPCM16,
		SampleRateHz:      16000,
	}

	issued, err := issueAudioSession(snapshot, model, "key_123", cfg, time.Minute)
	if err != nil {
		t.Fatalf("issueAudioSession() error = %v", err)
	}
	parsed, err := parseAudioSession(snapshot, issued.ID, issued.ClientSecret)
	if err != nil {
		t.Fatalf("parseAudioSession() error = %v", err)
	}
	if parsed.Model != model.ID {
		t.Fatalf("parsed.Model = %q, want %q", parsed.Model, model.ID)
	}
	if parsed.KeyID != "key_123" {
		t.Fatalf("parsed.KeyID = %q, want %q", parsed.KeyID, "key_123")
	}
	if parsed.Config.Voice != cfg.Voice {
		t.Fatalf("parsed.Config.Voice = %q, want %q", parsed.Config.Voice, cfg.Voice)
	}
}

func TestStreamingTranscriptionSessionTokenRoundTrip(t *testing.T) {
	snapshot := tokenTestSnapshot()
	model := provider.Model{
		ID:       "bytedance/doubao-streaming-asr-2.0",
		Provider: "bytedance",
	}
	interim := true
	utterances := true
	cfg := modality.StreamingTranscriptionSessionConfig{
		Model:            model.ID,
		InputAudioFormat: modality.AudioFormatPCM16,
		SampleRateHz:     16000,
		InterimResults:   &interim,
		ReturnUtterances: &utterances,
	}

	issued, err := issueStreamingTranscriptionSession(snapshot, model, "key_456", cfg, time.Minute)
	if err != nil {
		t.Fatalf("issueStreamingTranscriptionSession() error = %v", err)
	}
	parsed, err := parseStreamingTranscriptionSession(snapshot, issued.ID, issued.ClientSecret)
	if err != nil {
		t.Fatalf("parseStreamingTranscriptionSession() error = %v", err)
	}
	if parsed.Model != model.ID {
		t.Fatalf("parsed.Model = %q, want %q", parsed.Model, model.ID)
	}
	if parsed.KeyID != "key_456" {
		t.Fatalf("parsed.KeyID = %q, want %q", parsed.KeyID, "key_456")
	}
	if parsed.Config.SampleRateHz != cfg.SampleRateHz {
		t.Fatalf("parsed.Config.SampleRateHz = %d, want %d", parsed.Config.SampleRateHz, cfg.SampleRateHz)
	}
}

func tokenTestSnapshot() *gwruntime.Snapshot {
	return &gwruntime.Snapshot{
		Config: &config.Config{
			Providers: map[string]config.ProviderConfig{
				"bytedance": {
					APIKey: "ark-key",
				},
			},
		},
	}
}
