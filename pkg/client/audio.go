package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

// AudioSessionConn is a realtime WebSocket connection for audio session.
type AudioSessionConn struct {
	conn *websocket.Conn
}

// CreateAudioSession creates an audio session.
func (c *Client) CreateAudioSession(ctx context.Context, req *AudioSessionRequest) (*AudioSession, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	var response AudioSession
	if err := c.doJSON(ctx, http.MethodPost, "/v1/audio/sessions", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// DialAudioSession dials an audio session.
func (c *Client) DialAudioSession(ctx context.Context, session *AudioSession) (*AudioSessionConn, error) {
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
		target = c.defaultAudioWebSocketURL(session.ID)
	}

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+session.ClientSecret)
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, target, headers)
	if err != nil {
		return nil, fmt.Errorf("dial audio session: %w", err)
	}
	return &AudioSessionConn{conn: conn}, nil
}

// Send sends a message on the connection.
func (s *AudioSessionConn) Send(event *AudioClientEvent) error {
	if s == nil || s.conn == nil {
		return fmt.Errorf("audio session connection is not initialized")
	}
	if event == nil {
		return fmt.Errorf("event is required")
	}
	return s.conn.WriteJSON(event)
}

// Receive receives the next message from the connection.
func (s *AudioSessionConn) Receive() (*AudioServerEvent, error) {
	if s == nil || s.conn == nil {
		return nil, fmt.Errorf("audio session connection is not initialized")
	}
	var event AudioServerEvent
	if err := s.conn.ReadJSON(&event); err != nil {
		return nil, fmt.Errorf("read audio session event: %w", err)
	}
	return &event, nil
}

// Close closes the underlying connection.
func (s *AudioSessionConn) Close() error {
	if s == nil || s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

func (c *Client) defaultAudioWebSocketURL(sessionID string) string {
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
	parsed.Path = "/v1/audio/sessions/" + sessionID + "/ws"
	parsed.RawQuery = ""
	return parsed.String()
}
