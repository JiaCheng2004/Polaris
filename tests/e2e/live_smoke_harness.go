package e2e

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2/google"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/provider/verification"
	"github.com/JiaCheng2004/Polaris/internal/store"
	storecache "github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/JiaCheng2004/Polaris/internal/store/postgres"
	"github.com/JiaCheng2004/Polaris/internal/store/sqlite"
	"github.com/JiaCheng2004/Polaris/pkg/client"
)

type liveSmokeHarness struct {
	client       *client.Client
	cfg          *config.Config
	registry     *provider.Registry
	baseURL      string
	strict       bool
	includeOptIn bool
}

const tinyTransparentPNGDataURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAIAAACQkWg2AAAAHklEQVR4nGL5//8/AymAiSTVoxpGNQwpDYAAAAD//16dAyAxI91VAAAAAElFTkSuQmCC"
const liveSmokeSpeechSampleURL = "https://raw.githubusercontent.com/Uberi/speech_recognition/master/examples/english.wav"

var errInvalidOllamaURL = errors.New("OLLAMA_BASE_URL must use localhost, loopback, or private-network host")

type liveSmokeHarnessOptions struct {
	storeDriver          string
	storeDSN             string
	cacheDriver          string
	cacheURL             string
	responseCacheEnabled *bool
	rateLimitEnabled     *bool
	logBufferSize        int
	logFlushInterval     time.Duration
}

func newLiveSmokeHarness(t *testing.T) *liveSmokeHarness {
	t.Helper()
	return newLiveSmokeHarnessWithOptions(t, liveSmokeHarnessOptions{})
}

func newLiveSmokeHarnessWithOptions(t *testing.T, opts liveSmokeHarnessOptions) *liveSmokeHarness {
	t.Helper()

	if strings.TrimSpace(os.Getenv("MINIMAX_BASE_URL")) == "" {
		t.Setenv("MINIMAX_BASE_URL", "https://api.minimax.io")
	}
	if strings.TrimSpace(os.Getenv("GIN_MODE")) == "" {
		t.Setenv("GIN_MODE", "release")
	}

	configPath := os.Getenv("POLARIS_LIVE_SMOKE_CONFIG")
	if strings.TrimSpace(configPath) == "" {
		configPath = filepath.Join("..", "..", "config", "polaris.live-smoke.yaml")
	}

	cfg, warnings, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	for _, warning := range warnings {
		t.Logf("config warning: %s", warning)
	}

	if opts.storeDriver != "" {
		cfg.Store.Driver = opts.storeDriver
	}
	if opts.storeDSN != "" {
		cfg.Store.DSN = opts.storeDSN
	} else if strings.EqualFold(cfg.Store.Driver, "sqlite") {
		cfg.Store.DSN = filepath.Join(t.TempDir(), "polaris-live-smoke.db")
	}
	if opts.cacheDriver != "" {
		cfg.Cache.Driver = opts.cacheDriver
	}
	if opts.cacheURL != "" {
		cfg.Cache.URL = opts.cacheURL
	}
	if opts.responseCacheEnabled != nil {
		cfg.Cache.ResponseCache.Enabled = *opts.responseCacheEnabled
	}
	if opts.rateLimitEnabled != nil {
		cfg.Cache.RateLimit.Enabled = *opts.rateLimitEnabled
	}
	if opts.logBufferSize > 0 {
		cfg.Store.LogBufferSize = opts.logBufferSize
	}
	if opts.logFlushInterval > 0 {
		cfg.Store.LogFlushInterval = opts.logFlushInterval
	}

	appStore, err := newE2EStore(cfg.Store)
	if err != nil {
		t.Fatalf("newE2EStore(%s) error = %v", cfg.Store.Driver, err)
	}
	t.Cleanup(func() { _ = appStore.Close() })
	if err := appStore.Migrate(context.Background()); err != nil {
		t.Fatalf("appStore.Migrate() error = %v", err)
	}

	appCache, err := newE2ECache(cfg.Cache)
	if err != nil {
		t.Fatalf("newE2ECache(%s) error = %v", cfg.Cache.Driver, err)
	}
	t.Cleanup(func() { _ = appCache.Close() })

	registry, registryWarnings, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	for _, warning := range registryWarnings {
		t.Logf("registry warning: %s", warning)
	}

	requestLogger := store.NewAsyncRequestLogger(appStore, slog.New(slog.NewTextHandler(io.Discard, nil)), store.NewLoggerConfig(100, 25*time.Millisecond))
	t.Cleanup(func() {
		_ = requestLogger.Close(context.Background())
	})

	engine, err := gateway.NewEngine(gateway.Dependencies{
		Config:        cfg,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:         appStore,
		Cache:         appCache,
		Registry:      registry,
		RequestLogger: requestLogger,
	})
	if err != nil {
		t.Fatalf("gateway.NewEngine() error = %v", err)
	}

	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)

	sdk, err := client.New(server.URL)
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}

	return &liveSmokeHarness{
		client:       sdk,
		cfg:          cfg,
		registry:     registry,
		baseURL:      server.URL,
		strict:       os.Getenv("POLARIS_LIVE_SMOKE_STRICT") == "1",
		includeOptIn: liveSmokeOptInEnabled("POLARIS_LIVE_SMOKE_INCLUDE_OPT_IN"),
	}
}

func newE2EStore(cfg config.StoreConfig) (store.Store, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Driver)) {
	case "", "sqlite":
		return sqlite.New(cfg)
	case "postgres", "postgresql":
		return postgres.New(cfg)
	default:
		return nil, os.ErrInvalid
	}
}

func newE2ECache(cfg config.CacheConfig) (storecache.Cache, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Driver)) {
	case "", "memory":
		return storecache.NewMemory(), nil
	case "redis":
		return storecache.NewRedis(cfg.URL)
	default:
		return nil, os.ErrInvalid
	}
}

func liveSmokeOptInEnabled(name string) bool {
	return strings.TrimSpace(os.Getenv(name)) == "1"
}

func liveSmokeProviderOptInName(providerName string) string {
	replacer := strings.NewReplacer("-", "_", "/", "_")
	return "POLARIS_LIVE_SMOKE_PROVIDER_" + strings.ToUpper(replacer.Replace(strings.TrimSpace(providerName)))
}

func liveSmokeLegacyProviderOptInNames(providerName string) []string {
	switch providerName {
	case "elevenlabs":
		return []string{"POLARIS_LIVE_SMOKE_ELEVENLABS"}
	default:
		return nil
	}
}

func (h *liveSmokeHarness) optInEnabledForProviders(providerNames []string) bool {
	if h.includeOptIn {
		return true
	}
	for _, providerName := range providerNames {
		if liveSmokeOptInEnabled(liveSmokeProviderOptInName(providerName)) {
			return true
		}
		for _, legacy := range liveSmokeLegacyProviderOptInNames(providerName) {
			if liveSmokeOptInEnabled(legacy) {
				return true
			}
		}
	}
	return false
}

func (h *liveSmokeHarness) optInHints(providerNames []string) []string {
	hints := []string{"POLARIS_LIVE_SMOKE_INCLUDE_OPT_IN=1"}
	seen := map[string]struct{}{
		hints[0]: {},
	}
	for _, providerName := range providerNames {
		name := liveSmokeProviderOptInName(providerName) + "=1"
		if _, ok := seen[name]; !ok {
			hints = append(hints, name)
			seen[name] = struct{}{}
		}
		for _, legacy := range liveSmokeLegacyProviderOptInNames(providerName) {
			name := legacy + "=1"
			if _, ok := seen[name]; !ok {
				hints = append(hints, name)
				seen[name] = struct{}{}
			}
		}
	}
	return hints
}

func (h *liveSmokeHarness) gateCase(t *testing.T, feature string, modelNames ...string) verification.Result {
	t.Helper()

	result, err := verification.ForConfig(h.cfg, h.registry, modelNames...)
	if err != nil {
		t.Fatalf("%s verification policy error: %v", feature, err)
	}
	switch result.Class {
	case verification.ClassSkipped:
		t.Skipf("%s is marked skipped by the provider verification matrix", feature)
	case verification.ClassOptIn:
		if !h.optInEnabledForProviders(result.Providers) {
			t.Skipf("%s is opt-in; enable with %s", feature, strings.Join(h.optInHints(result.Providers), " or "))
		}
	}
	return result
}

func (h *liveSmokeHarness) mandatory(policy verification.Result) bool {
	return policy.Class == verification.ClassOptIn || (policy.Class == verification.ClassStrict && h.strict)
}

func (h *liveSmokeHarness) chat(ctx context.Context, model string, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	resp, err := h.client.CreateChatCompletion(ctx, &client.ChatCompletionRequest{
		Model: model,
		Messages: []client.ChatMessage{
			{Role: "user", Content: client.NewTextContent(prompt)},
		},
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content.Text == nil {
		return "", nil
	}
	return strings.TrimSpace(*resp.Choices[0].Message.Content.Text), nil
}

func (h *liveSmokeHarness) chatWithImage(ctx context.Context, model string, prompt string, imageURL string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	resp, err := h.client.CreateChatCompletion(ctx, &client.ChatCompletionRequest{
		Model: model,
		Messages: []client.ChatMessage{
			{
				Role: "user",
				Content: client.NewPartContent(
					client.ContentPart{Type: "text", Text: prompt},
					client.ContentPart{
						Type:     "image_url",
						ImageURL: &client.ImageURLPart{URL: imageURL},
					},
				),
			},
		},
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content.Text == nil {
		return "", nil
	}
	return strings.TrimSpace(*resp.Choices[0].Message.Content.Text), nil
}

func (h *liveSmokeHarness) requireEnv(t *testing.T, policy verification.Result, feature string, names ...string) {
	t.Helper()

	var missing []string
	for _, name := range names {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return
	}

	message := feature + " prerequisites missing: " + strings.Join(missing, ", ")
	if h.mandatory(policy) {
		t.Fatal(message)
	}
	t.Skip(message)
}

func (h *liveSmokeHarness) requireGoogleADC(t *testing.T, policy verification.Result) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform"); err != nil {
		if h.mandatory(policy) {
			t.Fatalf("Google Vertex ADC unavailable: %v", err)
		}
		t.Skipf("Google Vertex ADC unavailable: %v", err)
	}
}

func (h *liveSmokeHarness) requireOllama(t *testing.T, policy verification.Result) {
	t.Helper()

	baseURL := strings.TrimSpace(os.Getenv("OLLAMA_BASE_URL"))
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if err := validateLocalOllamaBaseURL(baseURL); err != nil {
		if h.mandatory(policy) {
			t.Fatalf("Ollama base URL is not an allowed local URL: %v", err)
		}
		t.Skipf("Ollama base URL is not an allowed local URL: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/tags", nil)
	if err != nil {
		t.Fatalf("build Ollama request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if h.mandatory(policy) {
			t.Fatalf("Ollama is unavailable at %s: %v", baseURL, err)
		}
		t.Skipf("Ollama is unavailable at %s: %v", baseURL, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 400 {
		if h.mandatory(policy) {
			t.Fatalf("Ollama tags endpoint returned %d", resp.StatusCode)
		}
		t.Skipf("Ollama tags endpoint returned %d", resp.StatusCode)
	}
}

func validateLocalOllamaBaseURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return &url.Error{Op: "parse", URL: raw, Err: errInvalidOllamaURL}
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return &url.Error{Op: "parse", URL: raw, Err: errInvalidOllamaURL}
	}
	if ip.IsLoopback() || ip.IsPrivate() {
		return nil
	}
	return &url.Error{Op: "parse", URL: raw, Err: errInvalidOllamaURL}
}

func (h *liveSmokeHarness) requireContains(t *testing.T, got string, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected %q to contain %q", got, want)
	}
}

func (h *liveSmokeHarness) voiceRoundTrip(t *testing.T, ctx context.Context, ttsModel string, sttModel string, voice string, input string, format string) (*client.Audio, *client.TranscriptionResponse) {
	t.Helper()

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	audio, err := h.client.CreateSpeech(ctx, &client.SpeechRequest{
		Model:          ttsModel,
		Input:          input,
		Voice:          voice,
		ResponseFormat: format,
	})
	if err != nil {
		t.Fatalf("CreateSpeech(%s) error = %v", ttsModel, err)
	}

	transcript, err := h.client.CreateTranscription(ctx, &client.TranscriptionRequest{
		Model:          sttModel,
		File:           audio.Data,
		Filename:       "smoke." + format,
		ContentType:    audio.ContentType,
		ResponseFormat: "json",
	})
	if err != nil {
		t.Fatalf("CreateTranscription(%s) error = %v", sttModel, err)
	}
	return audio, transcript
}

func (h *liveSmokeHarness) audioSessionRoundTrip(t *testing.T, ctx context.Context, model string, voice string, turnMode string, prompt string) (*client.AudioServerEvent, string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	session, err := h.client.CreateAudioSession(ctx, &client.AudioSessionRequest{
		Model:         model,
		Voice:         voice,
		TurnDetection: &client.AudioTurnDetection{Mode: turnMode},
	})
	if err != nil {
		t.Fatalf("CreateAudioSession(%s) error = %v", model, err)
	}

	conn, err := h.client.DialAudioSession(ctx, session)
	if err != nil {
		t.Fatalf("DialAudioSession(%s) error = %v", model, err)
	}
	defer func() {
		_ = conn.Close()
	}()

	created := receiveAudioEvent(t, conn)
	if created.Type != string(modality.AudioServerEventSessionCreated) {
		t.Fatalf("expected session.created, got %#v", created)
	}

	if err := conn.Send(&client.AudioClientEvent{
		Type: string(modality.AudioClientEventInputText),
		Text: prompt,
	}); err != nil {
		t.Fatalf("Send(input_text) error = %v", err)
	}
	if err := conn.Send(&client.AudioClientEvent{
		Type: string(modality.AudioClientEventResponseCreate),
	}); err != nil {
		t.Fatalf("Send(response.create) error = %v", err)
	}

	var (
		completed       *client.AudioServerEvent
		transcriptParts []string
	)
	for completed == nil {
		event := receiveAudioEvent(t, conn)
		if strings.TrimSpace(event.Text) != "" {
			transcriptParts = append(transcriptParts, event.Text)
		}
		if strings.TrimSpace(event.Transcript) != "" {
			transcriptParts = append(transcriptParts, event.Transcript)
		}
		if event.Type == string(modality.AudioServerEventResponseCompleted) {
			completed = event
		}
	}

	_ = conn.Send(&client.AudioClientEvent{Type: string(modality.AudioClientEventSessionClose)})
	return completed, strings.Join(transcriptParts, " ")
}

func (h *liveSmokeHarness) streamingTranscriptionRoundTrip(t *testing.T, ctx context.Context, sttModel string, ttsModel string, voice string, input string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	audio, err := h.client.CreateSpeech(ctx, &client.SpeechRequest{
		Model:          ttsModel,
		Input:          input,
		Voice:          voice,
		ResponseFormat: "pcm",
	})
	if err != nil {
		t.Fatalf("CreateSpeech(%s) error = %v", ttsModel, err)
	}
	if len(audio.Data) == 0 {
		t.Fatalf("expected non-empty PCM audio from %s", ttsModel)
	}

	session, err := h.client.CreateStreamingTranscriptionSession(ctx, &client.StreamingTranscriptionSessionRequest{
		Model:            sttModel,
		InputAudioFormat: "pcm16",
		SampleRateHz:     16000,
	})
	if err != nil {
		t.Fatalf("CreateStreamingTranscriptionSession(%s) error = %v", sttModel, err)
	}

	conn, err := h.client.DialStreamingTranscriptionSession(ctx, session)
	if err != nil {
		t.Fatalf("DialStreamingTranscriptionSession(%s) error = %v", sttModel, err)
	}
	defer func() {
		_ = conn.Close()
	}()

	created := receiveStreamingEvent(t, conn)
	if created.Type != modality.StreamingTranscriptionServerEventSessionCreated {
		t.Fatalf("expected session.created, got %#v", created)
	}

	if err := conn.Send(&client.StreamingTranscriptionClientEvent{
		Type:  modality.StreamingTranscriptionClientEventInputAudioAppend,
		Audio: base64.StdEncoding.EncodeToString(audio.Data),
	}); err != nil {
		t.Fatalf("Send(input_audio.append) error = %v", err)
	}
	if err := conn.Send(&client.StreamingTranscriptionClientEvent{
		Type: modality.StreamingTranscriptionClientEventInputAudioCommit,
	}); err != nil {
		t.Fatalf("Send(input_audio.commit) error = %v", err)
	}

	var transcriptParts []string
	for {
		event := receiveStreamingEvent(t, conn)
		if strings.TrimSpace(event.Text) != "" {
			transcriptParts = append(transcriptParts, event.Text)
		}
		if event.Segment != nil && strings.TrimSpace(event.Segment.Text) != "" {
			transcriptParts = append(transcriptParts, event.Segment.Text)
		}
		if event.Transcript != nil && strings.TrimSpace(event.Transcript.Text) != "" {
			transcriptParts = append(transcriptParts, event.Transcript.Text)
		}
		if event.Error != nil {
			t.Fatalf("unexpected streaming transcription error event %#v", event)
		}
		if event.Type == modality.StreamingTranscriptionServerEventTranscriptCompleted {
			break
		}
	}

	_ = conn.Send(&client.StreamingTranscriptionClientEvent{Type: modality.StreamingTranscriptionClientEventSessionClose})
	return strings.Join(transcriptParts, " ")
}

func (h *liveSmokeHarness) interpretingRoundTrip(t *testing.T, ctx context.Context, model string, ttsModel string, voice string, input string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	audio, err := h.client.CreateSpeech(ctx, &client.SpeechRequest{
		Model:          ttsModel,
		Input:          input,
		Voice:          voice,
		ResponseFormat: "pcm",
	})
	if err != nil {
		t.Fatalf("CreateSpeech(%s) error = %v", ttsModel, err)
	}
	if len(audio.Data) == 0 {
		t.Fatalf("expected non-empty PCM audio from %s", ttsModel)
	}

	session, err := h.client.CreateInterpretingSession(ctx, &client.InterpretingSessionRequest{
		Model:             model,
		Mode:              modality.InterpretingModeSpeechToText,
		SourceLanguage:    "en",
		TargetLanguage:    "zh",
		InputAudioFormat:  modality.InterpretingAudioFormatPCM16,
		InputSampleRateHz: 16000,
	})
	if err != nil {
		t.Fatalf("CreateInterpretingSession(%s) error = %v", model, err)
	}

	conn, err := h.client.DialInterpretingSession(ctx, session)
	if err != nil {
		t.Fatalf("DialInterpretingSession(%s) error = %v", model, err)
	}
	defer func() {
		_ = conn.Close()
	}()

	created := receiveInterpretingEvent(t, conn)
	if created.Type != modality.InterpretingServerEventSessionCreated {
		t.Fatalf("expected session.created, got %#v", created)
	}

	if err := conn.Send(&client.InterpretingClientEvent{
		Type: modality.InterpretingClientEventSessionUpdate,
		Session: &client.InterpretingSessionRequest{
			Model:             model,
			Mode:              modality.InterpretingModeSpeechToText,
			SourceLanguage:    "en",
			TargetLanguage:    "zh",
			InputAudioFormat:  modality.InterpretingAudioFormatPCM16,
			InputSampleRateHz: 16000,
		},
	}); err != nil {
		t.Fatalf("Send(session.update) error = %v", err)
	}

	const chunkSize = 2560
	for offset := 0; offset < len(audio.Data); offset += chunkSize {
		end := offset + chunkSize
		if end > len(audio.Data) {
			end = len(audio.Data)
		}
		if err := conn.Send(&client.InterpretingClientEvent{
			Type:  modality.InterpretingClientEventInputAudioAppend,
			Audio: base64.StdEncoding.EncodeToString(audio.Data[offset:end]),
		}); err != nil {
			t.Fatalf("Send(input_audio.append) error = %v", err)
		}
		time.Sleep(80 * time.Millisecond)
	}
	if err := conn.Send(&client.InterpretingClientEvent{
		Type: modality.InterpretingClientEventInputAudioCommit,
	}); err != nil {
		t.Fatalf("Send(input_audio.commit) error = %v", err)
	}

	var translated []string
	for {
		event := receiveInterpretingEvent(t, conn)
		if strings.TrimSpace(event.Text) != "" {
			translated = append(translated, event.Text)
		}
		if event.Segment != nil && strings.TrimSpace(event.Segment.Text) != "" {
			translated = append(translated, event.Segment.Text)
		}
		if event.Error != nil {
			t.Fatalf("unexpected interpreting error event: type=%s code=%s message=%s param=%s", event.Error.Type, event.Error.Code, event.Error.Message, event.Error.Param)
		}
		if event.Type == modality.InterpretingServerEventResponseCompleted {
			break
		}
	}

	_ = conn.Send(&client.InterpretingClientEvent{Type: modality.InterpretingClientEventSessionClose})
	return strings.Join(translated, " ")
}

func (h *liveSmokeHarness) waitForMusicJobAsset(t *testing.T, ctx context.Context, jobID string, timeout time.Duration) (*client.MusicStatus, *client.MusicAsset) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		status, err := h.client.GetMusicJob(callCtx, jobID)
		cancel()
		if err != nil {
			t.Fatalf("GetMusicJob(%s) error = %v", jobID, err)
		}
		switch status.Status {
		case "completed":
			callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			asset, err := h.client.GetMusicJobContent(callCtx, jobID)
			cancel()
			if err != nil {
				t.Fatalf("GetMusicJobContent(%s) error = %v", jobID, err)
			}
			return status, asset
		case "failed", "cancelled", "canceled":
			t.Fatalf("music job %s ended in %s: %#v", jobID, status.Status, status)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for music job %s to complete; last status=%#v", jobID, status)
		}
		time.Sleep(10 * time.Second)
	}
}

func (h *liveSmokeHarness) waitForAudioNote(t *testing.T, ctx context.Context, noteID string, timeout time.Duration) *client.AudioNoteResult {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		job, err := h.client.GetAudioNote(callCtx, noteID)
		cancel()
		if err != nil {
			t.Fatalf("GetAudioNote(%s) error = %v", noteID, err)
		}
		switch job.Status {
		case "completed", "succeeded", "success":
			if job.Result == nil {
				t.Fatalf("audio note %s completed without a result: %#v", noteID, job)
			}
			return job.Result
		case "failed", "cancelled", "canceled":
			t.Fatalf("audio note %s ended in %s: %#v", noteID, job.Status, job)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for audio note %s to complete; last status=%#v", noteID, job)
		}
		time.Sleep(5 * time.Second)
	}
}

func (h *liveSmokeHarness) waitForPodcastAsset(t *testing.T, ctx context.Context, podcastID string, timeout time.Duration) (*client.PodcastStatus, *client.PodcastAsset) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		status, err := h.client.GetPodcast(callCtx, podcastID)
		cancel()
		if err != nil {
			t.Fatalf("GetPodcast(%s) error = %v", podcastID, err)
		}
		switch status.Status {
		case "completed", "succeeded", "success":
			callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			asset, err := h.client.GetPodcastContent(callCtx, podcastID)
			cancel()
			if err != nil {
				t.Fatalf("GetPodcastContent(%s) error = %v", podcastID, err)
			}
			return status, asset
		case "failed", "cancelled", "canceled":
			t.Fatalf("podcast %s ended in %s: %#v", podcastID, status.Status, status)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for podcast %s to complete; last status=%#v", podcastID, status)
		}
		time.Sleep(5 * time.Second)
	}
}

func receiveAudioEvent(t *testing.T, conn *client.AudioSessionConn) *client.AudioServerEvent {
	t.Helper()

	type result struct {
		event *client.AudioServerEvent
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		event, err := conn.Receive()
		ch <- result{event: event, err: err}
	}()

	select {
	case got := <-ch:
		if got.err != nil {
			t.Fatalf("Receive() error = %v", got.err)
		}
		return got.event
	case <-time.After(45 * time.Second):
		t.Fatalf("timed out waiting for audio session event")
		return nil
	}
}

func receiveStreamingEvent(t *testing.T, conn *client.StreamingTranscriptionConn) *client.StreamingTranscriptionEvent {
	t.Helper()

	type result struct {
		event *client.StreamingTranscriptionEvent
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		event, err := conn.Receive()
		ch <- result{event: event, err: err}
	}()

	select {
	case got := <-ch:
		if got.err != nil {
			t.Fatalf("Receive() error = %v", got.err)
		}
		return got.event
	case <-time.After(45 * time.Second):
		t.Fatalf("timed out waiting for streaming transcription event")
		return nil
	}
}

func receiveInterpretingEvent(t *testing.T, conn *client.InterpretingConn) *client.InterpretingEvent {
	t.Helper()

	type result struct {
		event *client.InterpretingEvent
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		event, err := conn.Receive()
		ch <- result{event: event, err: err}
	}()

	select {
	case got := <-ch:
		if got.err != nil {
			t.Fatalf("Receive() error = %v", got.err)
		}
		return got.event
	case <-time.After(45 * time.Second):
		t.Fatalf("timed out waiting for interpreting event")
		return nil
	}
}

func firstImageValue(result client.ImageResult) string {
	if strings.TrimSpace(result.URL) != "" {
		return result.URL
	}
	return result.B64JSON
}
