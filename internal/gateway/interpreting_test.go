package gateway

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	eventpb "github.com/JiaCheng2004/Polaris/internal/provider/bytedance/astpb/common/event"
	rpcmetapb "github.com/JiaCheng2004/Polaris/internal/provider/bytedance/astpb/common/rpcmeta"
	astpb "github.com/JiaCheng2004/Polaris/internal/provider/bytedance/astpb/products/understanding/ast"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func TestInterpretingSessionLifecycleByteDance(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Upgrade() error = %v", err)
		}
		defer func() {
			_ = conn.Close()
		}()

		start := expectGatewayInterpretingFrame(t, conn)
		if start.GetEvent() != eventpb.Type_StartSession {
			t.Fatalf("unexpected start event %#v", start)
		}
		writeGatewayInterpretingResponse(t, conn, &astpb.TranslateResponse{
			ResponseMeta: &rpcmetapb.ResponseMeta{
				SessionID:  start.GetRequestMeta().GetSessionID(),
				StatusCode: 20000000,
			},
			Event: eventpb.Type_SessionStarted,
		})

		audio := expectGatewayInterpretingFrame(t, conn)
		if audio.GetEvent() != eventpb.Type_TaskRequest {
			t.Fatalf("unexpected audio event %#v", audio)
		}
		if string(audio.GetSourceAudio().GetBinaryData()) != string([]byte("wavdata")) {
			t.Fatalf("unexpected audio payload %q", string(audio.GetSourceAudio().GetBinaryData()))
		}

		finish := expectGatewayInterpretingFrame(t, conn)
		if finish.GetEvent() != eventpb.Type_FinishSession {
			t.Fatalf("unexpected finish event %#v", finish)
		}

		writeGatewayInterpretingResponse(t, conn, &astpb.TranslateResponse{
			ResponseMeta: &rpcmetapb.ResponseMeta{
				SessionID:  start.GetRequestMeta().GetSessionID(),
				StatusCode: 20000000,
			},
			Event:     eventpb.Type_TranslationSubtitleResponse,
			Text:      "hello world",
			StartTime: 0,
			EndTime:   900,
		})
		writeGatewayInterpretingResponse(t, conn, &astpb.TranslateResponse{
			ResponseMeta: &rpcmetapb.ResponseMeta{
				SessionID:  start.GetRequestMeta().GetSessionID(),
				StatusCode: 20000000,
				Billing: &rpcmetapb.Billing{
					DurationMsec: 900,
					Items: []*rpcmetapb.BillingItem{
						{Unit: "input_audio_tokens", Quantity: 1},
						{Unit: "output_text_tokens", Quantity: 1},
					},
				},
			},
			Event:     eventpb.Type_TranslationSubtitleEnd,
			Text:      "hello world",
			StartTime: 0,
			EndTime:   900,
		})
		writeGatewayInterpretingResponse(t, conn, &astpb.TranslateResponse{
			ResponseMeta: &rpcmetapb.ResponseMeta{
				SessionID:  start.GetRequestMeta().GetSessionID(),
				StatusCode: 20000000,
				Billing: &rpcmetapb.Billing{
					DurationMsec: 900,
					Items: []*rpcmetapb.BillingItem{
						{Unit: "input_audio_tokens", Quantity: 1},
						{Unit: "output_text_tokens", Quantity: 1},
					},
				},
			},
			Event: eventpb.Type_UsageResponse,
		})
		writeGatewayInterpretingResponse(t, conn, &astpb.TranslateResponse{
			ResponseMeta: &rpcmetapb.ResponseMeta{
				SessionID:  start.GetRequestMeta().GetSessionID(),
				StatusCode: 20000000,
			},
			Event: eventpb.Type_SessionFinished,
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "100/min",
		AllowedModels: []string{"bytedance/*"},
	}}
	cfg.Providers = map[string]config.ProviderConfig{
		"bytedance": {
			AppID:             "app-123",
			SpeechAccessToken: "speech-token",
			SpeechAPIKey:      "speech-api-key",
			Timeout:           time.Second,
			Models: map[string]config.ModelConfig{
				"doubao-interpreting-2.0": {
					Modality:     modality.ModalityInterpreting,
					Capabilities: []modality.Capability{modality.CapabilityAudioInput, modality.CapabilityAudioOutput},
					Endpoint:     "ws" + strings.TrimPrefix(upstream.URL, "http"),
					SessionTTL:   2 * time.Minute,
				},
			},
		},
	}
	cfg.Routing.Aliases = map[string]string{
		"bytedance-interpreting": "bytedance/doubao-interpreting-2.0",
	}

	registry, warnings, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no registry warnings, got %v", warnings)
	}

	engine, err := NewEngine(Dependencies{
		Config:   cfg,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    testSQLiteStore(t),
		Cache:    cache.NewMemory(),
		Registry: registry,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	server := httptest.NewServer(engine)
	defer server.Close()

	createReq, err := http.NewRequest(http.MethodPost, server.URL+"/v1/audio/interpreting/sessions", strings.NewReader(`{
		"model":"bytedance-interpreting",
		"mode":"speech_to_text",
		"source_language":"zh",
		"target_language":"en",
		"input_audio_format":"wav",
		"input_sample_rate_hz":16000
	}`))
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	createReq.Header.Set("Authorization", "Bearer secret")
	createReq.Header.Set("Content-Type", "application/json")
	createRes, err := server.Client().Do(createReq)
	if err != nil {
		t.Fatalf("server.Client().Do() error = %v", err)
	}
	defer func() {
		_ = createRes.Body.Close()
	}()
	if createRes.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(createRes.Body)
		t.Fatalf("expected 200, got %d body=%s", createRes.StatusCode, string(body))
	}

	var session modality.InterpretingSessionDescriptor
	if err := json.NewDecoder(createRes.Body).Decode(&session); err != nil {
		t.Fatalf("Decode(session) error = %v", err)
	}
	if session.ID == "" || session.ClientSecret == "" {
		t.Fatalf("unexpected session descriptor %#v", session)
	}

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+session.ClientSecret)
	wsURL := strings.Replace(session.WebSocketURL, "http://", "ws://", 1)
	if parsed, err := url.Parse(wsURL); err == nil && parsed.Host == "example.com" {
		wsURL = strings.Replace(server.URL, "http://", "ws://", 1) + parsed.Path
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	var created modality.InterpretingServerEvent
	if err := conn.ReadJSON(&created); err != nil {
		t.Fatalf("ReadJSON(session.created) error = %v", err)
	}
	if created.Type != modality.InterpretingServerEventSessionCreated {
		t.Fatalf("unexpected created event %#v", created)
	}

	if err := conn.WriteJSON(modality.InterpretingClientEvent{
		Type:  modality.InterpretingClientEventInputAudioAppend,
		Audio: base64.StdEncoding.EncodeToString([]byte("wavdata")),
	}); err != nil {
		t.Fatalf("WriteJSON(input_audio.append) error = %v", err)
	}
	if err := conn.WriteJSON(modality.InterpretingClientEvent{
		Type: modality.InterpretingClientEventInputAudioCommit,
	}); err != nil {
		t.Fatalf("WriteJSON(input_audio.commit) error = %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var completed modality.InterpretingServerEvent
	for time.Now().Before(deadline) {
		if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
			t.Fatalf("SetReadDeadline() error = %v", err)
		}
		var event modality.InterpretingServerEvent
		if err := conn.ReadJSON(&event); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				break
			}
			continue
		}
		if event.Type == modality.InterpretingServerEventResponseCompleted {
			completed = event
			break
		}
	}
	if completed.Usage == nil || completed.Usage.TotalTokens != 2 {
		t.Fatalf("unexpected completed event %#v", completed)
	}
}

func expectGatewayInterpretingFrame(t *testing.T, conn *websocket.Conn) *astpb.TranslateRequest {
	t.Helper()
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	request := &astpb.TranslateRequest{}
	if err := proto.Unmarshal(payload, request); err != nil {
		t.Fatalf("proto.Unmarshal() error = %v", err)
	}
	return request
}

func writeGatewayInterpretingResponse(t *testing.T, conn *websocket.Conn, response *astpb.TranslateResponse) {
	t.Helper()
	payload, err := proto.Marshal(response)
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}
}
