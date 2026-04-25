package httputil

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
)

type ProviderErrorDetails struct {
	Message string
	Body    string
	Code    string
	Param   string
	Type    string
	Status  string
}

func ProviderAPIError(providerName string, status int, details ProviderErrorDetails) *APIError {
	message := firstNonEmptyText(details.Message, details.Body)
	if message == "" {
		message = strings.TrimSpace(providerName) + " returned an error."
	}

	code := normalizeProviderCode(details.Code, details.Status, details.Type)

	switch {
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		return NewError(http.StatusBadRequest, "invalid_request_error", firstNonEmptyText(code, "provider_bad_request"), details.Param, message)
	case status == http.StatusTooManyRequests:
		if quotaExceeded(details) {
			return NewError(http.StatusTooManyRequests, "rate_limit_error", "quota_exceeded", "", message)
		}
		return NewError(http.StatusTooManyRequests, "rate_limit_error", firstNonEmptyText(code, "provider_rate_limit"), "", message)
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return NewError(http.StatusBadGateway, "provider_error", "provider_auth_failed", "", message)
	case status == http.StatusRequestTimeout:
		return NewError(http.StatusGatewayTimeout, "timeout_error", "provider_timeout", "", message)
	case status >= http.StatusInternalServerError:
		return NewError(http.StatusBadGateway, "provider_error", "provider_server_error", "", message)
	default:
		return NewError(http.StatusBadGateway, "provider_error", firstNonEmptyText(code, "provider_error"), details.Param, message)
	}
}

func ProviderAuthError(providerName string, message string) *APIError {
	message = strings.TrimSpace(message)
	if message == "" {
		message = strings.TrimSpace(providerName) + " authentication failed."
	}
	return NewError(http.StatusBadGateway, "provider_error", "provider_auth_failed", "", message)
}

func ProviderTransportError(err error, providerName string) *APIError {
	if err == nil {
		return NewError(http.StatusBadGateway, "provider_error", "provider_transport_error", "", strings.TrimSpace(providerName)+" request failed.")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewError(http.StatusGatewayTimeout, "timeout_error", "provider_timeout", "", strings.TrimSpace(providerName)+" timed out.")
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return NewError(http.StatusGatewayTimeout, "timeout_error", "provider_timeout", "", strings.TrimSpace(providerName)+" timed out.")
	}
	return NewError(http.StatusBadGateway, "provider_error", "provider_transport_error", "", strings.TrimSpace(providerName)+" request failed.")
}

func normalizeProviderCode(values ...string) string {
	value := strings.ToLower(strings.TrimSpace(firstNonEmptyText(values...)))
	value = strings.ReplaceAll(value, " ", "_")
	return value
}

func quotaExceeded(details ProviderErrorDetails) bool {
	haystack := strings.ToLower(strings.Join([]string{
		details.Message,
		details.Body,
		details.Code,
		details.Type,
		details.Status,
	}, " "))

	return strings.Contains(haystack, "insufficient_quota") ||
		strings.Contains(haystack, "quota exceeded") ||
		strings.Contains(haystack, "exceeded your current quota") ||
		strings.Contains(haystack, "billing") ||
		strings.Contains(haystack, "credit balance") ||
		strings.Contains(haystack, "insufficient balance") ||
		strings.Contains(haystack, "quota")
}

func firstNonEmptyText(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
