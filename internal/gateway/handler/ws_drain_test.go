package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/drain"
	"github.com/gorilla/websocket"
)

// TestWSDrainSendsCloseFrame proves R6: on graceful shutdown, a live WebSocket
// receives a close frame and its server-side read loop breaks.
func TestWSDrainSendsCloseFrame(t *testing.T) {
	drainer := drain.NewRegistry()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	serverReturned := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		dereg := registerWSDrain(drainer, conn)
		defer dereg()
		defer close(serverReturned)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Wait for the server to register the connection.
	deadline := time.Now().Add(2 * time.Second)
	for drainer.Count() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if drainer.Count() != 1 {
		t.Fatalf("connection not registered (count=%d)", drainer.Count())
	}

	go drainer.Drain(context.Background())

	// The client should observe a CloseServiceRestart close frame.
	gotClose := false
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		if _, _, err := client.ReadMessage(); err != nil {
			if ce, ok := err.(*websocket.CloseError); ok && ce.Code == websocket.CloseServiceRestart {
				gotClose = true
			}
			break
		}
	}
	if !gotClose {
		t.Fatal("client did not receive a CloseServiceRestart frame on drain")
	}

	select {
	case <-serverReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("server read loop did not break after drain")
	}
}
