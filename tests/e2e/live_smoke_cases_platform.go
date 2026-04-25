package e2e

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/pkg/client"
)

func liveSmokePlatformCases() []liveSmokeCase {
	return []liveSmokeCase{
		{
			name:       "minimax_music_lyrics",
			modelNames: []string{"minimax-music"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "MiniMax music lyrics", "minimax-music")
				harness.requireEnv(t, policy, "MiniMax music lyrics", "MINIMAX_API_KEY")
				resp, err := harness.client.CreateMusicLyrics(ctx, &client.MusicLyricsRequest{
					Model:  "minimax-music",
					Prompt: "Write a two-line synth-pop chorus about city lights.",
				})
				if err != nil {
					t.Fatalf("MiniMax lyrics smoke failed: %v", err)
				}
				if strings.TrimSpace(resp.Lyrics) == "" {
					t.Fatalf("unexpected MiniMax lyrics response %#v", resp)
				}
			},
		},
		{
			name:       "minimax_music_async_generation",
			modelNames: []string{"minimax-music"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "MiniMax music generation", "minimax-music")
				harness.requireEnv(t, policy, "MiniMax music generation", "MINIMAX_API_KEY")
				resp, err := harness.client.CreateMusicGeneration(ctx, &client.MusicGenerationRequest{
					Model:        "minimax-music",
					Mode:         "async",
					Prompt:       "Create a short upbeat synth-pop instrumental about driving through neon city lights at midnight.",
					DurationMS:   10000,
					Instrumental: true,
					OutputFormat: "mp3",
				})
				if err != nil {
					t.Fatalf("MiniMax async generation smoke failed: %v", err)
				}
				if resp.Job == nil || strings.TrimSpace(resp.Job.JobID) == "" {
					t.Fatalf("expected MiniMax music job, got %#v", resp)
				}
				status, asset := harness.waitForMusicJobAsset(t, ctx, resp.Job.JobID, 8*time.Minute)
				if status == nil || status.Status != "completed" || asset == nil || len(asset.Data) == 0 {
					t.Fatalf("unexpected MiniMax music completion status=%#v asset=%#v", status, asset)
				}
			},
		},
		{
			name:       "elevenlabs_music_generation",
			modelNames: []string{"elevenlabs-music"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "ElevenLabs music generation", "elevenlabs-music")
				harness.requireEnv(t, policy, "ElevenLabs music generation", "ELEVENLABS_API_KEY")
				resp, err := harness.client.CreateMusicGeneration(ctx, &client.MusicGenerationRequest{
					Model:        "elevenlabs-music",
					Prompt:       "Create a short cinematic electronic theme with a clear pulse.",
					DurationMS:   10000,
					Instrumental: true,
					OutputFormat: "mp3",
				})
				if err != nil {
					t.Fatalf("ElevenLabs generation smoke failed: %v", err)
				}
				if resp.Asset == nil || len(resp.Asset.Data) == 0 {
					t.Fatalf("unexpected ElevenLabs music response %#v", resp)
				}
			},
		},
		{
			name:       "elevenlabs_music_stream",
			modelNames: []string{"elevenlabs-music"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "ElevenLabs music stream", "elevenlabs-music")
				harness.requireEnv(t, policy, "ElevenLabs music stream", "ELEVENLABS_API_KEY")
				stream, err := harness.client.StreamMusicGeneration(ctx, &client.MusicGenerationRequest{
					Model:        "elevenlabs-music",
					Prompt:       "Create a short ambient electronic build with soft pads.",
					DurationMS:   10000,
					Instrumental: true,
					OutputFormat: "mp3",
				})
				if err != nil {
					t.Fatalf("ElevenLabs stream smoke failed: %v", err)
				}
				defer func() {
					_ = stream.Body.Close()
				}()
				payload, err := io.ReadAll(stream.Body)
				if err != nil {
					t.Fatalf("read ElevenLabs stream: %v", err)
				}
				if len(payload) == 0 {
					t.Fatalf("expected streamed ElevenLabs music bytes")
				}
			},
		},
		{
			name:       "elevenlabs_music_plan",
			modelNames: []string{"elevenlabs-music"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "ElevenLabs music plans", "elevenlabs-music")
				harness.requireEnv(t, policy, "ElevenLabs music plans", "ELEVENLABS_API_KEY")
				resp, err := harness.client.CreateMusicPlan(ctx, &client.MusicPlanRequest{
					Model:      "elevenlabs-music",
					Prompt:     "Plan a 10-second electronic intro that grows into a beat drop.",
					DurationMS: 10000,
				})
				if err != nil {
					t.Fatalf("ElevenLabs plan smoke failed: %v", err)
				}
				if len(resp.Plan) == 0 {
					t.Fatalf("unexpected ElevenLabs plan response %#v", resp)
				}
			},
		},
		{
			name:       "elevenlabs_music_stems",
			modelNames: []string{"elevenlabs-music"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "ElevenLabs music stems", "elevenlabs-music")
				harness.requireEnv(t, policy, "ElevenLabs music stems", "ELEVENLABS_API_KEY")
				generated, err := harness.client.CreateMusicGeneration(ctx, &client.MusicGenerationRequest{
					Model:        "elevenlabs-music",
					Prompt:       "Create a short layered beat with drums, bass, and synth.",
					DurationMS:   10000,
					Instrumental: true,
					OutputFormat: "mp3",
				})
				if err != nil {
					t.Fatalf("ElevenLabs stems smoke setup failed: %v", err)
				}
				if generated.Asset == nil || len(generated.Asset.Data) == 0 {
					t.Fatalf("unexpected ElevenLabs setup response %#v", generated)
				}
				stems, err := harness.client.SeparateMusicStems(ctx, &client.MusicStemRequest{
					Model:       "stems-music",
					File:        generated.Asset.Data,
					Filename:    "music.mp3",
					ContentType: generated.Asset.ContentType,
				})
				if err != nil {
					t.Fatalf("ElevenLabs stems smoke failed: %v", err)
				}
				if stems.Asset == nil || len(stems.Asset.Data) == 0 {
					t.Fatalf("unexpected ElevenLabs stems response %#v", stems)
				}
			},
		},
		{
			name:       "xai_chat",
			modelNames: []string{"xai-chat"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "xAI chat", "xai-chat")
				harness.requireEnv(t, policy, "xAI chat", "XAI_API_KEY")
				resp, err := harness.chat(ctx, "xai-chat", "Reply with XAI_OK only.")
				if err != nil {
					t.Fatalf("xAI chat smoke failed: %v", err)
				}
				harness.requireContains(t, resp, "XAI_OK")
			},
		},
		{
			name:       "openrouter_chat",
			modelNames: []string{"openrouter-chat"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "OpenRouter chat", "openrouter-chat")
				harness.requireEnv(t, policy, "OpenRouter chat", "OPENROUTER_API_KEY")
				resp, err := harness.chat(ctx, "openrouter-chat", "Reply with OPENROUTER_OK only.")
				if err != nil {
					t.Fatalf("OpenRouter chat smoke failed: %v", err)
				}
				harness.requireContains(t, resp, "OPENROUTER_OK")
			},
		},
		{
			name:       "together_chat",
			modelNames: []string{"together-chat"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "Together chat", "together-chat")
				harness.requireEnv(t, policy, "Together chat", "TOGETHER_API_KEY")
				resp, err := harness.chat(ctx, "together-chat", "Reply with TOGETHER_OK only.")
				if err != nil {
					t.Fatalf("Together chat smoke failed: %v", err)
				}
				harness.requireContains(t, resp, "TOGETHER_OK")
			},
		},
		{
			name:       "groq_chat",
			modelNames: []string{"groq-chat"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "Groq chat", "groq-chat")
				harness.requireEnv(t, policy, "Groq chat", "GROQ_API_KEY")
				resp, err := harness.chat(ctx, "groq-chat", "Reply with GROQ_OK only.")
				if err != nil {
					t.Fatalf("Groq chat smoke failed: %v", err)
				}
				harness.requireContains(t, resp, "GROQ_OK")
			},
		},
		{
			name:       "fireworks_chat",
			modelNames: []string{"fireworks-chat"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "Fireworks chat", "fireworks-chat")
				harness.requireEnv(t, policy, "Fireworks chat", "FIREWORKS_API_KEY")
				resp, err := harness.chat(ctx, "fireworks-chat", "Reply with FIREWORKS_OK only.")
				if err != nil {
					t.Fatalf("Fireworks chat smoke failed: %v", err)
				}
				harness.requireContains(t, resp, "FIREWORKS_OK")
			},
		},
		{
			name:       "featherless_chat",
			modelNames: []string{"featherless-chat"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "Featherless chat", "featherless-chat")
				harness.requireEnv(t, policy, "Featherless chat", "FEATHERLESS_API_KEY")
				resp, err := harness.chat(ctx, "featherless-chat", "Reply with FEATHERLESS_OK only.")
				if err != nil {
					t.Fatalf("Featherless chat smoke failed: %v", err)
				}
				harness.requireContains(t, resp, "FEATHERLESS_OK")
			},
		},
		{
			name:       "moonshot_chat",
			modelNames: []string{"moonshot-chat"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "Moonshot chat", "moonshot-chat")
				harness.requireEnv(t, policy, "Moonshot chat", "MOONSHOT_API_KEY")
				resp, err := harness.chat(ctx, "moonshot-chat", "Reply with MOONSHOT_OK only.")
				if err != nil {
					t.Fatalf("Moonshot chat smoke failed: %v", err)
				}
				harness.requireContains(t, resp, "MOONSHOT_OK")
			},
		},
		{
			name:       "glm_chat",
			modelNames: []string{"glm-chat"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "GLM chat", "glm-chat")
				harness.requireEnv(t, policy, "GLM chat", "GLM_API_KEY")
				resp, err := harness.chat(ctx, "glm-chat", "Reply with GLM_OK only.")
				if err != nil {
					t.Fatalf("GLM chat smoke failed: %v", err)
				}
				harness.requireContains(t, resp, "GLM_OK")
			},
		},
		{
			name:       "mistral_chat",
			modelNames: []string{"mistral-chat"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "Mistral chat", "mistral-chat")
				harness.requireEnv(t, policy, "Mistral chat", "MISTRAL_API_KEY")
				resp, err := harness.chat(ctx, "mistral-chat", "Reply with MISTRAL_OK only.")
				if err != nil {
					t.Fatalf("Mistral chat smoke failed: %v", err)
				}
				harness.requireContains(t, resp, "MISTRAL_OK")
			},
		},
		{
			name:       "bedrock_chat",
			modelNames: []string{"bedrock-chat"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "Amazon Bedrock chat", "bedrock-chat")
				harness.requireEnv(t, policy, "Amazon Bedrock chat", "AWS_BEDROCK_ACCESS_KEY_ID", "AWS_BEDROCK_SECRET_ACCESS_KEY", "AWS_BEDROCK_REGION")
				resp, err := harness.chat(ctx, "bedrock-chat", "Reply with BEDROCK_OK only.")
				if err != nil {
					t.Fatalf("Amazon Bedrock chat smoke failed: %v", err)
				}
				harness.requireContains(t, resp, "BEDROCK_OK")
			},
		},
		{
			name:       "nvidia_chat",
			modelNames: []string{"nvidia-chat"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "NVIDIA chat", "nvidia-chat")
				harness.requireEnv(t, policy, "NVIDIA chat", "NVIDIA_API_KEY")
				resp, err := harness.chat(ctx, "nvidia-chat", "Reply with NVIDIA_OK only.")
				if err != nil {
					t.Fatalf("NVIDIA chat smoke failed: %v", err)
				}
				harness.requireContains(t, resp, "NVIDIA_OK")
			},
		},
		{
			name:       "replicate_video",
			modelNames: []string{"replicate-video"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "Replicate video", "replicate-video")
				harness.requireEnv(t, policy, "Replicate video", "REPLICATE_API_KEY")
				job, err := harness.client.CreateVideoGeneration(ctx, &client.VideoGenerationRequest{
					Model:       "replicate-video",
					Prompt:      "A short clip of a paper airplane gliding across a blue sky",
					Duration:    6,
					AspectRatio: "16:9",
					Resolution:  "720p",
				})
				if err != nil {
					t.Fatalf("Replicate video submit failed: %v", err)
				}
				if strings.TrimSpace(job.JobID) == "" {
					t.Fatalf("expected Replicate video job id, got %#v", job)
				}
				status, err := harness.client.GetVideoGeneration(ctx, job.JobID)
				if err != nil {
					t.Fatalf("Replicate video status failed: %v", err)
				}
				if strings.TrimSpace(status.Status) == "" {
					t.Fatalf("expected Replicate video status, got %#v", status)
				}
			},
		},
		{
			name:       "qwen_chat",
			modelNames: []string{"qwen-chat"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "Qwen chat", "qwen-chat")
				harness.requireEnv(t, policy, "Qwen chat", "DASHSCOPE_API_KEY")
				resp, err := harness.chat(ctx, "qwen-chat", "Reply with QWEN_OK only.")
				if err != nil {
					t.Fatalf("Qwen chat smoke failed: %v", err)
				}
				harness.requireContains(t, resp, "QWEN_OK")
			},
		},
		{
			name:       "qwen_image",
			modelNames: []string{"qwen-image"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "Qwen image generation", "qwen-image")
				harness.requireEnv(t, policy, "Qwen image generation", "DASHSCOPE_API_KEY")
				resp, err := harness.client.GenerateImage(ctx, &client.ImageGenerationRequest{
					Model:          "qwen-image",
					Prompt:         "A tiny yellow square on a white background",
					ResponseFormat: "url",
				})
				if err != nil {
					t.Fatalf("Qwen image smoke failed: %v", err)
				}
				if len(resp.Data) != 1 || strings.TrimSpace(firstImageValue(resp.Data[0])) == "" {
					t.Fatalf("unexpected Qwen image response %#v", resp)
				}
			},
		},
		{
			name:       "ollama_chat",
			modelNames: []string{"ollama-chat"},
			run: func(t *testing.T, ctx context.Context, harness *liveSmokeHarness) {
				policy := harness.gateCase(t, "Ollama chat", "ollama-chat")
				harness.requireOllama(t, policy)
				resp, err := harness.chat(ctx, "ollama-chat", "Reply with OLLAMA_OK only.")
				if err != nil {
					t.Fatalf("Ollama chat smoke failed: %v", err)
				}
				harness.requireContains(t, resp, "OLLAMA_OK")
			},
		},
	}
}
