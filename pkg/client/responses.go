package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (c *Client) CreateResponse(ctx context.Context, req *ResponsesRequest) (*ResponsesResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	payload := *req
	payload.Stream = false

	var response ResponsesResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/responses", nil, payload, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) StreamResponse(ctx context.Context, req *ResponsesRequest) (*ResponsesStream, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	payload := *req
	payload.Stream = true

	resp, err := c.do(ctx, http.MethodPost, "/v1/responses", nil, payload, "application/json", "text/event-stream")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer func() {
			_ = resp.Body.Close()
		}()
		return nil, decodeAPIErrorResponse(resp)
	}
	return &ResponsesStream{
		body:   resp.Body,
		reader: bufio.NewReader(resp.Body),
	}, nil
}

type ResponsesStream struct {
	body   io.ReadCloser
	reader *bufio.Reader
	event  ResponsesStreamEvent
	err    error
	done   bool
}

func (stream *ResponsesStream) Next() bool {
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
	var event ResponsesStreamEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		stream.err = fmt.Errorf("decode responses stream event: %w", err)
		stream.done = true
		return false
	}
	stream.event = event
	return true
}

func (stream *ResponsesStream) Event() ResponsesStreamEvent {
	if stream == nil {
		return ResponsesStreamEvent{}
	}
	return stream.event
}

func (stream *ResponsesStream) Err() error {
	if stream == nil {
		return nil
	}
	return stream.err
}

func (stream *ResponsesStream) Close() error {
	if stream == nil || stream.body == nil {
		return nil
	}
	stream.done = true
	body := stream.body
	stream.body = nil
	return body.Close()
}

func readSSEPayload(reader *bufio.Reader) (string, bool, error) {
	var dataLines []string
	flush := func() (string, bool) {
		if len(dataLines) == 0 {
			return "", false
		}
		payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = nil
		if payload == "[DONE]" {
			return "", true
		}
		return payload, false
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			if err == io.EOF {
				payload, done := flush()
				return payload, done, nil
			}
			return "", false, fmt.Errorf("read sse stream: %w", err)
		}

		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			payload, done := flush()
			if done || payload != "" {
				return payload, done, nil
			}
		} else if strings.HasPrefix(trimmed, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
		}

		if err == io.EOF {
			payload, done := flush()
			return payload, done, nil
		}
	}
}

func decodeSSEError(payload string) error {
	type errorEnvelope struct {
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
			Param   string `json:"param"`
		} `json:"error"`
	}

	var envelope errorEnvelope
	if err := json.Unmarshal([]byte(payload), &envelope); err == nil && envelope.Error != nil {
		return decodeAPIError(http.StatusBadGateway, []byte(payload))
	}
	return nil
}
