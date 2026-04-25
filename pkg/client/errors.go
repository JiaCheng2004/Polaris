package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type APIError struct {
	StatusCode int
	Type       string
	Code       string
	Param      string
	Message    string
	Body       []byte
}

func (e *APIError) Error() string {
	if e == nil {
		return "polaris API error"
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return fmt.Sprintf("polaris API error: status=%d", e.StatusCode)
}

func decodeAPIErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	return decodeAPIError(resp.StatusCode, body)
}

func decodeAPIError(statusCode int, body []byte) error {
	type errorEnvelope struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
			Param   string `json:"param"`
		} `json:"error"`
	}

	var envelope errorEnvelope
	_ = json.Unmarshal(body, &envelope)

	message := strings.TrimSpace(envelope.Error.Message)
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	if message == "" {
		message = http.StatusText(statusCode)
	}

	errorType := strings.TrimSpace(envelope.Error.Type)
	if errorType == "" {
		errorType = "api_error"
	}

	return &APIError{
		StatusCode: statusCode,
		Type:       errorType,
		Code:       strings.TrimSpace(envelope.Error.Code),
		Param:      strings.TrimSpace(envelope.Error.Param),
		Message:    message,
		Body:       append([]byte(nil), body...),
	}
}
