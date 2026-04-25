package retry

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
)

const maxBackoffDelay = 5 * time.Second

func RetryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func RetryableTransportError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func ShouldRetryAPIError(apiErr *httputil.APIError) bool {
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

func BackoffDelay(initial time.Duration, attempt int) time.Duration {
	if initial <= 0 {
		initial = 200 * time.Millisecond
	}
	if attempt <= 1 {
		return initial
	}
	delay := initial
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay > maxBackoffDelay {
			return maxBackoffDelay
		}
	}
	return delay
}

func SleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func TranslateTransportError(err error, providerName string) error {
	return httputil.ProviderTransportError(err, providerName)
}
