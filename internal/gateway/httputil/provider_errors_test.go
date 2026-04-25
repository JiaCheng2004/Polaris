package httputil

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
)

func TestProviderAPIErrorMapsQuotaExceeded(t *testing.T) {
	err := ProviderAPIError("OpenAI", http.StatusTooManyRequests, ProviderErrorDetails{
		Message: "You exceeded your current quota, please check your plan and billing details.",
		Code:    "insufficient_quota",
	})

	if err.Status != http.StatusTooManyRequests || err.Type != "rate_limit_error" || err.Code != "quota_exceeded" {
		t.Fatalf("unexpected API error %#v", err)
	}
}

func TestProviderAPIErrorMapsRateLimit(t *testing.T) {
	err := ProviderAPIError("OpenAI", http.StatusTooManyRequests, ProviderErrorDetails{
		Message: "Too many requests.",
		Code:    "rate_limit",
	})

	if err.Status != http.StatusTooManyRequests || err.Type != "rate_limit_error" || err.Code != "rate_limit" {
		t.Fatalf("unexpected API error %#v", err)
	}
}

func TestProviderAPIErrorMapsInvalidRequest(t *testing.T) {
	err := ProviderAPIError("Google", http.StatusBadRequest, ProviderErrorDetails{
		Message: "Prompt is required.",
		Code:    "INVALID_ARGUMENT",
		Param:   "prompt",
	})

	if err.Status != http.StatusBadRequest || err.Type != "invalid_request_error" || err.Code != "invalid_argument" || err.Param != "prompt" {
		t.Fatalf("unexpected API error %#v", err)
	}
}

func TestProviderAPIErrorMapsAuthFailure(t *testing.T) {
	err := ProviderAPIError("ByteDance", http.StatusUnauthorized, ProviderErrorDetails{
		Message: "Unauthorized.",
	})

	if err.Status != http.StatusBadGateway || err.Type != "provider_error" || err.Code != "provider_auth_failed" {
		t.Fatalf("unexpected API error %#v", err)
	}
}

func TestProviderTransportErrorMapsTimeout(t *testing.T) {
	err := ProviderTransportError(context.DeadlineExceeded, "Anthropic")
	if err.Status != http.StatusGatewayTimeout || err.Type != "timeout_error" || err.Code != "provider_timeout" {
		t.Fatalf("unexpected API error %#v", err)
	}
}

func TestProviderTransportErrorMapsNetworkFailure(t *testing.T) {
	err := ProviderTransportError(errors.New("dial tcp"), "DeepSeek")
	if err.Status != http.StatusBadGateway || err.Type != "provider_error" || err.Code != "provider_transport_error" {
		t.Fatalf("unexpected API error %#v", err)
	}
}

func TestProviderTransportErrorMapsNetTimeout(t *testing.T) {
	err := ProviderTransportError(timeoutNetError{}, "Qwen")
	if err.Status != http.StatusGatewayTimeout || err.Type != "timeout_error" || err.Code != "provider_timeout" {
		t.Fatalf("unexpected API error %#v", err)
	}
}

type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return false }

var _ net.Error = timeoutNetError{}
