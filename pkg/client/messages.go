package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// CreateMessage creates a message.
func (c *Client) CreateMessage(ctx context.Context, req *MessagesRequest) (*MessagesResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	payload := *req
	payload.Stream = false

	var response MessagesResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/messages", nil, payload, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// StreamMessage opens a streaming message.
func (c *Client) StreamMessage(ctx context.Context, req *MessagesRequest) (*MessagesStream, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	payload := *req
	payload.Stream = true

	resp, err := c.do(ctx, http.MethodPost, "/v1/messages", nil, payload, "application/json", "text/event-stream")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer func() {
			_ = resp.Body.Close()
		}()
		return nil, decodeAPIErrorResponse(resp)
	}
	return &MessagesStream{
		body:   resp.Body,
		reader: bufio.NewReader(resp.Body),
	}, nil
}

// MessagesStream is a server-sent-event iterator over messages chunks.
type MessagesStream struct {
	body   io.ReadCloser
	reader *bufio.Reader
	event  MessagesStreamEvent
	err    error
	done   bool
}

// Next advances the iterator, returning false at end of stream or on error.
func (stream *MessagesStream) Next() bool {
	if stream == nil || stream.done {
		return false
	}
	payload, done, err := readSSEPayload(stream.reader)
	if err != nil {
		stream.err = err
		stream.done = true
		return false
	}
	if done {
		stream.done = true
		return false
	}
	if err := decodeSSEError(payload); err != nil {
		stream.err = err
		stream.done = true
		return false
	}
	var event MessagesStreamEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		stream.err = fmt.Errorf("decode messages stream event: %w", err)
		stream.done = true
		return false
	}
	stream.event = event
	return true
}

// Event returns the most recent event yielded by the stream.
func (stream *MessagesStream) Event() MessagesStreamEvent {
	if stream == nil {
		return MessagesStreamEvent{}
	}
	return stream.event
}

// Err returns the terminal stream error, if any.
func (stream *MessagesStream) Err() error {
	if stream == nil {
		return nil
	}
	return stream.err
}

// Close closes the underlying connection.
func (stream *MessagesStream) Close() error {
	if stream == nil || stream.body == nil {
		return nil
	}
	stream.done = true
	body := stream.body
	stream.body = nil
	return body.Close()
}
