package bytedance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gorilla/websocket"
)

func TestEncodeDialogStartConnectionMatchesOfficialExample(t *testing.T) {
	frame, err := encodeDialogJSONFrame(dialogMessageTypeFullClient, dialogEventStartConnection, "", "", map[string]any{})
	if err != nil {
		t.Fatalf("encodeDialogJSONFrame() error = %v", err)
	}
	want := []byte{17, 20, 16, 0, 0, 0, 0, 1, 0, 0, 0, 2, 123, 125}
	if string(frame) != string(want) {
		t.Fatalf("unexpected start connection frame %v want %v", frame, want)
	}
}

func TestRealtimeAudioAdapterTextTurn(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Api-App-ID"); got != "app-123" {
			t.Fatalf("unexpected X-Api-App-ID %q", got)
		}
		if got := r.Header.Get("X-Api-Access-Key"); got != "speech-token" {
			t.Fatalf("unexpected X-Api-Access-Key %q", got)
		}
		if got := r.Header.Get("X-Api-Resource-Id"); got != defaultRealtimeDialogueResourceID {
			t.Fatalf("unexpected X-Api-Resource-Id %q", got)
		}
		if got := r.Header.Get("X-Api-App-Key"); got != defaultRealtimeDialogueAppKey {
			t.Fatalf("unexpected X-Api-App-Key %q", got)
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Upgrade() error = %v", err)
		}
		defer func() {
			_ = conn.Close()
		}()

		expectJSONFrame(t, conn, dialogMessageTypeFullClient, dialogEventStartConnection, "", func(payload []byte) {
			if string(payload) != "{}" {
				t.Fatalf("unexpected start connection payload %q", string(payload))
			}
		})
		startSession := expectJSONFrame(t, conn, dialogMessageTypeFullClient, dialogEventStartSession, "present", func(payload []byte) {
			var data map[string]any
			if err := json.Unmarshal(payload, &data); err != nil {
				t.Fatalf("unmarshal StartSession payload: %v", err)
			}
			dialog := data["dialog"].(map[string]any)
			extra := dialog["extra"].(map[string]any)
			if extra["model"] != "1.2.1.1" {
				t.Fatalf("unexpected realtime model %#v", extra)
			}
			if extra["input_mod"] != "push_to_talk" {
				t.Fatalf("unexpected input_mod %#v", extra)
			}
			tts := data["tts"].(map[string]any)
			if tts["speaker"] != "zh_female_vv_jupiter_bigtts" {
				t.Fatalf("unexpected speaker %#v", tts)
			}
		})

		writeServerJSONFrame(t, conn, dialogEventConnectionStarted, "", map[string]any{})
		writeServerJSONFrame(t, conn, dialogEventSessionStarted, startSession.SessionID, map[string]any{"dialog_id": "dlg_123"})

		expectJSONFrame(t, conn, dialogMessageTypeFullClient, dialogEventChatTextQuery, startSession.SessionID, func(payload []byte) {
			var data map[string]any
			if err := json.Unmarshal(payload, &data); err != nil {
				t.Fatalf("unmarshal ChatTextQuery payload: %v", err)
			}
			if data["content"] != "Reply with BYTEDANCE_AUDIO_OK only." {
				t.Fatalf("unexpected text query %#v", data)
			}
		})

		writeServerJSONFrame(t, conn, dialogEventChatResponse, startSession.SessionID, map[string]any{
			"content":     "BYTEDANCE_AUDIO_OK",
			"question_id": "q1",
			"reply_id":    "r1",
		})
		writeServerJSONFrame(t, conn, dialogEventChatEnded, startSession.SessionID, map[string]any{
			"question_id": "q1",
			"reply_id":    "r1",
		})
		writeServerAudioFrame(t, conn, dialogEventTTSResponse, startSession.SessionID, []byte{0x00, 0x00, 0x10, 0x00, 0x20, 0x00})
		writeServerJSONFrame(t, conn, dialogEventUsageResponse, startSession.SessionID, map[string]any{
			"usage": map[string]any{
				"input_text_tokens":   11,
				"output_text_tokens":  7,
				"input_audio_tokens":  0,
				"output_audio_tokens": 12,
			},
		})
		writeServerJSONFrame(t, conn, dialogEventTTSEnded, startSession.SessionID, map[string]any{
			"question_id": "q1",
			"reply_id":    "r1",
		})
	}))
	defer server.Close()

	session := newRealtimeTestSession(t, server.URL, modality.TurnDetectionManual)
	defer func() {
		_ = session.Close()
	}()

	if err := session.Send(modality.AudioClientEvent{Type: modality.AudioClientEventInputText, Text: "Reply with BYTEDANCE_AUDIO_OK only."}); err != nil {
		t.Fatalf("Send(input_text) error = %v", err)
	}
	if err := session.Send(modality.AudioClientEvent{Type: modality.AudioClientEventResponseCreate}); err != nil {
		t.Fatalf("Send(response.create) error = %v", err)
	}

	events := collectAudioEventsUntilCompleted(t, session.Events())
	if events[modality.AudioServerEventResponseTextDelta].Text != "BYTEDANCE_AUDIO_OK" {
		t.Fatalf("unexpected response text %#v", events[modality.AudioServerEventResponseTextDelta])
	}
	audioDelta := events[modality.AudioServerEventResponseAudioDelta]
	audio, err := base64.StdEncoding.DecodeString(audioDelta.Audio)
	if err != nil {
		t.Fatalf("DecodeString(audio delta) error = %v", err)
	}
	if len(audio) != 4 {
		t.Fatalf("expected 16k resampled audio payload, got %d bytes", len(audio))
	}
	completed := events[modality.AudioServerEventResponseCompleted]
	if completed.Usage == nil || completed.Usage.TotalTokens != 18 || completed.Usage.Source != modality.TokenCountSourceProviderReported {
		t.Fatalf("unexpected completed usage %#v", completed)
	}
}

func TestRealtimeAudioAdapterManualAudioTurn(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Upgrade() error = %v", err)
		}
		defer func() {
			_ = conn.Close()
		}()

		expectJSONFrame(t, conn, dialogMessageTypeFullClient, dialogEventStartConnection, "", nil)
		startSession := expectJSONFrame(t, conn, dialogMessageTypeFullClient, dialogEventStartSession, "present", nil)
		writeServerJSONFrame(t, conn, dialogEventConnectionStarted, "", map[string]any{})
		writeServerJSONFrame(t, conn, dialogEventSessionStarted, startSession.SessionID, map[string]any{"dialog_id": "dlg_audio"})

		audioFrame := expectAudioFrame(t, conn, dialogEventTaskRequest, startSession.SessionID)
		if string(audioFrame.Payload) != string([]byte{0x01, 0x02, 0x03, 0x04}) {
			t.Fatalf("unexpected audio payload %v", audioFrame.Payload)
		}
		expectJSONFrame(t, conn, dialogMessageTypeFullClient, dialogEventEndASR, startSession.SessionID, nil)

		writeServerJSONFrame(t, conn, dialogEventASRResponse, startSession.SessionID, map[string]any{
			"results": []map[string]any{{"text": "Hello", "is_interim": false}},
		})
		writeServerJSONFrame(t, conn, dialogEventASREnded, startSession.SessionID, map[string]any{})
		writeServerJSONFrame(t, conn, dialogEventChatResponse, startSession.SessionID, map[string]any{
			"content":     "Hello, nice to meet you",
			"question_id": "q2",
			"reply_id":    "r2",
		})
		writeServerJSONFrame(t, conn, dialogEventChatEnded, startSession.SessionID, map[string]any{
			"question_id": "q2",
			"reply_id":    "r2",
		})
		writeServerAudioFrame(t, conn, dialogEventTTSResponse, startSession.SessionID, []byte{0x00, 0x00, 0x20, 0x00, 0x40, 0x00})
		writeServerJSONFrame(t, conn, dialogEventUsageResponse, startSession.SessionID, map[string]any{
			"usage": map[string]any{
				"input_text_tokens":  5,
				"output_text_tokens": 4,
			},
		})
		writeServerJSONFrame(t, conn, dialogEventTTSEnded, startSession.SessionID, map[string]any{
			"question_id": "q2",
			"reply_id":    "r2",
		})
	}))
	defer server.Close()

	session := newRealtimeTestSession(t, server.URL, modality.TurnDetectionManual)
	defer func() {
		_ = session.Close()
	}()

	if err := session.Send(modality.AudioClientEvent{
		Type:  modality.AudioClientEventInputAudioAppend,
		Audio: base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03, 0x04}),
	}); err != nil {
		t.Fatalf("Send(input_audio.append) error = %v", err)
	}
	if err := session.Send(modality.AudioClientEvent{Type: modality.AudioClientEventInputAudioCommit}); err != nil {
		t.Fatalf("Send(input_audio.commit) error = %v", err)
	}

	events := collectAudioEventsUntilCompleted(t, session.Events())
	committed := events[modality.AudioServerEventInputAudioCommitted]
	if committed.Transcript != "Hello" {
		t.Fatalf("unexpected committed transcript %#v", committed)
	}
	completed := events[modality.AudioServerEventResponseCompleted]
	if completed.Usage == nil || completed.Usage.TotalTokens != 9 {
		t.Fatalf("unexpected completed usage %#v", completed)
	}
}

func newRealtimeTestSession(t *testing.T, serverURL string, mode string) modality.AudioSession {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http")
	client := NewClient(config.ProviderConfig{
		AppID:             "app-123",
		SpeechAccessToken: "speech-token",
		SpeechAPIKey:      "speech-api-key",
		Timeout:           time.Second,
	})
	adapter := newRealtimeAudioAdapter(client, "bytedance/doubao-audio", config.ModelConfig{
		Modality: modality.ModalityAudio,
		RealtimeSession: config.AudioRealtimeConfig{
			Transport: "bytedance_dialog",
			URL:       wsURL,
			Model:     "1.2.1.1",
		},
		SessionTTL: 2 * time.Minute,
	}).(*realtimeAudioAdapter)
	session, err := adapter.Connect(context.Background(), &modality.AudioSessionConfig{
		Model: "bytedance/doubao-audio",
		Voice: "zh_female_vv_jupiter_bigtts",
		TurnDetection: &modality.TurnDetectionConfig{
			Mode: mode,
		},
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	return session
}

func TestRealtimeAudioAdapterAPIKeyAuth(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Api-Key"); got != "speech-api-key" {
			t.Fatalf("unexpected X-Api-Key %q", got)
		}
		if got := r.Header.Get("X-Api-App-ID"); got != "" {
			t.Fatalf("expected no X-Api-App-ID, got %q", got)
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Upgrade() error = %v", err)
		}
		_ = conn.Close()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := NewClient(config.ProviderConfig{
		SpeechAPIKey: "speech-api-key",
		Timeout:      time.Second,
	})
	adapter := newRealtimeAudioAdapter(client, "bytedance/doubao-audio", config.ModelConfig{
		Modality: modality.ModalityAudio,
		RealtimeSession: config.AudioRealtimeConfig{
			Transport: "bytedance_dialog",
			Auth:      "api_key",
			URL:       wsURL,
			Model:     "1.2.1.1",
		},
	}).(*realtimeAudioAdapter)
	session, err := adapter.Connect(context.Background(), &modality.AudioSessionConfig{
		Model: "bytedance/doubao-audio",
		Voice: "zh_female_vv_jupiter_bigtts",
		TurnDetection: &modality.TurnDetectionConfig{
			Mode: modality.TurnDetectionManual,
		},
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer func() {
		_ = session.Close()
	}()

	if err := session.Send(modality.AudioClientEvent{Type: modality.AudioClientEventInputText, Text: "ping"}); err != nil {
		t.Fatalf("Send(input_text) error = %v", err)
	}
}

func collectAudioEventsUntilCompleted(t *testing.T, events <-chan modality.AudioServerEvent) map[string]modality.AudioServerEvent {
	t.Helper()
	deadline := time.After(3 * time.Second)
	seen := map[string]modality.AudioServerEvent{}
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for completed realtime audio turn")
		case event := <-events:
			seen[event.Type] = event
			if event.Type == modality.AudioServerEventResponseCompleted {
				return seen
			}
		}
	}
}

func expectJSONFrame(t *testing.T, conn *websocket.Conn, messageType byte, eventID uint32, sessionExpectation string, assertPayload func([]byte)) dialogFrame {
	t.Helper()
	frame := readDialogFrame(t, conn)
	if frame.MessageType != messageType {
		t.Fatalf("unexpected message type %d", frame.MessageType)
	}
	if frame.Event == nil || *frame.Event != eventID {
		t.Fatalf("unexpected event %#v", frame.Event)
	}
	switch sessionExpectation {
	case "":
		if frame.SessionID != "" {
			t.Fatalf("expected no session id, got %q", frame.SessionID)
		}
	case "present":
		if strings.TrimSpace(frame.SessionID) == "" {
			t.Fatalf("expected session id, got %#v", frame)
		}
	default:
		if frame.SessionID != sessionExpectation {
			t.Fatalf("expected session id %q, got %q", sessionExpectation, frame.SessionID)
		}
	}
	if assertPayload != nil {
		assertPayload(frame.Payload)
	}
	return frame
}

func expectAudioFrame(t *testing.T, conn *websocket.Conn, eventID uint32, sessionID string) dialogFrame {
	t.Helper()
	frame := readDialogFrame(t, conn)
	if frame.MessageType != dialogMessageTypeAudioClient {
		t.Fatalf("unexpected audio message type %d", frame.MessageType)
	}
	if frame.Event == nil || *frame.Event != eventID {
		t.Fatalf("unexpected audio event %#v", frame.Event)
	}
	if frame.SessionID != sessionID {
		t.Fatalf("unexpected session id %q, want %q", frame.SessionID, sessionID)
	}
	return frame
}

func readDialogFrame(t *testing.T, conn *websocket.Conn) dialogFrame {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	messageType, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if messageType != websocket.BinaryMessage {
		t.Fatalf("unexpected websocket message type %d", messageType)
	}
	frame, err := decodeDialogFrame(payload)
	if err != nil {
		t.Fatalf("decodeDialogFrame() error = %v", err)
	}
	return frame
}

func writeServerJSONFrame(t *testing.T, conn *websocket.Conn, eventID uint32, sessionID string, payload any) {
	t.Helper()
	frame, err := encodeDialogJSONFrame(dialogMessageTypeFullServer, eventID, sessionID, "", payload)
	if err != nil {
		t.Fatalf("encodeDialogJSONFrame() error = %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		t.Fatalf("WriteMessage(JSON) error = %v", err)
	}
}

func writeServerAudioFrame(t *testing.T, conn *websocket.Conn, eventID uint32, sessionID string, payload []byte) {
	t.Helper()
	frame, err := encodeDialogFrame(dialogFrame{
		MessageType:   dialogMessageTypeAudioServer,
		Flags:         dialogFlagEventPresent,
		Serialization: dialogSerializationRaw,
		Compression:   dialogCompressionNone,
		Event:         &eventID,
		SessionID:     sessionID,
		Payload:       payload,
	})
	if err != nil {
		t.Fatalf("encodeDialogFrame(audio) error = %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		t.Fatalf("WriteMessage(audio) error = %v", err)
	}
}
