package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestCreateAndDialInterpretingSession(t *testing.T) {
	var server *httptest.Server
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/audio/interpreting/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected auth header %q", got)
		}
		var req InterpretingSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "default-interpreting" || req.Mode != "speech_to_text" || req.SourceLanguage != "zh" || req.TargetLanguage != "en" {
			t.Fatalf("unexpected request %#v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"intsess_123",
			"object":"audio.interpreting.session",
			"model":"bytedance/doubao-interpreting-2.0",
			"expires_at":1712699999,
			"websocket_url":"` + strings.Replace(server.URL, "http://", "ws://", 1) + `/v1/audio/interpreting/sessions/intsess_123/ws",
			"client_secret":"intsec_123"
		}`))
	})

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux.HandleFunc("/v1/audio/interpreting/sessions/intsess_123/ws", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer intsec_123" {
			t.Fatalf("unexpected websocket auth header %q", got)
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Upgrade() error = %v", err)
		}
		defer func() {
			_ = conn.Close()
		}()

		if err := conn.WriteJSON(InterpretingEvent{Type: "session.created"}); err != nil {
			t.Fatalf("WriteJSON(session.created) error = %v", err)
		}

		var event InterpretingClientEvent
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("ReadJSON() error = %v", err)
		}
		if event.Type != "input_audio.commit" {
			t.Fatalf("unexpected client event %#v", event)
		}

		if err := conn.WriteJSON(InterpretingEvent{
			Type: "response.completed",
			Usage: &InterpretingUsage{
				TotalTokens: 2,
				Source:      "provider_reported",
			},
		}); err != nil {
			t.Fatalf("WriteJSON(response.completed) error = %v", err)
		}
	})

	server = httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL, WithAPIKey("secret"))
	session, err := client.CreateInterpretingSession(context.Background(), &InterpretingSessionRequest{
		Model:          "default-interpreting",
		Mode:           "speech_to_text",
		SourceLanguage: "zh",
		TargetLanguage: "en",
	})
	if err != nil {
		t.Fatalf("CreateInterpretingSession() error = %v", err)
	}
	if session.ID != "intsess_123" || session.ClientSecret != "intsec_123" {
		t.Fatalf("unexpected session %#v", session)
	}

	conn, err := client.DialInterpretingSession(context.Background(), session)
	if err != nil {
		t.Fatalf("DialInterpretingSession() error = %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	created, err := conn.Receive()
	if err != nil {
		t.Fatalf("Receive(session.created) error = %v", err)
	}
	if created.Type != "session.created" {
		t.Fatalf("unexpected created event %#v", created)
	}

	if err := conn.Send(&InterpretingClientEvent{Type: "input_audio.commit"}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if err := conn.conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	completed, err := conn.Receive()
	if err != nil {
		t.Fatalf("Receive(response.completed) error = %v", err)
	}
	if completed.Type != "response.completed" || completed.Usage == nil || completed.Usage.TotalTokens != 2 {
		t.Fatalf("unexpected completed event %#v", completed)
	}
}
