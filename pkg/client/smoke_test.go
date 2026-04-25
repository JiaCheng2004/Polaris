package client

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway"
	gwmiddleware "github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/JiaCheng2004/Polaris/internal/store/sqlite"
)

func TestSDKSmokeAgainstPolarisEngine(t *testing.T) {
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id":"chatcmpl-1",
				"object":"chat.completion",
				"created":1744329600,
				"model":"gpt-4o",
				"choices":[{"index":0,"message":{"role":"assistant","content":"Hello from Polaris"},"finish_reason":"stop"}],
				"usage":{"prompt_tokens":20,"completion_tokens":6,"total_tokens":26}
			}`))
		case "/v1/embeddings":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"object":"list",
				"data":[{"object":"embedding","index":0,"embedding":[0.1,0.2]}],
				"model":"text-embedding-3-small",
				"usage":{"prompt_tokens":8,"total_tokens":8}
			}`))
		case "/v1/images/generations":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"created":1744329600,
				"data":[{"url":"https://example.com/generated.png"}]
			}`))
		case "/v1/images/edits":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"created":1744329601,
				"data":[{"b64_json":"AQID"}]
			}`))
		case "/v1/contents/generations/tasks":
			switch r.Method {
			case http.MethodPost:
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"id":"task_123",
					"status":"queued",
					"estimated_time":12
				}`))
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"job_id":"vid_123","status":"completed","progress":1}`))
			case http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			default:
				t.Fatalf("unexpected upstream method %s for %s", r.Method, r.URL.Path)
			}
		case "/v1/contents/generations/tasks/task_123":
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"id":"task_123",
					"status":"completed",
					"progress":1,
					"content":{"video_url":"` + upstream.URL + `/video.mp4","duration":8,"mime_type":"video/mp4"}
				}`))
			case http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			default:
				t.Fatalf("unexpected upstream method %s for %s", r.Method, r.URL.Path)
			}
		case "/video.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video-bytes"))
		case "/v1/audio/speech":
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("ID3"))
		case "/v1/audio/transcriptions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"text":"Hello from speech",
				"language":"en",
				"duration":1.25,
				"segments":[{"id":0,"start":0.0,"end":1.25,"text":"Hello from speech"}]
			}`))
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:            "127.0.0.1",
			Port:            8080,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			ShutdownTimeout: 5 * time.Second,
		},
		Auth: config.AuthConfig{
			Mode: config.AuthModeMultiUser,
		},
		Cache: config.CacheConfig{
			Driver: "memory",
			RateLimit: config.RateLimitConfig{
				Enabled: true,
				Default: "20/min",
				Window:  "sliding",
			},
		},
		Providers: map[string]config.ProviderConfig{
			"openai": {
				APIKey:  "sk-openai",
				BaseURL: upstream.URL + "/v1",
				Timeout: time.Second,
				Models: map[string]config.ModelConfig{
					"gpt-4o": {
						Modality:     modality.ModalityChat,
						Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityJSONMode},
					},
					"text-embedding-3-small": {
						Modality:   modality.ModalityEmbed,
						Dimensions: 1536,
					},
					"gpt-image-1": {
						Modality:     modality.ModalityImage,
						Capabilities: []modality.Capability{modality.CapabilityGeneration, modality.CapabilityEditing},
					},
					"tts-1": {
						Modality:     modality.ModalityVoice,
						Capabilities: []modality.Capability{modality.CapabilityTTS},
						Voices:       []string{"nova"},
					},
					"whisper-1": {
						Modality:     modality.ModalityVoice,
						Capabilities: []modality.Capability{modality.CapabilitySTT},
						Formats:      []string{"wav"},
					},
				},
			},
			"bytedance": {
				APIKey:  "ark-bytedance",
				BaseURL: upstream.URL + "/v1",
				Timeout: time.Second,
				Models: map[string]config.ModelConfig{
					"seedance-2.0": {
						Modality:         modality.ModalityVideo,
						Capabilities:     []modality.Capability{modality.CapabilityTextToVideo, modality.CapabilityImageToVideo, modality.CapabilityLastFrame, modality.CapabilityReferenceImages, modality.CapabilityVideoInput, modality.CapabilityAudioInput, modality.CapabilityNativeAudio},
						MaxDuration:      15,
						AllowedDurations: []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
						Resolutions:      []string{"720p"},
						Cancelable:       true,
						Endpoint:         upstream.URL + "/v1",
					},
				},
			},
		},
		Routing: config.RoutingConfig{
			Aliases: map[string]string{
				"default-chat":      "openai/gpt-4o",
				"default-embed":     "openai/text-embedding-3-small",
				"default-image":     "openai/gpt-image-1",
				"default-video":     "bytedance/seedance-2.0",
				"default-voice-tts": "openai/tts-1",
				"default-voice-stt": "openai/whisper-1",
			},
		},
		Observability: config.ObservabilityConfig{
			Metrics: config.MetricsConfig{
				Enabled: true,
				Path:    "/metrics",
			},
			Logging: config.LoggingConfig{
				Level:  "error",
				Format: "json",
			},
		},
	}

	sqliteStore, err := sqlite.New(config.StoreConfig{
		Driver:           "sqlite",
		DSN:              filepath.Join(t.TempDir(), "polaris.db"),
		MaxConnections:   1,
		LogRetentionDays: 30,
		LogBufferSize:    10,
		LogFlushInterval: time.Second,
	})
	if err != nil {
		t.Fatalf("sqlite.New() error = %v", err)
	}
	defer func() {
		_ = sqliteStore.Close()
	}()
	if err := sqliteStore.Migrate(context.Background()); err != nil {
		t.Fatalf("sqliteStore.Migrate() error = %v", err)
	}

	adminRawKey := "polaris-admin-secret"
	if err := sqliteStore.CreateAPIKey(context.Background(), store.APIKey{
		ID:            "key_admin",
		Name:          "admin",
		KeyHash:       gwmiddleware.HashAPIKey(adminRawKey),
		KeyPrefix:     "polaris-",
		AllowedModels: []string{"*"},
		IsAdmin:       true,
	}); err != nil {
		t.Fatalf("CreateAPIKey(admin) error = %v", err)
	}

	registry, warnings, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no registry warnings, got %v", warnings)
	}

	requestLogger := store.NewAsyncRequestLogger(sqliteStore, slog.New(slog.NewTextHandler(io.Discard, nil)), store.NewLoggerConfig(10, 5*time.Millisecond))
	engine, err := gateway.NewEngine(gateway.Dependencies{
		Config:        cfg,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:         sqliteStore,
		Cache:         cache.NewMemory(),
		Registry:      registry,
		RequestLogger: requestLogger,
	})
	if err != nil {
		t.Fatalf("gateway.NewEngine() error = %v", err)
	}

	server := httptest.NewServer(engine)
	defer server.Close()

	adminClient := newTestClient(t, server.URL, WithAPIKey(adminRawKey))
	created, err := adminClient.CreateKey(context.Background(), &CreateKeyRequest{
		Name:          "sdk-smoke",
		OwnerID:       "smoke-user",
		AllowedModels: []string{"*"},
	})
	if err != nil {
		t.Fatalf("CreateKey() error = %v", err)
	}
	if created.Key == "" {
		t.Fatalf("expected plaintext key on create, got %#v", created)
	}

	userClient := newTestClient(t, server.URL, WithAPIKey(created.Key))
	models, err := userClient.ListModels(context.Background(), true)
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models.Data) < 7 {
		t.Fatalf("expected registered models plus aliases, got %#v", models)
	}

	chatResponse, err := userClient.CreateChatCompletion(context.Background(), &ChatCompletionRequest{
		Model:    "default-chat",
		Messages: []ChatMessage{{Role: "user", Content: NewTextContent("Hello")}},
	})
	if err != nil {
		t.Fatalf("CreateChatCompletion() error = %v", err)
	}
	if chatResponse.Model != "openai/gpt-4o" {
		t.Fatalf("unexpected chat response %#v", chatResponse)
	}

	embedResponse, err := userClient.CreateEmbedding(context.Background(), &EmbeddingRequest{
		Model: "default-embed",
		Input: NewSingleEmbeddingInput("hello"),
	})
	if err != nil {
		t.Fatalf("CreateEmbedding() error = %v", err)
	}
	if embedResponse.Model != "openai/text-embedding-3-small" {
		t.Fatalf("unexpected embed response %#v", embedResponse)
	}

	imageResponse, err := userClient.GenerateImage(context.Background(), &ImageGenerationRequest{
		Model:  "default-image",
		Prompt: "a lighthouse",
	})
	if err != nil {
		t.Fatalf("GenerateImage() error = %v", err)
	}
	if len(imageResponse.Data) != 1 || imageResponse.Data[0].URL == "" {
		t.Fatalf("unexpected generate image response %#v", imageResponse)
	}

	imageEditResponse, err := userClient.EditImage(context.Background(), &ImageEditRequest{
		Model:          "default-image",
		Prompt:         "make it brighter",
		Image:          []byte("png-bytes"),
		ImageFilename:  "input.png",
		ResponseFormat: "b64_json",
	})
	if err != nil {
		t.Fatalf("EditImage() error = %v", err)
	}
	if len(imageEditResponse.Data) != 1 || imageEditResponse.Data[0].B64JSON != "AQID" {
		t.Fatalf("unexpected edit image response %#v", imageEditResponse)
	}

	videoJob, err := userClient.CreateVideoGeneration(context.Background(), &VideoGenerationRequest{
		Model:      "default-video",
		Prompt:     "A cinematic skyline",
		Duration:   8,
		Resolution: "720p",
	})
	if err != nil {
		t.Fatalf("CreateVideoGeneration() error = %v", err)
	}
	if !strings.HasPrefix(videoJob.JobID, "vid_") || videoJob.Model != "bytedance/seedance-2.0" {
		t.Fatalf("unexpected video job %#v", videoJob)
	}

	videoStatus, err := userClient.GetVideoGeneration(context.Background(), videoJob.JobID)
	if err != nil {
		t.Fatalf("GetVideoGeneration() error = %v", err)
	}
	if videoStatus.Status != "completed" || videoStatus.Result == nil || videoStatus.Result.VideoURL == "" || videoStatus.Result.DownloadURL == "" {
		t.Fatalf("unexpected video status %#v", videoStatus)
	}

	videoAsset, err := userClient.GetVideoGenerationContent(context.Background(), videoJob.JobID)
	if err != nil {
		t.Fatalf("GetVideoGenerationContent() error = %v", err)
	}
	if len(videoAsset.Data) == 0 {
		t.Fatalf("unexpected empty video asset %#v", videoAsset)
	}

	if err := userClient.CancelVideoGeneration(context.Background(), videoJob.JobID); err != nil {
		t.Fatalf("CancelVideoGeneration() error = %v", err)
	}

	audioResponse, err := userClient.CreateSpeech(context.Background(), &SpeechRequest{
		Model: "default-voice-tts",
		Input: "Hello",
		Voice: "nova",
	})
	if err != nil {
		t.Fatalf("CreateSpeech() error = %v", err)
	}
	if string(audioResponse.Data) != "ID3" {
		t.Fatalf("unexpected speech response %#v", audioResponse)
	}

	transcriptionResponse, err := userClient.CreateTranscription(context.Background(), &TranscriptionRequest{
		Model:          "default-voice-stt",
		File:           []byte("wav-bytes"),
		Filename:       "sample.wav",
		ResponseFormat: "json",
	})
	if err != nil {
		t.Fatalf("CreateTranscription() error = %v", err)
	}
	if transcriptionResponse.Text != "Hello from speech" {
		t.Fatalf("unexpected transcription response %#v", transcriptionResponse)
	}

	if err := requestLogger.Close(context.Background()); err != nil {
		t.Fatalf("requestLogger.Close() error = %v", err)
	}

	usage, err := userClient.GetUsage(context.Background(), &UsageParams{GroupBy: "model"})
	if err != nil {
		t.Fatalf("GetUsage() error = %v", err)
	}
	if usage.TotalRequests != 7 || usage.TotalTokens != 34 {
		t.Fatalf("unexpected usage response %#v", usage)
	}

	keys, err := adminClient.ListKeys(context.Background(), &ListKeysParams{OwnerID: "smoke-user"})
	if err != nil {
		t.Fatalf("ListKeys() error = %v", err)
	}
	if len(keys.Data) != 1 || keys.Data[0].ID != created.ID {
		t.Fatalf("unexpected key list %#v", keys)
	}

	if err := adminClient.DeleteKey(context.Background(), created.ID); err != nil {
		t.Fatalf("DeleteKey() error = %v", err)
	}

	includeRevoked := true
	keys, err = adminClient.ListKeys(context.Background(), &ListKeysParams{
		OwnerID:        "smoke-user",
		IncludeRevoked: &includeRevoked,
	})
	if err != nil {
		t.Fatalf("ListKeys(include_revoked) error = %v", err)
	}
	if len(keys.Data) != 1 || !keys.Data[0].IsRevoked {
		t.Fatalf("expected revoked key in list, got %#v", keys)
	}
}
