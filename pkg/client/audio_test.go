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

func TestCreateAndDialAudioSession(t *testing.T) {
	var server *httptest.Server
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/audio/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected auth header %q", got)
		}
		var req AudioSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "default-audio" || req.Voice != "nova" || req.TurnDetection == nil || req.TurnDetection.Mode != "manual" {
			t.Fatalf("unexpected request %#v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"audsess_123",
			"object":"audio.session",
			"model":"openai/gpt-4o-audio",
			"expires_at":1712699999,
			"websocket_url":"` + strings.Replace(server.URL, "http://", "ws://", 1) + `/v1/audio/sessions/audsess_123/ws",
			"client_secret":"audsec_123"
		}`))
	})

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux.HandleFunc("/v1/audio/sessions/audsess_123/ws", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer audsec_123" {
			t.Fatalf("unexpected websocket auth header %q", got)
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Upgrade() error = %v", err)
		}
		defer func() {
			_ = conn.Close()
		}()

		if err := conn.WriteJSON(AudioServerEvent{Type: "session.created"}); err != nil {
			t.Fatalf("WriteJSON(session.created) error = %v", err)
		}

		var event AudioClientEvent
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("ReadJSON() error = %v", err)
		}
		if event.Type != "input_text" || event.Text != "hello" {
			t.Fatalf("unexpected client event %#v", event)
		}

		if err := conn.WriteJSON(AudioServerEvent{Type: "response.completed", Usage: &AudioUsage{TotalTokens: 18, Source: "provider_reported"}}); err != nil {
			t.Fatalf("WriteJSON(response.completed) error = %v", err)
		}
	})

	server = httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL, WithAPIKey("secret"))

	session, err := client.CreateAudioSession(context.Background(), &AudioSessionRequest{
		Model: "default-audio",
		Voice: "nova",
		TurnDetection: &AudioTurnDetection{
			Mode: "manual",
		},
	})
	if err != nil {
		t.Fatalf("CreateAudioSession() error = %v", err)
	}
	if session.ID != "audsess_123" || session.ClientSecret != "audsec_123" {
		t.Fatalf("unexpected session %#v", session)
	}

	conn, err := client.DialAudioSession(context.Background(), session)
	if err != nil {
		t.Fatalf("DialAudioSession() error = %v", err)
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

	if err := conn.Send(&AudioClientEvent{Type: "input_text", Text: "hello"}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if err := conn.conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	completed, err := conn.Receive()
	if err != nil {
		t.Fatalf("Receive(response.completed) error = %v", err)
	}
	if completed.Type != "response.completed" {
		t.Fatalf("unexpected completed event %#v", completed)
	}
	if completed.Usage == nil || completed.Usage.Source != "provider_reported" {
		t.Fatalf("unexpected completed usage %#v", completed)
	}
}
