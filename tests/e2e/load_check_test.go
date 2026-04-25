package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/verification"
	"github.com/JiaCheng2004/Polaris/pkg/client"
	"github.com/gorilla/websocket"
	"golang.org/x/oauth2/google"
)

const (
	loadBackendSQLiteMemory = "sqlite_memory"
	loadCacheHeader         = "X-Polaris-Cache"
)

type loadCheckHarness struct {
	*liveSmokeHarness
	httpClient *http.Client
	backend    string
	runID      string
	counts     loadCheckCounts
	report     loadCheckReport
}

type loadCheckCounts struct {
	ChatRepeats    int
	SyncRepeats    int
	BurstParallel  int
	VideoJobs      int
	AudioSessions  int
	MusicAsyncJobs int
}

type loadCheckReport struct {
	StartedAt time.Time
	Backend   string
	RunID     string
	Scenarios []loadCheckScenarioResult
}

type loadCheckScenarioResult struct {
	Name     string
	Status   string
	Duration time.Duration
	Note     string
}

type loadCheckScenario struct {
	name string
	run  func(context.Context, *loadCheckHarness) (string, error)
}

type loadCheckSkip struct {
	reason string
}

type rawLoadResponse struct {
	status int
	header http.Header
	body   []byte
}

func (e loadCheckSkip) Error() string {
	return e.reason
}

func TestLoadCheckMatrix(t *testing.T) {
	if os.Getenv("POLARIS_LOAD_CHECK") != "1" {
		t.Skip("POLARIS_LOAD_CHECK is not set")
	}

	harness := newLoadCheckHarness(t)
	t.Cleanup(func() {
		harness.writeReport(t)
	})

	ctx := context.Background()
	for _, scenario := range loadCheckScenarios() {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			start := time.Now()
			note, err := scenario.run(ctx, harness)
			status := "passed"
			if err != nil {
				var skip loadCheckSkip
				if errors.As(err, &skip) {
					status = "skipped"
					harness.recordScenario(scenario.name, status, time.Since(start), skip.reason)
					t.Skip(skip.reason)
				}
				status = "failed"
				if strings.TrimSpace(note) == "" {
					note = err.Error()
				}
			}
			harness.recordScenario(scenario.name, status, time.Since(start), note)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func newLoadCheckHarness(t *testing.T) *loadCheckHarness {
	t.Helper()

	responseCacheEnabled := true
	rateLimitEnabled := false
	opts := liveSmokeHarnessOptions{
		responseCacheEnabled: &responseCacheEnabled,
		rateLimitEnabled:     &rateLimitEnabled,
		logBufferSize:        5000,
		logFlushInterval:     25 * time.Millisecond,
		storeDriver:          "sqlite",
		cacheDriver:          "memory",
	}

	live := newLiveSmokeHarnessWithOptions(t, opts)
	live.strict = true

	return &loadCheckHarness{
		liveSmokeHarness: live,
		httpClient:       &http.Client{Timeout: 10 * time.Minute},
		backend:          loadBackendSQLiteMemory,
		runID:            "load-" + strconv.FormatInt(time.Now().UnixNano(), 36),
		counts:           loadCheckCountsFromEnv(),
		report: loadCheckReport{
			StartedAt: time.Now().UTC(),
			Backend:   loadBackendSQLiteMemory,
		},
	}
}

func loadCheckCountsFromEnv() loadCheckCounts {
	return loadCheckCounts{
		ChatRepeats:    loadCheckIntEnv("POLARIS_LOAD_CHAT_REPEATS", 20),
		SyncRepeats:    loadCheckIntEnv("POLARIS_LOAD_SYNC_REPEATS", 3),
		BurstParallel:  loadCheckIntEnv("POLARIS_LOAD_BURST_PARALLEL", 5),
		VideoJobs:      loadCheckIntEnv("POLARIS_LOAD_VIDEO_JOBS", 5),
		AudioSessions:  loadCheckIntEnv("POLARIS_LOAD_AUDIO_SESSIONS", 5),
		MusicAsyncJobs: loadCheckIntEnv("POLARIS_LOAD_MUSIC_JOBS", 3),
	}
}

func loadCheckIntEnv(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func loadCheckScenarios() []loadCheckScenario {
	return []loadCheckScenario{
		{name: "cached_sync_traffic", run: runLoadCachedSyncTraffic},
		{name: "mixed_modality_burst", run: runLoadMixedModalityBurst},
		{name: "video_async_lifecycle", run: runLoadVideoAsyncLifecycle},
		{name: "audio_session_concurrency", run: runLoadAudioSessionConcurrency},
		{name: "music_jobs_and_cache", run: runLoadMusicJobsAndCache},
		{name: "store_cache_health", run: runLoadStoreCacheHealth},
	}
}

func runLoadCachedSyncTraffic(ctx context.Context, h *loadCheckHarness) (string, error) {
	if err := h.requireModels("OpenAI cached sync traffic", "openai-chat", "openai-embed", "openai-image", "openai-tts", "openai-stt"); err != nil {
		return "", err
	}
	if err := h.requireEnv("OpenAI cached sync traffic", "OPENAI_API_KEY"); err != nil {
		return "", err
	}
	if err := h.requireModels("MiniMax cached music traffic", "minimax-music"); err != nil {
		return "", err
	}
	if err := h.requireEnv("MiniMax cached music traffic", "MINIMAX_API_KEY"); err != nil {
		return "", err
	}

	chatPayload := map[string]any{
		"model": "openai-chat",
		"messages": []map[string]any{
			{"role": "user", "content": "Reply with POLARIS_LOAD_CACHE_OK only. " + h.runID},
		},
		"temperature": 0,
	}
	if err := h.expectCacheTransition(ctx, "/v1/chat/completions", chatPayload, h.counts.ChatRepeats, "application/json"); err != nil {
		return "", fmt.Errorf("chat cache transition: %w", err)
	}

	streamPayload := map[string]any{
		"model":  "openai-chat",
		"stream": true,
		"messages": []map[string]any{
			{"role": "user", "content": "Stream one short sentence. " + h.runID},
		},
	}
	stream, err := h.rawJSON(ctx, http.MethodPost, "/v1/chat/completions", streamPayload, "text/event-stream")
	if err != nil {
		return "", fmt.Errorf("streaming chat request: %w", err)
	}
	if status := stream.header.Get(loadCacheHeader); status != "bypass" {
		return "", fmt.Errorf("streaming chat cache header = %q, want bypass", status)
	}
	if !bytes.Contains(stream.body, []byte("[DONE]")) {
		return "", fmt.Errorf("streaming chat response did not include [DONE]")
	}

	embeddingPayload := map[string]any{
		"model": "openai-embed",
		"input": "polaris load embedding " + h.runID,
	}
	if err := h.expectCacheTransition(ctx, "/v1/embeddings", embeddingPayload, h.counts.SyncRepeats, "application/json"); err != nil {
		return "", fmt.Errorf("embedding cache transition: %w", err)
	}

	imagePayload := map[string]any{
		"model":           "openai-image",
		"prompt":          "A tiny navy square on a plain white background. " + h.runID,
		"size":            "1024x1024",
		"response_format": "url",
	}
	if err := h.expectCacheTransition(ctx, "/v1/images/generations", imagePayload, h.counts.SyncRepeats, "application/json"); err != nil {
		return "", fmt.Errorf("image cache transition: %w", err)
	}

	speechPayload := map[string]any{
		"model":           "openai-tts",
		"voice":           "nova",
		"input":           "Polaris load speech " + h.runID,
		"response_format": "wav",
	}
	if err := h.expectCacheTransition(ctx, "/v1/audio/speech", speechPayload, h.counts.SyncRepeats, "*/*"); err != nil {
		return "", fmt.Errorf("speech cache transition: %w", err)
	}

	audio, err := h.client.CreateSpeech(ctx, &client.SpeechRequest{
		Model:          "openai-tts",
		Voice:          "nova",
		Input:          "Polaris load transcription " + h.runID,
		ResponseFormat: "wav",
	})
	if err != nil {
		return "", fmt.Errorf("create speech for transcription: %w", err)
	}
	for i := 0; i < h.counts.SyncRepeats; i++ {
		transcript, err := h.client.CreateTranscription(ctx, &client.TranscriptionRequest{
			Model:          "openai-stt",
			File:           audio.Data,
			Filename:       "load.wav",
			ContentType:    audio.ContentType,
			ResponseFormat: "json",
		})
		if err != nil {
			return "", fmt.Errorf("transcription repeat %d: %w", i+1, err)
		}
		if strings.TrimSpace(transcript.Text) == "" {
			return "", fmt.Errorf("transcription repeat %d returned empty text", i+1)
		}
	}

	lyricsPayload := map[string]any{
		"model":  "minimax-music",
		"prompt": "Write four short lines about a north star. " + h.runID,
	}
	if err := h.expectCacheTransition(ctx, "/v1/music/lyrics", lyricsPayload, h.counts.SyncRepeats, "application/json"); err != nil {
		return "", fmt.Errorf("music lyrics cache transition: %w", err)
	}

	generationPayload := map[string]any{
		"model":         "minimax-music",
		"mode":          "sync",
		"prompt":        "Short instrumental ambient test. " + h.runID,
		"duration_ms":   10000,
		"instrumental":  true,
		"output_format": "mp3",
	}
	if err := h.expectCacheTransition(ctx, "/v1/music/generations", generationPayload, h.counts.SyncRepeats, "*/*"); err != nil {
		return "", fmt.Errorf("music generation cache transition: %w", err)
	}

	return fmt.Sprintf("cache repeated chat=%d sync=%d", h.counts.ChatRepeats, h.counts.SyncRepeats), nil
}

func runLoadMixedModalityBurst(ctx context.Context, h *loadCheckHarness) (string, error) {
	if err := h.requireModels("OpenAI mixed modality burst", "openai-chat", "openai-embed", "openai-image", "openai-tts", "openai-stt"); err != nil {
		return "", err
	}
	if err := h.requireEnv("OpenAI mixed modality burst", "OPENAI_API_KEY"); err != nil {
		return "", err
	}

	errs := h.runParallel(ctx, h.counts.BurstParallel, func(ctx context.Context, index int) error {
		switch index % 5 {
		case 0:
			_, err := h.chat(ctx, "openai-chat", fmt.Sprintf("Reply with BURST_%d only. %s", index, h.runID))
			return err
		case 1:
			resp, err := h.client.CreateEmbedding(ctx, &client.EmbeddingRequest{
				Model: "openai-embed",
				Input: client.NewSingleEmbeddingInput(fmt.Sprintf("polaris burst embedding %d %s", index, h.runID)),
			})
			if err != nil {
				return err
			}
			if len(resp.Data) == 0 || len(resp.Data[0].Embedding.Float32) == 0 {
				return fmt.Errorf("empty embedding response")
			}
		case 2:
			resp, err := h.client.GenerateImage(ctx, &client.ImageGenerationRequest{
				Model:          "openai-image",
				Prompt:         fmt.Sprintf("A tiny gray square on white. %d %s", index, h.runID),
				Size:           "1024x1024",
				ResponseFormat: "url",
			})
			if err != nil {
				return err
			}
			if len(resp.Data) == 0 {
				return fmt.Errorf("empty image response")
			}
		case 3:
			audio, err := h.client.CreateSpeech(ctx, &client.SpeechRequest{
				Model:          "openai-tts",
				Voice:          "nova",
				Input:          fmt.Sprintf("Polaris burst speech %d %s", index, h.runID),
				ResponseFormat: "wav",
			})
			if err != nil {
				return err
			}
			if len(audio.Data) == 0 {
				return fmt.Errorf("empty speech response")
			}
		default:
			audio, err := h.client.CreateSpeech(ctx, &client.SpeechRequest{
				Model:          "openai-tts",
				Voice:          "nova",
				Input:          fmt.Sprintf("Polaris burst transcription %d %s", index, h.runID),
				ResponseFormat: "wav",
			})
			if err != nil {
				return err
			}
			transcript, err := h.client.CreateTranscription(ctx, &client.TranscriptionRequest{
				Model:          "openai-stt",
				File:           audio.Data,
				Filename:       "burst.wav",
				ContentType:    audio.ContentType,
				ResponseFormat: "json",
			})
			if err != nil {
				return err
			}
			if strings.TrimSpace(transcript.Text) == "" {
				return fmt.Errorf("empty transcription response")
			}
		}
		return nil
	})
	if err := errors.Join(errs...); err != nil {
		return "", err
	}
	if err := h.checkReadyMetricsAndUsage(ctx, "model", ""); err != nil {
		return "", err
	}
	return fmt.Sprintf("burst parallel=%d", h.counts.BurstParallel), nil
}

func runLoadVideoAsyncLifecycle(ctx context.Context, h *loadCheckHarness) (string, error) {
	targets := []loadVideoTarget{
		{model: "openai-video", env: []string{"OPENAI_API_KEY"}, prompt: "A four second clip of ocean waves at sunrise"},
		{model: "google-video", env: []string{"GOOGLE_CLOUD_PROJECT", "GOOGLE_CLOUD_LOCATION", "GOOGLE_VERTEX_JOB_SECRET"}, needsADC: true, prompt: "A four second clip of clouds over mountains"},
		{model: "bytedance-video", env: []string{"VOLCENGINE_ARK_API_KEY"}, prompt: "A four second clip of a city skyline at dusk"},
		{model: "bytedance-video-fast", env: []string{"VOLCENGINE_ARK_API_KEY"}, prompt: "A four second clip of a quiet forest path"},
		{model: "replicate-video", env: []string{"REPLICATE_API_KEY"}, prompt: "A short clip of a paper boat on a lake"},
	}

	selected, err := h.selectVideoTargets(targets)
	if err != nil {
		return "", err
	}
	if len(selected) == 0 {
		return "", loadCheckSkip{reason: "no video targets selected by verification policy"}
	}

	errs := h.runParallel(ctx, h.counts.VideoJobs, func(ctx context.Context, index int) error {
		target := selected[index%len(selected)]
		return h.videoLifecycle(ctx, target.model, fmt.Sprintf("%s. %s job %d", target.prompt, h.runID, index+1))
	})
	if err := errors.Join(errs...); err != nil {
		return "", err
	}
	return fmt.Sprintf("video jobs=%d targets=%s", h.counts.VideoJobs, strings.Join(loadVideoModelNames(selected), ",")), nil
}

func runLoadAudioSessionConcurrency(ctx context.Context, h *loadCheckHarness) (string, error) {
	if err := h.requireModels("OpenAI audio sessions", "openai-audio"); err != nil {
		return "", err
	}
	if err := h.requireEnv("OpenAI audio sessions", "OPENAI_API_KEY"); err != nil {
		return "", err
	}

	errs := h.runParallel(ctx, h.counts.AudioSessions, func(ctx context.Context, index int) error {
		turnMode := "manual"
		if index == 0 {
			turnMode = "server_vad"
		}
		return h.audioSessionRoundTrip(ctx, "openai-audio", "marin", turnMode, fmt.Sprintf("Reply with AUDIO_LOAD_%d only. %s", index, h.runID))
	})
	if err := errors.Join(errs...); err != nil {
		return "", err
	}
	if err := h.checkReadyMetricsAndUsage(ctx, "model", "audio"); err != nil {
		return "", err
	}
	return fmt.Sprintf("audio sessions=%d including server_vad", h.counts.AudioSessions), nil
}

func runLoadMusicJobsAndCache(ctx context.Context, h *loadCheckHarness) (string, error) {
	if err := h.requireModels("MiniMax music jobs", "minimax-music"); err != nil {
		return "", err
	}
	if err := h.requireEnv("MiniMax music jobs", "MINIMAX_API_KEY"); err != nil {
		return "", err
	}

	lyricsPayload := map[string]any{
		"model":  "minimax-music",
		"prompt": "Write a short chorus about a compass. " + h.runID,
	}
	if err := h.expectCacheTransition(ctx, "/v1/music/lyrics", lyricsPayload, h.counts.SyncRepeats, "application/json"); err != nil {
		return "", fmt.Errorf("lyrics cache transition: %w", err)
	}

	generationPayload := map[string]any{
		"model":         "minimax-music",
		"mode":          "sync",
		"prompt":        "Short calm instrumental validation. " + h.runID,
		"duration_ms":   10000,
		"instrumental":  true,
		"output_format": "mp3",
	}
	if err := h.expectCacheTransition(ctx, "/v1/music/generations", generationPayload, h.counts.SyncRepeats, "*/*"); err != nil {
		return "", fmt.Errorf("music sync cache transition: %w", err)
	}

	if h.loadOptInEnabledForProviders([]string{"elevenlabs"}) {
		if err := h.requireModels("ElevenLabs music plans", "elevenlabs-music"); err != nil {
			return "", err
		}
		if err := h.requireEnv("ElevenLabs music plans", "ELEVENLABS_API_KEY"); err != nil {
			return "", err
		}
		planPayload := map[string]any{
			"model":       "elevenlabs-music",
			"prompt":      "Plan a short ambient track for release validation. " + h.runID,
			"duration_ms": 10000,
		}
		if err := h.expectCacheTransition(ctx, "/v1/music/plans", planPayload, h.counts.SyncRepeats, "application/json"); err != nil {
			return "", fmt.Errorf("music plan cache transition: %w", err)
		}
	}

	errs := h.runParallel(ctx, h.counts.MusicAsyncJobs, func(ctx context.Context, index int) error {
		resp, err := h.client.CreateMusicGeneration(ctx, &client.MusicGenerationRequest{
			Model:        "minimax-music",
			Mode:         "async",
			Prompt:       fmt.Sprintf("Short async instrumental validation %d %s", index+1, h.runID),
			DurationMS:   10000,
			Instrumental: true,
			OutputFormat: "mp3",
		})
		if err != nil {
			return err
		}
		if resp == nil || resp.Job == nil || strings.TrimSpace(resp.Job.JobID) == "" {
			return fmt.Errorf("empty music job response")
		}
		return h.waitForMusicJobAssetErr(ctx, resp.Job.JobID, 15*time.Minute)
	})
	if err := errors.Join(errs...); err != nil {
		return "", err
	}
	return fmt.Sprintf("music async jobs=%d", h.counts.MusicAsyncJobs), nil
}

func runLoadStoreCacheHealth(ctx context.Context, h *loadCheckHarness) (string, error) {
	if err := h.checkReadyMetricsAndUsage(ctx, "model", ""); err != nil {
		return "", err
	}
	if err := h.checkReadyMetricsAndUsage(ctx, "day", ""); err != nil {
		return "", err
	}
	return "ready, metrics, and usage aggregation passed", nil
}

func (h *loadCheckHarness) requireModels(feature string, modelNames ...string) error {
	result, err := verification.ForConfig(h.cfg, h.registry, modelNames...)
	if err != nil {
		return fmt.Errorf("%s verification policy error: %w", feature, err)
	}
	switch result.Class {
	case verification.ClassSkipped:
		return loadCheckSkip{reason: feature + " is marked skipped by the provider verification matrix"}
	case verification.ClassOptIn:
		if !h.loadOptInEnabledForProviders(result.Providers) {
			return loadCheckSkip{reason: feature + " is opt-in; enable POLARIS_LOAD_INCLUDE_OPT_IN=1 or provider-specific POLARIS_LOAD_PROVIDER_<NAME>=1"}
		}
	}
	return nil
}

func (h *loadCheckHarness) requireEnv(feature string, names ...string) error {
	var missing []string
	for _, name := range names {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%s prerequisites missing: %s", feature, strings.Join(missing, ", "))
}

func (h *loadCheckHarness) requireGoogleADC(feature string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform"); err != nil {
		return fmt.Errorf("%s Google ADC unavailable: %w", feature, err)
	}
	return nil
}

func (h *loadCheckHarness) rawJSON(ctx context.Context, method string, path string, payload any, accept string) (*rawLoadResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(h.baseURL, "/")+path, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("%s %s returned %d: %s", method, path, resp.StatusCode, truncateForError(data))
	}
	return &rawLoadResponse{status: resp.StatusCode, header: resp.Header.Clone(), body: data}, nil
}

func (h *loadCheckHarness) rawGet(ctx context.Context, path string, accept string) (*rawLoadResponse, error) {
	return h.rawJSON(ctx, http.MethodGet, path, nil, accept)
}

func (h *loadCheckHarness) expectCacheTransition(ctx context.Context, path string, payload any, repeats int, accept string) error {
	if repeats < 2 {
		repeats = 2
	}
	var statuses []string
	for i := 0; i < repeats; i++ {
		resp, err := h.rawJSON(ctx, http.MethodPost, path, payload, accept)
		if err != nil {
			return fmt.Errorf("repeat %d: %w", i+1, err)
		}
		if len(resp.body) == 0 {
			return fmt.Errorf("repeat %d returned empty body", i+1)
		}
		statuses = append(statuses, resp.header.Get(loadCacheHeader))
	}
	if statuses[0] != "miss" {
		return fmt.Errorf("first cache status = %q, want miss; statuses=%v", statuses[0], statuses)
	}
	for _, status := range statuses[1:] {
		if status == "hit" {
			return nil
		}
	}
	return fmt.Errorf("no cache hit after %d repeats; statuses=%v", repeats, statuses)
}

func (h *loadCheckHarness) runParallel(ctx context.Context, count int, fn func(context.Context, int) error) []error {
	if count < 1 {
		count = 1
	}
	errs := make([]error, count)
	var wg sync.WaitGroup
	wg.Add(count)
	for i := 0; i < count; i++ {
		i := i
		go func() {
			defer wg.Done()
			callCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
			defer cancel()
			if err := fn(callCtx, i); err != nil {
				errs[i] = fmt.Errorf("worker %d: %w", i+1, err)
			}
		}()
	}
	wg.Wait()
	return errs
}

type loadVideoTarget struct {
	model    string
	env      []string
	needsADC bool
	prompt   string
}

func (h *loadCheckHarness) selectVideoTargets(targets []loadVideoTarget) ([]loadVideoTarget, error) {
	var selected []loadVideoTarget
	for _, target := range targets {
		err := h.requireModels(target.model+" video", target.model)
		if err != nil {
			var skip loadCheckSkip
			if errors.As(err, &skip) {
				continue
			}
			return nil, err
		}
		if err := h.requireEnv(target.model+" video", target.env...); err != nil {
			return nil, err
		}
		if target.needsADC {
			if err := h.requireGoogleADC(target.model + " video"); err != nil {
				return nil, err
			}
		}
		selected = append(selected, target)
	}
	return selected, nil
}

func loadVideoModelNames(targets []loadVideoTarget) []string {
	names := make([]string, 0, len(targets))
	for _, target := range targets {
		names = append(names, target.model)
	}
	return names
}

func (h *loadCheckHarness) videoLifecycle(ctx context.Context, model string, prompt string) error {
	job, err := h.client.CreateVideoGeneration(ctx, &client.VideoGenerationRequest{
		Model:       model,
		Prompt:      prompt,
		Duration:    4,
		AspectRatio: "16:9",
		Resolution:  "720p",
	})
	if err != nil {
		return fmt.Errorf("submit %s video: %w", model, err)
	}
	if strings.TrimSpace(job.JobID) == "" {
		return fmt.Errorf("%s video returned empty job id", model)
	}
	return h.waitForVideoJob(ctx, job.JobID, 20*time.Minute)
}

func (h *loadCheckHarness) waitForVideoJob(ctx context.Context, jobID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last *client.VideoStatus
	for {
		status, err := h.client.GetVideoGeneration(ctx, jobID)
		if err != nil {
			return fmt.Errorf("get video job %s: %w", jobID, err)
		}
		last = status
		switch strings.ToLower(strings.TrimSpace(status.Status)) {
		case "completed", "succeeded", "success":
			asset, err := h.client.GetVideoGenerationContent(ctx, jobID)
			if err != nil {
				return fmt.Errorf("download video job %s: %w", jobID, err)
			}
			if len(asset.Data) == 0 {
				return fmt.Errorf("video job %s completed with empty content", jobID)
			}
			return nil
		case "failed", "cancelled", "canceled":
			return fmt.Errorf("video job %s ended in %s: %#v", jobID, status.Status, status)
		}
		if time.Now().After(deadline) {
			if loadAcceptProviderTimeouts() {
				return nil
			}
			return fmt.Errorf("timed out waiting for video job %s; last status=%#v", jobID, last)
		}
		time.Sleep(10 * time.Second)
	}
}

func (h *loadCheckHarness) audioSessionRoundTrip(ctx context.Context, model string, voice string, turnMode string, prompt string) error {
	session, err := h.client.CreateAudioSession(ctx, &client.AudioSessionRequest{
		Model:         model,
		Voice:         voice,
		TurnDetection: &client.AudioTurnDetection{Mode: turnMode},
	})
	if err != nil {
		return fmt.Errorf("create audio session: %w", err)
	}
	target := strings.TrimSpace(session.WebSocketURL)
	if target == "" {
		target = h.defaultAudioWebSocketURL(session.ID)
	}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+session.ClientSecret)
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, target, headers)
	if err != nil {
		return fmt.Errorf("dial audio session: %w", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Minute)); err != nil {
		return fmt.Errorf("set read deadline: %w", err)
	}
	var created client.AudioServerEvent
	if err := conn.ReadJSON(&created); err != nil {
		return fmt.Errorf("read session.created: %w", err)
	}
	if created.Type != string(modality.AudioServerEventSessionCreated) {
		return fmt.Errorf("expected session.created, got %#v", created)
	}
	if err := conn.WriteJSON(&client.AudioClientEvent{Type: string(modality.AudioClientEventInputText), Text: prompt}); err != nil {
		return fmt.Errorf("send input_text: %w", err)
	}
	if err := conn.WriteJSON(&client.AudioClientEvent{Type: string(modality.AudioClientEventResponseCreate)}); err != nil {
		return fmt.Errorf("send response.create: %w", err)
	}
	for i := 0; i < 40; i++ {
		if err := conn.SetReadDeadline(time.Now().Add(2 * time.Minute)); err != nil {
			return fmt.Errorf("set read deadline: %w", err)
		}
		var event client.AudioServerEvent
		if err := conn.ReadJSON(&event); err != nil {
			return fmt.Errorf("read audio event: %w", err)
		}
		if event.Error != nil {
			return fmt.Errorf("audio session error: %#v", event.Error)
		}
		if event.Type == string(modality.AudioServerEventResponseCompleted) {
			if event.Usage == nil || event.Usage.TotalTokens == 0 {
				return fmt.Errorf("response.completed missing usage: %#v", event)
			}
			_ = conn.WriteJSON(&client.AudioClientEvent{Type: string(modality.AudioClientEventSessionClose)})
			return nil
		}
	}
	return fmt.Errorf("audio session did not complete within event limit")
}

func (h *loadCheckHarness) defaultAudioWebSocketURL(sessionID string) string {
	parsed, err := url.Parse(h.baseURL)
	if err != nil {
		return ""
	}
	if parsed.Scheme == "https" {
		parsed.Scheme = "wss"
	} else {
		parsed.Scheme = "ws"
	}
	parsed.Path = "/v1/audio/sessions/" + sessionID + "/ws"
	parsed.RawQuery = ""
	return parsed.String()
}

func (h *loadCheckHarness) waitForMusicJobAssetErr(ctx context.Context, jobID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		status, err := h.client.GetMusicJob(ctx, jobID)
		if err != nil {
			return fmt.Errorf("get music job %s: %w", jobID, err)
		}
		switch strings.ToLower(strings.TrimSpace(status.Status)) {
		case "completed", "succeeded", "success":
			asset, err := h.client.GetMusicJobContent(ctx, jobID)
			if err != nil {
				return fmt.Errorf("download music job %s: %w", jobID, err)
			}
			if len(asset.Data) == 0 {
				return fmt.Errorf("music job %s completed with empty content", jobID)
			}
			return nil
		case "failed", "cancelled", "canceled":
			return fmt.Errorf("music job %s ended in %s: %#v", jobID, status.Status, status)
		}
		if time.Now().After(deadline) {
			if loadAcceptProviderTimeouts() {
				return nil
			}
			return fmt.Errorf("timed out waiting for music job %s; last status=%#v", jobID, status)
		}
		time.Sleep(10 * time.Second)
	}
}

func (h *loadCheckHarness) checkReadyMetricsAndUsage(ctx context.Context, groupBy string, usageModality string) error {
	time.Sleep(2 * time.Second)
	ready, err := h.rawGet(ctx, "/ready", "application/json")
	if err != nil {
		return fmt.Errorf("ready check: %w", err)
	}
	if ready.status != http.StatusOK {
		return fmt.Errorf("ready status = %d", ready.status)
	}
	metrics, err := h.rawGet(ctx, "/metrics", "text/plain")
	if err != nil {
		return fmt.Errorf("metrics check: %w", err)
	}
	if metrics.status != http.StatusOK || !bytes.Contains(metrics.body, []byte("polaris_requests_total")) {
		return fmt.Errorf("metrics response did not include polaris_requests_total")
	}
	usage, err := h.client.GetUsage(ctx, &client.UsageParams{GroupBy: groupBy, Modality: usageModality})
	if err != nil {
		return fmt.Errorf("usage check: %w", err)
	}
	if usage.TotalRequests == 0 {
		return fmt.Errorf("usage returned zero requests")
	}
	return nil
}

func (h *loadCheckHarness) recordScenario(name string, status string, duration time.Duration, note string) {
	h.report.Scenarios = append(h.report.Scenarios, loadCheckScenarioResult{
		Name:     name,
		Status:   status,
		Duration: duration,
		Note:     note,
	})
}

func (h *loadCheckHarness) writeReport(t *testing.T) {
	t.Helper()
	h.report.RunID = h.runID

	var b strings.Builder
	fmt.Fprintf(&b, "# Polaris Load Check Report\n\n")
	fmt.Fprintf(&b, "- backend: %s\n", h.report.Backend)
	fmt.Fprintf(&b, "- run_id: %s\n", h.report.RunID)
	fmt.Fprintf(&b, "- started_at: %s\n\n", h.report.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "| Scenario | Status | Duration | Note |\n")
	fmt.Fprintf(&b, "|---|---|---:|---|\n")
	for _, scenario := range h.report.Scenarios {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", scenario.Name, scenario.Status, scenario.Duration.Round(time.Millisecond), strings.ReplaceAll(scenario.Note, "|", "\\|"))
	}

	t.Log("\n" + b.String())

	path := strings.TrimSpace(os.Getenv("POLARIS_LOAD_REPORT"))
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create load report directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write load report: %v", err)
	}
}

func (h *loadCheckHarness) loadOptInEnabledForProviders(providerNames []string) bool {
	if liveSmokeOptInEnabled("POLARIS_LOAD_INCLUDE_OPT_IN") {
		return true
	}
	for _, providerName := range providerNames {
		name := "POLARIS_LOAD_PROVIDER_" + strings.ToUpper(strings.NewReplacer("-", "_", "/", "_").Replace(strings.TrimSpace(providerName)))
		if liveSmokeOptInEnabled(name) {
			return true
		}
		if providerName == "elevenlabs" && liveSmokeOptInEnabled("POLARIS_LOAD_ELEVENLABS") {
			return true
		}
	}
	return false
}

func loadAcceptProviderTimeouts() bool {
	raw := strings.TrimSpace(os.Getenv("POLARIS_LOAD_ACCEPT_PROVIDER_TIMEOUTS"))
	return raw == "" || raw == "1" || strings.EqualFold(raw, "true")
}

func truncateForError(data []byte) string {
	text := strings.TrimSpace(string(data))
	if len(text) > 500 {
		return text[:500] + "..."
	}
	return text
}
