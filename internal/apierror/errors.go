// Package apierror defines Polaris's OpenAI-compatible error type and the
// provider-error translation helpers. It is dependency-light (stdlib only) so
// that provider adapters and the outbound transport can produce canonical
// errors without importing the HTTP gateway layer. The gin-coupled response
// writer lives in internal/gateway/httputil.
package apierror

import (
	"errors"
	"fmt"
	"net/http"
)

// APIError is the canonical Polaris error carried across every layer and
// rendered as the OpenAI-compatible error envelope.
type APIError struct {
	Status  int
	Type    string
	Code    string
	Param   string
	Message string
}

// ErrorEnvelope is the JSON wire shape: {"error": {...}}.
type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody is the inner error object of the envelope.
type ErrorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
	Param   string `json:"param,omitempty"`
}

func (e *APIError) Error() string {
	return e.Message
}

// NewError constructs an APIError.
func NewError(status int, typ, code, param, message string) *APIError {
	return &APIError{
		Status:  status,
		Type:    typ,
		Code:    code,
		Param:   param,
		Message: message,
	}
}

// RequestBodyTooLargeError builds the 413 error for an over-limit request body.
func RequestBodyTooLargeError(maxBytes int64) *APIError {
	if maxBytes <= 0 {
		return NewError(http.StatusRequestEntityTooLarge, "invalid_request_error", "request_body_too_large", "", "Request body exceeds the configured maximum size.")
	}
	return NewError(http.StatusRequestEntityTooLarge, "invalid_request_error", "request_body_too_large", "", fmt.Sprintf("Request body exceeds the configured maximum size of %d bytes.", maxBytes))
}

// IsRequestBodyTooLarge reports whether err is an http.MaxBytesError.
func IsRequestBodyTooLarge(err error) bool {
	var maxBytesError *http.MaxBytesError
	return errors.As(err, &maxBytesError)
}
