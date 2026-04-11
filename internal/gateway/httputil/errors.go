package httputil

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type APIError struct {
	Status  int
	Type    string
	Code    string
	Param   string
	Message string
}

type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
	Param   string `json:"param,omitempty"`
}

func (e *APIError) Error() string {
	return e.Message
}

func NewError(status int, typ, code, param, message string) *APIError {
	return &APIError{
		Status:  status,
		Type:    typ,
		Code:    code,
		Param:   param,
		Message: message,
	}
}

func WriteError(c *gin.Context, err error) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		apiErr = NewError(http.StatusInternalServerError, "internal_error", "internal_error", "", "An internal error occurred.")
	}
	c.AbortWithStatusJSON(apiErr.Status, ErrorEnvelope{
		Error: ErrorBody{
			Message: apiErr.Message,
			Type:    apiErr.Type,
			Code:    apiErr.Code,
			Param:   apiErr.Param,
		},
	})
}
