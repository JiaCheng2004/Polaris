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

func (c *Client) CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	payload := *req
	payload.Stream = false

	var response ChatCompletionResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/chat/completions", nil, payload, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) StreamChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatStream, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	payload := *req
	payload.Stream = true

	resp, err := c.do(ctx, http.MethodPost, "/v1/chat/completions", nil, payload, "application/json", "text/event-stream")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer func() {
			_ = resp.Body.Close()
		}()
		return nil, decodeAPIErrorResponse(resp)
	}

	return &ChatStream{
		body:   resp.Body,
		reader: bufio.NewReader(resp.Body),
	}, nil
}

type ChatStream struct {
	body   io.ReadCloser
	reader *bufio.Reader
	chunk  ChatCompletionChunk
	err    error
	done   bool
}

func (stream *ChatStream) Next() bool {
	if stream == nil || stream.done {
		return false
	}

	var dataLines []string
	flush := func() (bool, error) {
		if len(dataLines) == 0 {
			return false, nil
		}

		payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = nil
		if payload == "" {
			return false, nil
		}
		if payload == "[DONE]" {
			stream.done = true
			return false, nil
		}

		var errorEnvelope struct {
			Error *struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
				Param   string `json:"param"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &errorEnvelope); err == nil && errorEnvelope.Error != nil {
			stream.err = decodeAPIError(http.StatusBadGateway, []byte(payload))
			stream.done = true
			return false, nil
		}

		var chunk ChatCompletionChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return false, fmt.Errorf("decode chat stream chunk: %w", err)
		}
		stream.chunk = chunk
		return true, nil
	}

	for {
		line, err := stream.reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			if err == io.EOF {
				ok, flushErr := flush()
				if flushErr != nil {
					stream.err = flushErr
				}
				stream.done = true
				return ok
			}
			stream.err = fmt.Errorf("read chat stream: %w", err)
			stream.done = true
			return false
		}

		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			ok, flushErr := flush()
			if flushErr != nil {
				stream.err = flushErr
				stream.done = true
				return false
			}
			if ok {
				return true
			}
			if stream.done {
				return false
			}
		} else if strings.HasPrefix(trimmed, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
		}

		if err == io.EOF {
			ok, flushErr := flush()
			if flushErr != nil {
				stream.err = flushErr
				stream.done = true
				return false
			}
			stream.done = true
			return ok
		}
	}
}

func (stream *ChatStream) Chunk() ChatCompletionChunk {
	if stream == nil {
		return ChatCompletionChunk{}
	}
	return stream.chunk
}

func (stream *ChatStream) Err() error {
	if stream == nil {
		return nil
	}
	return stream.err
}

func (stream *ChatStream) Close() error {
	if stream == nil || stream.body == nil {
		return nil
	}
	stream.done = true
	body := stream.body
	stream.body = nil
	return body.Close()
}
