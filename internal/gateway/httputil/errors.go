// Package httputil holds the gin-coupled HTTP glue for the gateway layer. The
// canonical error type and provider-error translation now live in
// internal/apierror; this package re-exports them (so gateway handlers keep a
// single import) and adds WriteError, the only piece that depends on gin.
package httputil

import (
	"errors"
	"net/http"

	"github.com/JiaCheng2004/Polaris/internal/apierror"
	"github.com/gin-gonic/gin"
)

// Re-exported canonical error types (defined in internal/apierror).
type (
	APIError             = apierror.APIError
	ErrorEnvelope        = apierror.ErrorEnvelope
	ErrorBody            = apierror.ErrorBody
	ProviderErrorDetails = apierror.ProviderErrorDetails
)

// Re-exported constructors/helpers (defined in internal/apierror).
var (
	NewError                 = apierror.NewError
	RequestBodyTooLargeError = apierror.RequestBodyTooLargeError
	IsRequestBodyTooLarge    = apierror.IsRequestBodyTooLarge
	ProviderAPIError         = apierror.ProviderAPIError
	ProviderAuthError        = apierror.ProviderAuthError
	ProviderTransportError   = apierror.ProviderTransportError
)

// WriteError renders err as the OpenAI-compatible error envelope and aborts the
// request. Non-APIError values become a generic 500.
func WriteError(c *gin.Context, err error) {
	var apiErr *apierror.APIError
	if !errors.As(err, &apiErr) {
		apiErr = apierror.NewError(http.StatusInternalServerError, "internal_error", "internal_error", "", "An internal error occurred.")
	}
	c.AbortWithStatusJSON(apiErr.Status, apierror.ErrorEnvelope{
		Error: apierror.ErrorBody{
			Message: apiErr.Message,
			Type:    apiErr.Type,
			Code:    apiErr.Code,
			Param:   apiErr.Param,
		},
	})
}
