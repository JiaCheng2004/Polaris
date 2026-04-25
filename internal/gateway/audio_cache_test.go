package gateway

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/gorilla/websocket"
)

func TestAudioSessionLifecycleOpenAI(t *testing.T) {
	var chatCalls int32
	var speechCalls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			atomic.AddInt32(&chatCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id":"chatcmpl-audio",
				"object":"chat.completion",
				"created":1744329600,
				"model":"gpt-4o",
				"choices":[{"index":0,"message":{"role":"assistant","content":"Hello from audio"},"finish_reason":"stop"}],
				"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}
			}`))
		case "/v1/audio/speech":
			atomic.AddInt32(&speechCalls, 1)
			w.Header().Set("Content-Type", "audio/L16")
			_, _ = w.Write([]byte{0x00, 0x01, 0x00, 0x02})
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "100/min",
		AllowedModels: []string{"openai/*"},
	}}
	cfg.Providers["openai"] = config.ProviderConfig{
		APIKey:  "sk-openai",
		BaseURL: upstream.URL + "/v1",
		Timeout: time.Second,
		Models: map[string]config.ModelConfig{
			"gpt-4o": {
				Modality:     modality.ModalityChat,
				Capabilities: []modality.Capability{modality.CapabilityStreaming, modality.CapabilityJSONMode},
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
			"gpt-4o-audio": {
				Modality:     modality.ModalityAudio,
				Capabilities: []modality.Capability{modality.CapabilityAudioInput, modality.CapabilityAudioOutput},
				Voices:       []string{"nova"},
				SessionTTL:   2 * time.Minute,
				AudioPipeline: config.AudioPipelineConfig{
					ChatModel: "openai/gpt-4o",
					STTModel:  "openai/whisper-1",
					TTSModel:  "openai/tts-1",
				},
			},
		},
	}
	cfg.Routing.Aliases = map[string]string{
		"default-audio": "openai/gpt-4o-audio",
	}

	sqliteStore := testSQLiteStore(t)
	registry, warnings, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no registry warnings, got %v", warnings)
	}
	requestLogger := store.NewAsyncRequestLogger(sqliteStore, slog.New(slog.NewTextHandler(io.Discard, nil)), store.NewLoggerConfig(10, 5*time.Millisecond))

	engine, err := NewEngine(Dependencies{
		Config:        cfg,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:         sqliteStore,
		Cache:         cache.NewMemory(),
		Registry:      registry,
		RequestLogger: requestLogger,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	server := httptest.NewServer(engine)
	defer server.Close()

	reqBody := strings.NewReader(`{"model":"default-audio","voice":"nova"}`)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/audio/sessions", reqBody)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do(create audio session) error = %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var session modality.AudioSessionDescriptor
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode audio session: %v", err)
	}
	if session.ID == "" || session.ClientSecret == "" || !strings.HasPrefix(session.WebSocketURL, "ws://") {
		t.Fatalf("unexpected session descriptor %#v", session)
	}

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+session.ClientSecret)
	conn, _, err := websocket.DefaultDialer.Dial(session.WebSocketURL, headers)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	readEvent := func() modality.AudioServerEvent {
		t.Helper()
		if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatalf("SetReadDeadline() error = %v", err)
		}
		var event modality.AudioServerEvent
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("ReadJSON() error = %v", err)
		}
		return event
	}

	created := readEvent()
	if created.Type != modality.AudioServerEventSessionCreated || created.Session == nil || created.Session.ID != session.ID {
		t.Fatalf("unexpected created event %#v", created)
	}

	if err := conn.WriteJSON(modality.AudioClientEvent{
		Type: modality.AudioClientEventInputText,
		Text: "Hello from user",
	}); err != nil {
		t.Fatalf("WriteJSON(input_text) error = %v", err)
	}
	if err := conn.WriteJSON(modality.AudioClientEvent{
		Type: modality.AudioClientEventResponseCreate,
	}); err != nil {
		t.Fatalf("WriteJSON(response.create) error = %v", err)
	}

	seen := map[string]modality.AudioServerEvent{}
	for {
		event := readEvent()
		seen[event.Type] = event
		if event.Type == modality.AudioServerEventResponseCompleted {
			break
		}
	}

	if seen[modality.AudioServerEventResponseTextDelta].Text != "Hello from audio" {
		t.Fatalf("unexpected response text %#v", seen[modality.AudioServerEventResponseTextDelta])
	}
	audioEvent := seen[modality.AudioServerEventResponseAudioDelta]
	if audioEvent.Audio == "" {
		t.Fatalf("expected audio delta payload, got %#v", audioEvent)
	}
	if payload, err := base64.StdEncoding.DecodeString(audioEvent.Audio); err != nil || !bytes.Equal(payload, []byte{0x00, 0x01, 0x00, 0x02}) {
		t.Fatalf("unexpected audio payload %q err=%v", audioEvent.Audio, err)
	}
	completed := seen[modality.AudioServerEventResponseCompleted]
	if completed.Usage == nil || completed.Usage.TotalTokens != 18 {
		t.Fatalf("unexpected completed usage %#v", completed)
	}
	if completed.Usage.Source != modality.TokenCountSourceProviderReported {
		t.Fatalf("expected provider_reported audio usage source, got %#v", completed.Usage)
	}
	if atomic.LoadInt32(&chatCalls) != 1 || atomic.LoadInt32(&speechCalls) != 1 {
		t.Fatalf("expected one chat and one speech call, got chat=%d speech=%d", chatCalls, speechCalls)
	}

	if err := conn.WriteJSON(modality.AudioClientEvent{Type: modality.AudioClientEventSessionClose}); err != nil {
		t.Fatalf("WriteJSON(session.close) error = %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
	_ = conn.Close()

	var usage struct {
		TotalRequests int64 `json:"total_requests"`
		TotalTokens   int64 `json:"total_tokens"`
		ByModel       []struct {
			Model string `json:"model"`
		} `json:"by_model"`
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		usageReq, err := http.NewRequest(http.MethodGet, server.URL+"/v1/usage?group_by=model&modality=audio", nil)
		if err != nil {
			t.Fatalf("NewRequest(usage) error = %v", err)
		}
		usageReq.Header.Set("Authorization", "Bearer secret")
		usageResp, err := http.DefaultClient.Do(usageReq)
		if err != nil {
			t.Fatalf("Do(usage) error = %v", err)
		}
		if usageResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(usageResp.Body)
			_ = usageResp.Body.Close()
			t.Fatalf("expected usage 200, got %d body=%s", usageResp.StatusCode, string(body))
		}
		usage = struct {
			TotalRequests int64 `json:"total_requests"`
			TotalTokens   int64 `json:"total_tokens"`
			ByModel       []struct {
				Model string `json:"model"`
			} `json:"by_model"`
		}{}
		if err := json.NewDecoder(usageResp.Body).Decode(&usage); err != nil {
			_ = usageResp.Body.Close()
			t.Fatalf("decode usage response: %v", err)
		}
		_ = usageResp.Body.Close()
		if usage.TotalRequests == 1 && usage.TotalTokens == 18 && len(usage.ByModel) == 1 && usage.ByModel[0].Model == "openai/gpt-4o-audio" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for audio usage row, last report=%#v", usage)
		}
		time.Sleep(25 * time.Millisecond)
	}

	if err := requestLogger.Close(context.Background()); err != nil {
		t.Fatalf("requestLogger.Close() error = %v", err)
	}
	if usage.TotalRequests != 1 || usage.TotalTokens != 18 {
		t.Fatalf("unexpected usage response %#v", usage)
	}
	if len(usage.ByModel) != 1 || usage.ByModel[0].Model != "openai/gpt-4o-audio" {
		t.Fatalf("unexpected audio usage by_model %#v", usage.ByModel)
	}
}

func TestAudioSessionCreateRejectsUnsupportedTurnModeForByteDance(t *testing.T) {
	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"bytedance/*"},
	}}
	cfg.Providers = map[string]config.ProviderConfig{
		"bytedance": {
			APIKey:       "ark-key",
			AppID:        "app-id",
			SpeechAPIKey: "speech-key",
			BaseURL:      "https://ark.cn-beijing.volces.com/api/v3",
			Timeout:      time.Second,
			Models: map[string]config.ModelConfig{
				"doubao-pro-256k": {
					Modality:     modality.ModalityChat,
					Capabilities: []modality.Capability{modality.CapabilityStreaming},
					Endpoint:     "https://ark.cn-beijing.volces.com/api/v3",
				},
				"doubao-tts-2.0": {
					Modality:     modality.ModalityVoice,
					Capabilities: []modality.Capability{modality.CapabilityTTS},
					Voices:       []string{"zh_female_vv_uranus_bigtts"},
					Endpoint:     "https://openspeech.bytedance.com/api/v3/tts/unidirectional/sse",
				},
				"doubao-asr-2.0": {
					Modality:     modality.ModalityVoice,
					Capabilities: []modality.Capability{modality.CapabilitySTT},
					Formats:      []string{"wav"},
					Endpoint:     "https://openspeech.bytedance.com/api/v3/auc/bigmodel/recognize/flash",
				},
				"doubao-audio": {
					Modality:     modality.ModalityAudio,
					Capabilities: []modality.Capability{modality.CapabilityAudioInput, modality.CapabilityAudioOutput},
					Voices:       []string{"zh_female_vv_uranus_bigtts"},
					AudioPipeline: config.AudioPipelineConfig{
						ChatModel: "bytedance/doubao-pro-256k",
						STTModel:  "bytedance/doubao-asr-2.0",
						TTSModel:  "bytedance/doubao-tts-2.0",
					},
				},
			},
		},
	}
	cfg.Routing.Aliases = map[string]string{
		"default-audio": "bytedance/doubao-audio",
	}

	engine := newTestEngine(t, cfg)
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/sessions", strings.NewReader(`{
		"model":"default-audio",
		"voice":"zh_female_vv_uranus_bigtts",
		"turn_detection":{"mode":"server_vad"}
	}`))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"code":"unsupported_turn_detection"`) {
		t.Fatalf("expected unsupported_turn_detection error, got %s", res.Body.String())
	}
}

func TestAudioSessionCreateAcceptsServerVADForByteDanceRealtime(t *testing.T) {
	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"bytedance/*"},
	}}
	cfg.Providers = map[string]config.ProviderConfig{
		"bytedance": {
			AppID:             "app-123",
			SpeechAccessToken: "speech-token",
			SpeechAPIKey:      "speech-key",
			BaseURL:           "https://ark.cn-beijing.volces.com/api/v3",
			Timeout:           time.Second,
			Models: map[string]config.ModelConfig{
				"doubao-audio": {
					Modality:     modality.ModalityAudio,
					Capabilities: []modality.Capability{modality.CapabilityAudioInput, modality.CapabilityAudioOutput},
					Voices:       []string{"zh_female_vv_jupiter_bigtts"},
					RealtimeSession: config.AudioRealtimeConfig{
						Transport: "bytedance_dialog",
						Auth:      "access_token",
						Model:     "1.2.1.1",
					},
				},
			},
		},
	}
	cfg.Routing.Aliases = map[string]string{
		"default-audio": "bytedance/doubao-audio",
	}

	engine := newTestEngine(t, cfg)
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/sessions", strings.NewReader(`{
		"model":"default-audio",
		"voice":"zh_female_vv_jupiter_bigtts",
		"turn_detection":{"mode":"server_vad"}
	}`))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestEmbeddingsEndpointUsesResponseCache(t *testing.T) {
	var upstreamCalls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		atomic.AddInt32(&upstreamCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object":"list",
			"data":[{"object":"embedding","index":0,"embedding":[0.1,0.2]}],
			"model":"text-embedding-3-small",
			"usage":{"prompt_tokens":4,"total_tokens":4}
		}`))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"openai/*"},
	}}
	cfg.Cache.ResponseCache.Enabled = true
	cfg.Cache.ResponseCache.TTL = time.Hour
	cfg.Routing.Aliases = map[string]string{
		"default-embed": "openai/text-embedding-3-small",
	}
	cfg.Providers["openai"] = config.ProviderConfig{
		APIKey:  "sk-openai",
		BaseURL: upstream.URL + "/v1",
		Timeout: time.Second,
		Models: map[string]config.ModelConfig{
			"text-embedding-3-small": {
				Modality:   modality.ModalityEmbed,
				Dimensions: 1536,
			},
		},
	}

	engine := newTestEngine(t, cfg)
	body := `{"model":"default-embed","input":"cache me"}`
	for i, want := range []string{"miss", "hit"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer secret")
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d body=%s", i+1, res.Code, res.Body.String())
		}
		if got := res.Header().Get("X-Polaris-Cache"); got != want {
			t.Fatalf("request %d: expected X-Polaris-Cache=%s, got %q", i+1, want, got)
		}
	}
	if atomic.LoadInt32(&upstreamCalls) != 1 {
		t.Fatalf("expected one upstream call, got %d", upstreamCalls)
	}
}

func TestChatCompletionUsesSemanticResponseCache(t *testing.T) {
	var upstreamCalls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		atomic.AddInt32(&upstreamCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-cache",
			"object":"chat.completion",
			"created":1744329600,
			"model":"gpt-4o",
			"choices":[{"index":0,"message":{"role":"assistant","content":"Cached hello"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13}
		}`))
	}))
	defer upstream.Close()

	cfg := testConfigWithOpenAIBaseURL(t, upstream.URL+"/v1")
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "10/min",
		AllowedModels: []string{"openai/*"},
	}}
	cfg.Cache.ResponseCache.Enabled = true
	cfg.Cache.ResponseCache.TTL = time.Hour
	cfg.Cache.ResponseCache.SimilarityThreshold = 0.95
	cfg.Cache.ResponseCache.MaxEntriesPerModel = 10

	engine := newTestEngine(t, cfg)
	requests := []string{
		`{"model":"default-chat","messages":[{"role":"user","content":"Hello, Polaris!"}]}`,
		`{"model":"default-chat","messages":[{"role":"user","content":"hello polaris"}]}`,
	}
	for i, body := range requests {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer secret")
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d body=%s", i+1, res.Code, res.Body.String())
		}
		want := "miss"
		if i == 1 {
			want = "hit"
		}
		if got := res.Header().Get("X-Polaris-Cache"); got != want {
			t.Fatalf("request %d: expected X-Polaris-Cache=%s, got %q", i+1, want, got)
		}
	}
	if atomic.LoadInt32(&upstreamCalls) != 1 {
		t.Fatalf("expected one upstream call, got %d", upstreamCalls)
	}
}
