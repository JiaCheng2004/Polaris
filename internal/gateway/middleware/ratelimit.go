package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/metrics"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/obs"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
)

// rateStore is the subset of cache.Cache the sliding-window limiter needs. Both
// the primary limiter and the process-local fallback satisfy it.
type rateStore interface {
	Increment(ctx context.Context, key string, ttl time.Duration) (int64, error)
	Get(ctx context.Context, key string) (string, bool, error)
}

func RateLimit(holder *gwruntime.Holder, limiter cache.Cache, logger *slog.Logger, recorder *metrics.Recorder) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	// A best-effort, process-local fallback used when the primary limiter (Redis)
	// is unavailable and fail_mode is "open". Goroutine-free (lazy expiry) so it
	// adds no shutdown/goleak surface.
	fallback := newLocalRateCounter()

	return func(c *gin.Context) {
		ctx, span := obs.StartInternalSpan(c.Request.Context(), "rate_limit.evaluate")
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

		bucket := rateLimitBucket(c)
		currentKey := fmt.Sprintf("ratelimit:%s:%s:%d", bucket, auth.KeyID, currentStart)
		previousKey := fmt.Sprintf("ratelimit:%s:%s:%d", bucket, auth.KeyID, previousStart)

		failMode := strings.ToLower(strings.TrimSpace(cfg.Cache.RateLimit.FailMode))
		if failMode == "" {
			failMode = "open"
		}

		var effective rateStore = limiter
		currentCount, err := limiter.Increment(context.Background(), currentKey, 2*window)
		if err != nil {
			if failMode == "closed" {
				// Strict: reject rather than risk unbounded traffic (and provider
				// spend) while the limiter is down.
				logger.Warn("rate limit primary unavailable, failing closed", "request_id", GetRequestID(c), "error", err)
				recorder.IncRateLimitDegraded("closed", "rejected")
				retryAfter := int64(math.Ceil(window.Seconds()))
				if retryAfter < 1 {
					retryAfter = 1
				}
				c.Header("Retry-After", strconv.FormatInt(retryAfter, 10))
				httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "rate_limit_error", "rate_limiter_unavailable", "", "Rate limiter is temporarily unavailable."))
				return
			}
			// Open (default): degrade to best-effort process-local counting.
			logger.Warn("rate limit primary unavailable, degrading to local counting", "request_id", GetRequestID(c), "error", err)
			recorder.IncRateLimitDegraded("open", "fallback")
			effective = fallback
			currentCount, err = fallback.Increment(context.Background(), currentKey, 2*window)
			if err != nil {
				c.Next()
				return
			}
		}

		previousCount := int64(0)
		if previousRaw, ok, err := effective.Get(context.Background(), previousKey); err == nil && ok {
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

func rateLimitBucket(c *gin.Context) string {
	path := c.FullPath()
	if path == "" {
		path = c.Request.URL.Path
	}
	switch {
	case strings.HasPrefix(path, "/v1/files") && c.Request.Method == http.MethodPost:
		return "files_upload"
	case strings.HasPrefix(path, "/v1/files"):
		return "files_read"
	default:
		return "requests"
	}
}

type localRateEntry struct {
	count   int64
	expires time.Time
}

// localRateCounter is a process-local sliding-window counter with lazy expiry
// and no background goroutine, used as the rate-limit fallback when the primary
// limiter is unavailable in fail_mode "open".
type localRateCounter struct {
	mu sync.Mutex
	m  map[string]localRateEntry
}

func newLocalRateCounter() *localRateCounter {
	return &localRateCounter{m: make(map[string]localRateEntry)}
}

func (l *localRateCounter) Increment(_ context.Context, key string, ttl time.Duration) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.evictExpiredLocked(now)
	e, ok := l.m[key]
	if !ok || now.After(e.expires) {
		e = localRateEntry{expires: now.Add(ttl)}
	}
	e.count++
	l.m[key] = e
	return e.count, nil
}

func (l *localRateCounter) Get(_ context.Context, key string) (string, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.m[key]
	if !ok || time.Now().After(e.expires) {
		return "", false, nil
	}
	return strconv.FormatInt(e.count, 10), true, nil
}

// evictExpiredLocked drops expired entries once the map grows past a threshold,
// keeping it bounded without a janitor goroutine.
func (l *localRateCounter) evictExpiredLocked(now time.Time) {
	if len(l.m) < 1024 {
		return
	}
	for k, e := range l.m {
		if now.After(e.expires) {
			delete(l.m, k)
		}
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
