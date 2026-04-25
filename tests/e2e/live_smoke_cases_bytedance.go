package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/pkg/client"
)

func liveSmokeByteDanceCases() []liveSmokeCase {
	return []liveSmokeCase{
		{
			name:       "bytedance_chat",
			modelNames: []string{"bytedance-chat"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "ByteDance chat", "bytedance-chat")
				harness.requireEnv(t, policy, "ByteDance chat", "VOLCENGINE_ARK_API_KEY")
				resp, err := harness.chat(ctx, "bytedance-chat", "Reply with DOUBAO_OK only.")
				if err != nil {
					t.Fatalf("ByteDance chat smoke failed: %v", err)
				}
				harness.requireContains(t, resp, "DOUBAO_OK")
			},
		},
		{
			name:       "bytedance_vision",
			modelNames: []string{"bytedance-vision"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "ByteDance vision chat", "bytedance-vision")
				harness.requireEnv(t, policy, "ByteDance vision chat", "VOLCENGINE_ARK_API_KEY")
				resp, err := harness.chatWithImage(ctx, "bytedance-vision", "Reply with BYTEDANCE_VISION_OK only.", tinyTransparentPNGDataURL)
				if err != nil {
					t.Fatalf("ByteDance vision smoke failed: %v", err)
				}
				harness.requireContains(t, resp, "BYTEDANCE_VISION_OK")
			},
		},
		{
			name:       "bytedance_image",
			modelNames: []string{"bytedance-image"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "ByteDance image generation", "bytedance-image")
				harness.requireEnv(t, policy, "ByteDance image generation", "VOLCENGINE_ARK_API_KEY")
				resp, err := harness.client.GenerateImage(ctx, &client.ImageGenerationRequest{
					Model:          "bytedance-image",
					Prompt:         "A tiny red square on a white background",
					ResponseFormat: "url",
				})
				if err != nil {
					t.Fatalf("ByteDance image smoke failed: %v", err)
				}
				if len(resp.Data) != 1 || strings.TrimSpace(firstImageValue(resp.Data[0])) == "" {
					t.Fatalf("unexpected ByteDance image response %#v", resp)
				}
			},
		},
		{
			name:       "bytedance_voice",
			modelNames: []string{"bytedance-tts", "bytedance-stt"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "ByteDance voice", "bytedance-tts", "bytedance-stt")
				harness.requireEnv(t, policy, "ByteDance voice", "VOLCENGINE_OPENSPEECH_API_KEY")
				audio, transcript := harness.voiceRoundTrip(t, ctx, "bytedance-tts", "bytedance-stt", "zh_female_vv_uranus_bigtts", "Hello, Polaris live smoke passed.", "mp3")
				if len(audio.Data) == 0 || strings.TrimSpace(transcript.Text) == "" {
					t.Fatalf("unexpected ByteDance voice round-trip audio=%#v transcript=%#v", audio, transcript)
				}
			},
		},
		{
			name:       "bytedance_video",
			modelNames: []string{"bytedance-video"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "ByteDance video", "bytedance-video")
				harness.requireEnv(t, policy, "ByteDance video", "VOLCENGINE_ARK_API_KEY")
				job, err := harness.client.CreateVideoGeneration(ctx, &client.VideoGenerationRequest{
					Model:       "bytedance-video",
					Prompt:      "A short clip of lanterns floating on a river",
					Duration:    4,
					Resolution:  "720p",
					AspectRatio: "16:9",
				})
				if err != nil {
					t.Fatalf("ByteDance video submit failed: %v", err)
				}
				if strings.TrimSpace(job.JobID) == "" {
					t.Fatalf("expected ByteDance video job id, got %#v", job)
				}
				status, err := harness.client.GetVideoGeneration(ctx, job.JobID)
				if err != nil {
					t.Fatalf("ByteDance video status failed: %v", err)
				}
				if strings.TrimSpace(status.Status) == "" {
					t.Fatalf("expected ByteDance video status, got %#v", status)
				}
			},
		},
		{
			name:       "bytedance_video_fast",
			modelNames: []string{"bytedance-video-fast"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "ByteDance fast video", "bytedance-video-fast")
				harness.requireEnv(t, policy, "ByteDance fast video", "VOLCENGINE_ARK_API_KEY")
				job, err := harness.client.CreateVideoGeneration(ctx, &client.VideoGenerationRequest{
					Model:       "bytedance-video-fast",
					Prompt:      "A short fast-paced clip of lanterns floating on a river",
					Duration:    4,
					Resolution:  "720p",
					AspectRatio: "16:9",
				})
				if err != nil {
					t.Fatalf("ByteDance fast video submit failed: %v", err)
				}
				if strings.TrimSpace(job.JobID) == "" {
					t.Fatalf("expected ByteDance fast video job id, got %#v", job)
				}
				status, err := harness.client.GetVideoGeneration(ctx, job.JobID)
				if err != nil {
					t.Fatalf("ByteDance fast video status failed: %v", err)
				}
				if strings.TrimSpace(status.Status) == "" {
					t.Fatalf("expected ByteDance fast video status, got %#v", status)
				}
			},
		},
		{
			name:       "bytedance_audio_session",
			modelNames: []string{"bytedance-audio"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "ByteDance audio sessions", "bytedance-audio")
				harness.requireEnv(t, policy, "ByteDance audio sessions", "VOLCENGINE_OPENSPEECH_APP_ID", "VOLCENGINE_OPENSPEECH_ACCESS_TOKEN")
				completed, transcript := harness.audioSessionRoundTrip(t, ctx, "bytedance-audio", "zh_female_vv_jupiter_bigtts", "manual", "Reply with BYTEDANCE_AUDIO_OK only.")
				if completed.Usage == nil || completed.Usage.TotalTokens == 0 {
					t.Fatalf("unexpected ByteDance audio session completion %#v", completed)
				}
				if !strings.Contains(transcript, "BYTEDANCE_AUDIO_OK") {
					t.Fatalf("expected BYTEDANCE_AUDIO_OK in audio session transcript %q", transcript)
				}
			},
		},
		{
			name:       "bytedance_streaming_asr",
			modelNames: []string{"bytedance-streaming-asr"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "ByteDance streaming ASR", "bytedance-streaming-asr")
				harness.requireEnv(t, policy, "ByteDance streaming ASR", "VOLCENGINE_OPENSPEECH_APP_ID", "VOLCENGINE_OPENSPEECH_ACCESS_TOKEN", "VOLCENGINE_OPENSPEECH_API_KEY")
				transcript := harness.streamingTranscriptionRoundTrip(t, ctx, "bytedance-streaming-asr", "bytedance-tts", "zh_female_vv_uranus_bigtts", "Polaris streaming transcription passed.")
				if strings.TrimSpace(transcript) == "" {
					t.Fatalf("expected non-empty ByteDance streaming transcript")
				}
				if !strings.Contains(strings.ToLower(transcript), "polaris") {
					t.Fatalf("expected transcript to mention Polaris, got %q", transcript)
				}
			},
		},
		{
			name:       "bytedance_interpreting",
			modelNames: []string{"bytedance-interpreting"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "ByteDance simultaneous interpretation", "bytedance-interpreting")
				harness.requireEnv(t, policy, "ByteDance simultaneous interpretation", "VOLCENGINE_OPENSPEECH_APP_ID", "VOLCENGINE_OPENSPEECH_ACCESS_TOKEN", "VOLCENGINE_OPENSPEECH_API_KEY")
				translated := harness.interpretingRoundTrip(t, ctx, "bytedance-interpreting", "bytedance-tts", "zh_female_vv_uranus_bigtts", "Polaris simultaneous interpretation smoke passed.")
				if strings.TrimSpace(translated) == "" {
					t.Fatalf("expected non-empty interpreting output")
				}
				if strings.EqualFold(strings.TrimSpace(translated), "Polaris simultaneous interpretation smoke passed.") {
					t.Fatalf("expected translated interpreting output, got %q", translated)
				}
			},
		},
		{
			name:       "bytedance_translation",
			modelNames: []string{"bytedance-translation"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "ByteDance translation", "bytedance-translation")
				harness.requireEnv(t, policy, "ByteDance translation", "VOLCENGINE_OPENSPEECH_API_KEY")
				resp, err := harness.client.CreateTranslation(ctx, &client.TranslationRequest{
					Model:          "bytedance-translation",
					Input:          client.NewSingleTranslationInput("Hello, Polaris translation smoke passed."),
					TargetLanguage: "es",
				})
				if err != nil {
					t.Fatalf("ByteDance translation smoke failed: %v", err)
				}
				if len(resp.Translations) != 1 || strings.TrimSpace(resp.Translations[0].Text) == "" {
					t.Fatalf("unexpected ByteDance translation response %#v", resp)
				}
				if strings.EqualFold(strings.TrimSpace(resp.Translations[0].Text), "Hello, Polaris translation smoke passed.") {
					t.Fatalf("expected translated output, got %#v", resp)
				}
			},
		},
		{
			name:       "bytedance_voices",
			modelNames: []string{"bytedance-tts"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "ByteDance voice catalog", "bytedance-tts")
				harness.requireEnv(t, policy, "ByteDance voice catalog", "VOLCENGINE_ACCESS_KEY_ID", "VOLCENGINE_SECRET_ACCESS_KEY")
				resp, err := harness.client.ListVoices(ctx, &client.VoiceListRequest{
					Provider: "bytedance",
					Scope:    "provider",
					Limit:    3,
				})
				if err != nil {
					t.Fatalf("ByteDance voice catalog smoke failed: %v", err)
				}
				if resp.Scope != "provider" || resp.Provider != "bytedance" || len(resp.Data) == 0 {
					t.Fatalf("unexpected ByteDance voice catalog response %#v", resp)
				}
				if strings.TrimSpace(resp.Data[0].ID) == "" {
					t.Fatalf("expected non-empty ByteDance voice id, got %#v", resp.Data[0])
				}
			},
		},
		{
			name:       "bytedance_notes",
			modelNames: []string{"bytedance-notes"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "ByteDance audio notes", "bytedance-notes")
				harness.requireEnv(t, policy, "ByteDance audio notes", "VOLCENGINE_OPENSPEECH_APP_ID", "VOLCENGINE_OPENSPEECH_ACCESS_TOKEN")
				job, err := harness.client.CreateAudioNote(ctx, &client.AudioNoteRequest{
					Model:              "bytedance-notes",
					SourceURL:          liveSmokeSpeechSampleURL,
					FileType:           "wav",
					Language:           "en",
					IncludeSummary:     true,
					IncludeChapters:    true,
					IncludeActionItems: true,
					IncludeQAPairs:     true,
					TargetLanguage:     "zh",
				})
				if err != nil {
					t.Fatalf("ByteDance audio notes smoke failed: %v", err)
				}
				if strings.TrimSpace(job.ID) == "" {
					t.Fatalf("expected ByteDance audio note id, got %#v", job)
				}
				result := harness.waitForAudioNote(t, ctx, job.ID, 6*time.Minute)
				if strings.TrimSpace(result.Transcript) == "" {
					t.Fatalf("expected non-empty ByteDance audio note transcript, got %#v", result)
				}
			},
		},
		{
			name:       "bytedance_podcast",
			modelNames: []string{"bytedance-podcast"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "ByteDance podcast generation", "bytedance-podcast")
				harness.requireEnv(t, policy, "ByteDance podcast generation", "VOLCENGINE_OPENSPEECH_APP_ID", "VOLCENGINE_OPENSPEECH_ACCESS_TOKEN")
				job, err := harness.client.CreatePodcast(ctx, &client.PodcastRequest{
					Model: "bytedance-podcast",
					Segments: []client.PodcastSegment{
						{
							Speaker: "host",
							Voice:   "zh_female_vv_uranus_bigtts",
							Text:    "Welcome to the Polaris podcast smoke test.",
						},
						{
							Speaker: "guest",
							Voice:   "zh_male_m191_uranus_bigtts",
							Text:    "This short dialogue proves the podcast generation path is working.",
						},
					},
					OutputFormat: "mp3",
					SampleRateHz: 24000,
				})
				if err != nil {
					t.Fatalf("ByteDance podcast smoke failed: %v", err)
				}
				if strings.TrimSpace(job.ID) == "" {
					t.Fatalf("expected ByteDance podcast id, got %#v", job)
				}
				status, asset := harness.waitForPodcastAsset(t, ctx, job.ID, 2*time.Minute)
				if status == nil || status.Status != "completed" || asset == nil || len(asset.Data) == 0 {
					t.Fatalf("unexpected ByteDance podcast completion status=%#v asset=%#v", status, asset)
				}
			},
		},
	}
}
