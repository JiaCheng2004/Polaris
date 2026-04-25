package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/JiaCheng2004/Polaris/pkg/client"
)

func liveSmokeCoreCases() []liveSmokeCase {
	return []liveSmokeCase{
		{
			name:       "openai_chat",
			modelNames: []string{"openai-chat"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "OpenAI chat", "openai-chat")
				harness.requireEnv(t, policy, "OpenAI chat", "OPENAI_API_KEY")
				resp, err := harness.chat(ctx, "openai-chat", "Reply with OPENAI_OK only.")
				if err != nil {
					t.Fatalf("OpenAI chat smoke failed: %v", err)
				}
				harness.requireContains(t, resp, "OPENAI_OK")
			},
		},
		{
			name:       "openai_embeddings",
			modelNames: []string{"openai-embed"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "OpenAI embeddings", "openai-embed")
				harness.requireEnv(t, policy, "OpenAI embeddings", "OPENAI_API_KEY")
				resp, err := harness.client.CreateEmbedding(ctx, &client.EmbeddingRequest{
					Model: "openai-embed",
					Input: client.NewSingleEmbeddingInput("polaris openai embedding smoke"),
				})
				if err != nil {
					t.Fatalf("OpenAI embeddings smoke failed: %v", err)
				}
				if len(resp.Data) != 1 || len(resp.Data[0].Embedding.Float32) == 0 {
					t.Fatalf("unexpected OpenAI embedding response %#v", resp)
				}
			},
		},
		{
			name:       "openai_image",
			modelNames: []string{"openai-image"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "OpenAI image generation", "openai-image")
				harness.requireEnv(t, policy, "OpenAI image generation", "OPENAI_API_KEY")
				resp, err := harness.client.GenerateImage(ctx, &client.ImageGenerationRequest{
					Model:          "openai-image",
					Prompt:         "A tiny blue square on a white background",
					Size:           "1024x1024",
					ResponseFormat: "url",
				})
				if err != nil {
					t.Fatalf("OpenAI image smoke failed: %v", err)
				}
				if len(resp.Data) != 1 || strings.TrimSpace(firstImageValue(resp.Data[0])) == "" {
					t.Fatalf("unexpected OpenAI image response %#v", resp)
				}
			},
		},
		{
			name:       "openai_voice",
			modelNames: []string{"openai-tts", "openai-stt"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "OpenAI voice", "openai-tts", "openai-stt")
				harness.requireEnv(t, policy, "OpenAI voice", "OPENAI_API_KEY")
				audio, transcript := harness.voiceRoundTrip(t, ctx, "openai-tts", "openai-stt", "nova", "Polaris openai voice smoke.", "wav")
				if len(audio.Data) == 0 || strings.TrimSpace(transcript.Text) == "" {
					t.Fatalf("unexpected OpenAI voice round-trip audio=%#v transcript=%#v", audio, transcript)
				}
			},
		},
		{
			name:       "openai_video",
			modelNames: []string{"openai-video"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "OpenAI video", "openai-video")
				harness.requireEnv(t, policy, "OpenAI video", "OPENAI_API_KEY")
				job, err := harness.client.CreateVideoGeneration(ctx, &client.VideoGenerationRequest{
					Model:       "openai-video",
					Prompt:      "A short clip of ocean waves at sunrise",
					Duration:    4,
					AspectRatio: "16:9",
					Resolution:  "720p",
				})
				if err != nil {
					t.Fatalf("OpenAI video submit failed: %v", err)
				}
				if strings.TrimSpace(job.JobID) == "" {
					t.Fatalf("expected OpenAI video job id, got %#v", job)
				}
				status, err := harness.client.GetVideoGeneration(ctx, job.JobID)
				if err != nil {
					t.Fatalf("OpenAI video status failed: %v", err)
				}
				if strings.TrimSpace(status.Status) == "" {
					t.Fatalf("expected OpenAI video status, got %#v", status)
				}
			},
		},
		{
			name:       "openai_audio_session",
			modelNames: []string{"openai-audio"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "OpenAI audio sessions", "openai-audio")
				harness.requireEnv(t, policy, "OpenAI audio sessions", "OPENAI_API_KEY")
				completed, transcript := harness.audioSessionRoundTrip(t, ctx, "openai-audio", "marin", "manual", "Reply with OPENAI_AUDIO_OK only.")
				if completed.Usage == nil || completed.Usage.TotalTokens == 0 {
					t.Fatalf("unexpected OpenAI audio session completion %#v", completed)
				}
				if !strings.Contains(transcript, "OPENAI_AUDIO_OK") {
					t.Fatalf("expected OPENAI_AUDIO_OK in audio session transcript %q", transcript)
				}
			},
		},
		{
			name:       "anthropic_chat",
			modelNames: []string{"anthropic-chat"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "Anthropic chat", "anthropic-chat")
				harness.requireEnv(t, policy, "Anthropic chat", "ANTHROPIC_API_KEY")
				resp, err := harness.chat(ctx, "anthropic-chat", "Reply with ANTHROPIC_OK only.")
				if err != nil {
					t.Fatalf("Anthropic chat smoke failed: %v", err)
				}
				harness.requireContains(t, resp, "ANTHROPIC_OK")
			},
		},
		{
			name:       "google_chat",
			modelNames: []string{"google-chat"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "Google chat", "google-chat")
				harness.requireEnv(t, policy, "Google chat", "GOOGLE_API_KEY")
				resp, err := harness.chat(ctx, "google-chat", "Reply with GOOGLE_OK only.")
				if err != nil {
					t.Fatalf("Google chat smoke failed: %v", err)
				}
				harness.requireContains(t, resp, "GOOGLE_OK")
			},
		},
		{
			name:       "google_embeddings",
			modelNames: []string{"google-embed"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "Google embeddings", "google-embed")
				harness.requireEnv(t, policy, "Google embeddings", "GOOGLE_API_KEY")
				resp, err := harness.client.CreateEmbedding(ctx, &client.EmbeddingRequest{
					Model: "google-embed",
					Input: client.NewSingleEmbeddingInput("polaris google embedding smoke"),
				})
				if err != nil {
					t.Fatalf("Google embeddings smoke failed: %v", err)
				}
				if len(resp.Data) != 1 || len(resp.Data[0].Embedding.Float32) == 0 {
					t.Fatalf("unexpected Google embedding response %#v", resp)
				}
			},
		},
		{
			name:       "google_image",
			modelNames: []string{"google-image"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "Google image generation", "google-image")
				harness.requireEnv(t, policy, "Google image generation", "GOOGLE_API_KEY")
				resp, err := harness.client.GenerateImage(ctx, &client.ImageGenerationRequest{
					Model:          "google-image",
					Prompt:         "A tiny green square on a white background",
					ResponseFormat: "url",
				})
				if err != nil {
					t.Fatalf("Google image smoke failed: %v", err)
				}
				if len(resp.Data) != 1 || strings.TrimSpace(firstImageValue(resp.Data[0])) == "" {
					t.Fatalf("unexpected Google image response %#v", resp)
				}
			},
		},
		{
			name:       "google_vertex_video",
			modelNames: []string{"google-video"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "Google Vertex Veo", "google-video")
				harness.requireEnv(t, policy, "Google Vertex Veo", "GOOGLE_CLOUD_PROJECT", "GOOGLE_CLOUD_LOCATION", "GOOGLE_VERTEX_JOB_SECRET")
				harness.requireGoogleADC(t, policy)
				job, err := harness.client.CreateVideoGeneration(ctx, &client.VideoGenerationRequest{
					Model:       "google-video",
					Prompt:      "A short clip of clouds drifting over mountains",
					Duration:    4,
					AspectRatio: "16:9",
					Resolution:  "720p",
				})
				if err != nil {
					t.Fatalf("Google Vertex video submit failed: %v", err)
				}
				if strings.TrimSpace(job.JobID) == "" {
					t.Fatalf("expected Google Vertex video job id, got %#v", job)
				}
				status, err := harness.client.GetVideoGeneration(ctx, job.JobID)
				if err != nil {
					t.Fatalf("Google Vertex video status failed: %v", err)
				}
				if strings.TrimSpace(status.Status) == "" {
					t.Fatalf("expected Google Vertex video status, got %#v", status)
				}
			},
		},
		{
			name:       "deepseek_chat",
			modelNames: []string{"deepseek-chat"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "DeepSeek chat", "deepseek-chat")
				harness.requireEnv(t, policy, "DeepSeek chat", "DEEPSEEK_API_KEY")
				resp, err := harness.chat(ctx, "deepseek-chat", "Reply with DEEPSEEK_OK only.")
				if err != nil {
					t.Fatalf("DeepSeek chat smoke failed: %v", err)
				}
				harness.requireContains(t, resp, "DEEPSEEK_OK")
			},
		},
	}
}

func liveSmokeUsageCase() liveSmokeCase {
	return liveSmokeCase{
		name: "usage",
		run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
			report, err := harness.client.GetUsage(ctx, &client.UsageParams{GroupBy: "model"})
			if err != nil {
				t.Fatalf("GetUsage() error = %v", err)
			}
			if report.TotalRequests == 0 {
				t.Fatalf("expected usage rows after live smoke, got %#v", report)
			}
		},
	}
}
