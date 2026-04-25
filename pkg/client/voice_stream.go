package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

type StreamingTranscriptionConn struct {
	conn *websocket.Conn
}

func (c *Client) CreateStreamingTranscriptionSession(ctx context.Context, req *StreamingTranscriptionSessionRequest) (*StreamingTranscriptionSession, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	var response StreamingTranscriptionSession
	if err := c.doJSON(ctx, http.MethodPost, "/v1/audio/transcriptions/stream", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) DialStreamingTranscriptionSession(ctx context.Context, session *StreamingTranscriptionSession) (*StreamingTranscriptionConn, error) {
	if session == nil {
		return nil, fmt.Errorf("session is required")
	}
	if strings.TrimSpace(session.ClientSecret) == "" {
		return nil, fmt.Errorf("session client secret is required")
	}

	target := strings.TrimSpace(session.WebSocketURL)
	if target == "" {
		if strings.TrimSpace(session.ID) == "" {
			return nil, fmt.Errorf("session id is required when websocket_url is absent")
		}
		target = c.defaultStreamingTranscriptionWebSocketURL(session.ID)
	}

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+session.ClientSecret)
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, target, headers)
	if err != nil {
		return nil, fmt.Errorf("dial streaming transcription session: %w", err)
	}
	return &StreamingTranscriptionConn{conn: conn}, nil
}

func (s *StreamingTranscriptionConn) Send(event *StreamingTranscriptionClientEvent) error {
	if s == nil || s.conn == nil {
		return fmt.Errorf("streaming transcription connection is not initialized")
	}
	if event == nil {
		return fmt.Errorf("event is required")
	}
	return s.conn.WriteJSON(event)
}

func (s *StreamingTranscriptionConn) Receive() (*StreamingTranscriptionEvent, error) {
	if s == nil || s.conn == nil {
		return nil, fmt.Errorf("streaming transcription connection is not initialized")
	}
	var event StreamingTranscriptionEvent
	if err := s.conn.ReadJSON(&event); err != nil {
		return nil, fmt.Errorf("read streaming transcription event: %w", err)
	}
	return &event, nil
}

func (s *StreamingTranscriptionConn) Close() error {
	if s == nil || s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

func (c *Client) defaultStreamingTranscriptionWebSocketURL(sessionID string) string {
	parsed, err := url.Parse(c.baseURL)
	if err != nil {
		return ""
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	default:
		parsed.Scheme = "ws"
	}
	parsed.Path = "/v1/audio/transcriptions/stream/" + sessionID + "/ws"
	parsed.RawQuery = ""
	return parsed.String()
}
