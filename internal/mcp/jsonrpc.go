package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Request is a JSON-RPC 2.0 request or notification. A notification omits id.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// IsNotification reports whether the request carries no id (fire-and-forget).
func (r Request) IsNotification() bool { return len(bytes.TrimSpace(r.ID)) == 0 }

// Response is a JSON-RPC 2.0 response. Exactly one of Result/Error is set.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error is a JSON-RPC 2.0 error object.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *Error) Error() string { return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message) }

// NewError builds a JSON-RPC error object.
func NewError(code int, message string, data any) *Error {
	return &Error{Code: code, Message: message, Data: data}
}

func newResult(id json.RawMessage, result any) Response {
	return Response{JSONRPC: "2.0", ID: id, Result: result}
}

func newFailure(id json.RawMessage, err *Error) Response {
	return Response{JSONRPC: "2.0", ID: id, Error: err}
}

// DecodeRequests parses a single request or a batch array. The bool reports
// whether the payload was a batch (so the response is encoded as an array).
func DecodeRequests(data []byte) ([]Request, bool, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, false, errors.New("empty request body")
	}
	if trimmed[0] == '[' {
		var batch []Request
		if err := json.Unmarshal(trimmed, &batch); err != nil {
			return nil, true, err
		}
		if len(batch) == 0 {
			return nil, true, errors.New("empty batch")
		}
		return batch, true, nil
	}
	var single Request
	if err := json.Unmarshal(trimmed, &single); err != nil {
		return nil, false, err
	}
	return []Request{single}, false, nil
}
