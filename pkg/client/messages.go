package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

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

type MessagesStream struct {
	body   io.ReadCloser
	reader *bufio.Reader
	event  MessagesStreamEvent
	err    error
	done   bool
}

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

func (stream *MessagesStream) Event() MessagesStreamEvent {
	if stream == nil {
		return MessagesStreamEvent{}
	}
	return stream.event
}

func (stream *MessagesStream) Err() error {
	if stream == nil {
		return nil
	}
	return stream.err
}

func (stream *MessagesStream) Close() error {
	if stream == nil || stream.body == nil {
		return nil
	}
	stream.done = true
	body := stream.body
	stream.body = nil
	return body.Close()
}
