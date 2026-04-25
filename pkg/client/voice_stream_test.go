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

func TestCreateAndDialStreamingTranscriptionSession(t *testing.T) {
	var server *httptest.Server
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/audio/transcriptions/stream", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected auth header %q", got)
		}
		var req StreamingTranscriptionSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "default-streaming-asr" || req.SampleRateHz != 16000 || req.InterimResults == nil || !*req.InterimResults {
			t.Fatalf("unexpected request %#v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"sttsess_123",
			"object":"audio.transcription.session",
			"model":"bytedance/doubao-streaming-asr-2.0",
			"expires_at":1712699999,
			"websocket_url":"` + strings.Replace(server.URL, "http://", "ws://", 1) + `/v1/audio/transcriptions/stream/sttsess_123/ws",
			"client_secret":"sttsec_123"
		}`))
	})

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux.HandleFunc("/v1/audio/transcriptions/stream/sttsess_123/ws", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sttsec_123" {
			t.Fatalf("unexpected websocket auth header %q", got)
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Upgrade() error = %v", err)
		}
		defer func() {
			_ = conn.Close()
		}()

		if err := conn.WriteJSON(StreamingTranscriptionEvent{Type: "session.created"}); err != nil {
			t.Fatalf("WriteJSON(session.created) error = %v", err)
		}

		var event StreamingTranscriptionClientEvent
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("ReadJSON() error = %v", err)
		}
		if event.Type != "input_audio.commit" {
			t.Fatalf("unexpected client event %#v", event)
		}

		if err := conn.WriteJSON(StreamingTranscriptionEvent{
			Type: "transcript.completed",
			Transcript: &TranscriptionResponse{
				Text: "Hello, ByteDance",
			},
		}); err != nil {
			t.Fatalf("WriteJSON(transcript.completed) error = %v", err)
		}
	})

	server = httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL, WithAPIKey("secret"))
	session, err := client.CreateStreamingTranscriptionSession(context.Background(), &StreamingTranscriptionSessionRequest{
		Model:            "default-streaming-asr",
		SampleRateHz:     16000,
		InterimResults:   boolPtr(true),
		ReturnUtterances: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("CreateStreamingTranscriptionSession() error = %v", err)
	}
	if session.ID != "sttsess_123" || session.ClientSecret != "sttsec_123" {
		t.Fatalf("unexpected session %#v", session)
	}

	conn, err := client.DialStreamingTranscriptionSession(context.Background(), session)
	if err != nil {
		t.Fatalf("DialStreamingTranscriptionSession() error = %v", err)
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

	if err := conn.Send(&StreamingTranscriptionClientEvent{Type: "input_audio.commit"}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if err := conn.conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	completed, err := conn.Receive()
	if err != nil {
		t.Fatalf("Receive(transcript.completed) error = %v", err)
	}
	if completed.Type != "transcript.completed" || completed.Transcript == nil || completed.Transcript.Text != "Hello, ByteDance" {
		t.Fatalf("unexpected completed event %#v", completed)
	}
}
