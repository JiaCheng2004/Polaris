package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

type InterpretingConn struct {
	conn *websocket.Conn
}

func (c *Client) CreateInterpretingSession(ctx context.Context, req *InterpretingSessionRequest) (*InterpretingSession, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	var response InterpretingSession
	if err := c.doJSON(ctx, http.MethodPost, "/v1/audio/interpreting/sessions", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) DialInterpretingSession(ctx context.Context, session *InterpretingSession) (*InterpretingConn, error) {
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
		target = c.defaultInterpretingWebSocketURL(session.ID)
	}

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+session.ClientSecret)
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, target, headers)
	if err != nil {
		return nil, fmt.Errorf("dial interpreting session: %w", err)
	}
	return &InterpretingConn{conn: conn}, nil
}

func (s *InterpretingConn) Send(event *InterpretingClientEvent) error {
	if s == nil || s.conn == nil {
		return fmt.Errorf("interpreting connection is not initialized")
	}
	if event == nil {
		return fmt.Errorf("event is required")
	}
	return s.conn.WriteJSON(event)
}

func (s *InterpretingConn) Receive() (*InterpretingEvent, error) {
	if s == nil || s.conn == nil {
		return nil, fmt.Errorf("interpreting connection is not initialized")
	}
	var event InterpretingEvent
	if err := s.conn.ReadJSON(&event); err != nil {
		return nil, fmt.Errorf("read interpreting event: %w", err)
	}
	return &event, nil
}

func (s *InterpretingConn) Close() error {
	if s == nil || s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

func (c *Client) defaultInterpretingWebSocketURL(sessionID string) string {
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
	parsed.Path = "/v1/audio/interpreting/sessions/" + sessionID + "/ws"
	parsed.RawQuery = ""
	return parsed.String()
}
