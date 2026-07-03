package apierror

import "net/http"

// Retryable reports whether a canonical APIError represents a transient
// upstream failure that is safe to retry against a fallback target. This is
// failover-orchestration policy (used by the gateway handler / routing layer),
// distinct from the transport package's own per-request retry loop. Rate-limit,
// timeout, and provider server errors are retryable; client errors are not.
func Retryable(apiErr *APIError) bool {
	if apiErr == nil {
		return false
	}
	if apiErr.Status == http.StatusTooManyRequests || apiErr.Type == "rate_limit_error" || apiErr.Code == "provider_rate_limit" {
		return true
	}
	if apiErr.Status == http.StatusGatewayTimeout || apiErr.Type == "timeout_error" || apiErr.Code == "provider_timeout" {
		return true
	}
	return apiErr.Code == "provider_server_error"
}
