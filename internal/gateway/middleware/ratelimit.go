package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/metrics"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/gateway/telemetry"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
)

func RateLimit(holder *gwruntime.Holder, limiter cache.Cache, logger *slog.Logger, recorder *metrics.Recorder) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(c *gin.Context) {
		ctx, span := telemetry.StartInternalSpan(c.Request.Context(), "rate_limit.evaluate")
		defer span.End()
		c.Request = c.Request.WithContext(ctx)

		snapshot := RuntimeSnapshot(c, holder)
		if snapshot == nil || snapshot.Config == nil {
			httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "runtime_unavailable", "", "Runtime configuration is unavailable."))
			return
		}
		cfg := snapshot.Config

		if !cfg.Cache.RateLimit.Enabled || limiter == nil {
			c.Next()
			return
		}

		auth := GetAuthContext(c)
		rate := auth.RateLimit
		if rate == "" {
			rate = cfg.Cache.RateLimit.Default
		}
		if rate == "" {
			c.Next()
			return
		}

		limit, window, err := parseRateLimit(rate)
		if err != nil {
			logger.Error("invalid rate limit configuration", "rate_limit", rate, "error", err)
			c.Next()
			return
		}

		now := time.Now().UTC()
		windowSeconds := int64(window.Seconds())
		currentStart := now.Unix() / windowSeconds * windowSeconds
		previousStart := currentStart - windowSeconds

		currentKey := fmt.Sprintf("ratelimit:%s:%d", auth.KeyID, currentStart)
		previousKey := fmt.Sprintf("ratelimit:%s:%d", auth.KeyID, previousStart)

		currentCount, err := limiter.Increment(context.Background(), currentKey, 2*window)
		if err != nil {
			logger.Warn("rate limit increment failed, allowing request", "request_id", GetRequestID(c), "error", err)
			c.Next()
			return
		}

		previousCount := int64(0)
		if previousRaw, ok, err := limiter.Get(context.Background(), previousKey); err == nil && ok {
			if value, parseErr := strconv.ParseInt(previousRaw, 10, 64); parseErr == nil {
				previousCount = value
			}
		}

		elapsed := now.Sub(time.Unix(currentStart, 0))
		weightedCount := float64(currentCount) + float64(previousCount)*(1-float64(elapsed)/float64(window))
		remaining := max(0, limit-int64(math.Ceil(weightedCount)))
		c.Header("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))

		if weightedCount > float64(limit) {
			retryAfter := int64(math.Ceil((window - elapsed).Seconds()))
			if retryAfter < 1 {
				retryAfter = 1
			}
			span.SetAttributes(
				attribute.String("polaris.key_id", auth.KeyID),
				attribute.Bool("polaris.rate_limit.denied", true),
			)
			c.Header("Retry-After", strconv.FormatInt(retryAfter, 10))
			recorder.IncRateLimit(auth.KeyID)
			httputil.WriteError(c, httputil.NewError(http.StatusTooManyRequests, "rate_limit_error", "rate_limit_exceeded", "", "Rate limit exceeded."))
			return
		}

		c.Next()
	}
}

func parseRateLimit(raw string) (int64, time.Duration, error) {
	parts := strings.Split(strings.TrimSpace(raw), "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("rate limit %q must use count/window format", raw)
	}
	limit, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || limit <= 0 {
		return 0, 0, fmt.Errorf("rate limit %q has invalid count", raw)
	}

	switch strings.ToLower(parts[1]) {
	case "s", "sec", "second":
		return limit, time.Second, nil
	case "m", "min", "minute":
		return limit, time.Minute, nil
	case "h", "hour":
		return limit, time.Hour, nil
	case "d", "day":
		return limit, 24 * time.Hour, nil
	default:
		return 0, 0, fmt.Errorf("rate limit %q has invalid window", raw)
	}
}
