package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/metrics"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/gin-gonic/gin"
)

type fakeRateCache struct {
	mu     sync.Mutex
	counts map[string]int64
	incErr error
}

func newFakeRateCache() *fakeRateCache { return &fakeRateCache{counts: map[string]int64{}} }

func (f *fakeRateCache) Increment(_ context.Context, key string, _ time.Duration) (int64, error) {
	if f.incErr != nil {
		return 0, f.incErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counts[key]++
	return f.counts[key], nil
}

func (f *fakeRateCache) Get(_ context.Context, key string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.counts[key]
	if !ok {
		return "", false, nil
	}
	return strconv.FormatInt(v, 10), true, nil
}

func (f *fakeRateCache) Set(context.Context, string, string, time.Duration) error { return nil }
func (f *fakeRateCache) Ping(context.Context) error                               { return nil }
func (f *fakeRateCache) Close() error                                             { return nil }

func newRateLimitEngine(limiter *fakeRateCache, failMode, rate string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := config.Default()
	cfg.Cache.RateLimit.Enabled = true
	cfg.Cache.RateLimit.FailMode = failMode
	holder := gwruntime.NewHolder(&cfg, nil)
	rec := metrics.NewRecorder()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		SetRuntimeSnapshot(c, holder.Current())
		SetAuthContext(c, AuthContext{KeyID: "k", RateLimit: rate})
		c.Next()
	})
	r.Use(RateLimit(holder, limiter, nil, rec))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func fire(r *gin.Engine) int {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	return w.Code
}

func TestRateLimitSlidingWindow(t *testing.T) {
	r := newRateLimitEngine(newFakeRateCache(), "open", "3/min")
	for i := 1; i <= 3; i++ {
		if code := fire(r); code != http.StatusOK {
			t.Fatalf("request %d = %d, want 200", i, code)
		}
	}
	if code := fire(r); code != http.StatusTooManyRequests {
		t.Fatalf("4th request = %d, want 429", code)
	}
}

func TestRateLimitFailOpenDegradesToLocal(t *testing.T) {
	limiter := newFakeRateCache()
	limiter.incErr = errors.New("redis down")
	r := newRateLimitEngine(limiter, "open", "2/min")

	// Fail-open degrades to the local counter — still enforces (not unlimited).
	if code := fire(r); code != http.StatusOK {
		t.Fatalf("1st = %d, want 200", code)
	}
	if code := fire(r); code != http.StatusOK {
		t.Fatalf("2nd = %d, want 200", code)
	}
	if code := fire(r); code != http.StatusTooManyRequests {
		t.Fatalf("3rd = %d, want 429 (local fallback enforces)", code)
	}
}

func TestRateLimitFailClosed(t *testing.T) {
	limiter := newFakeRateCache()
	limiter.incErr = errors.New("redis down")
	r := newRateLimitEngine(limiter, "closed", "10/min")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("fail-closed status = %d, want 503", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("fail-closed missing Retry-After")
	}
}

func TestParseRateLimit(t *testing.T) {
	cases := map[string]struct {
		limit  int64
		window time.Duration
		err    bool
	}{
		"60/min":  {60, time.Minute, false},
		"10/s":    {10, time.Second, false},
		"5/hour":  {5, time.Hour, false},
		"1/day":   {1, 24 * time.Hour, false},
		"0/min":   {0, 0, true},
		"x/min":   {0, 0, true},
		"10":      {0, 0, true},
		"10/year": {0, 0, true},
	}
	for raw, want := range cases {
		limit, window, err := parseRateLimit(raw)
		if want.err {
			if err == nil {
				t.Fatalf("%q: expected error", raw)
			}
			continue
		}
		if err != nil || limit != want.limit || window != want.window {
			t.Fatalf("%q => (%d,%v,%v), want (%d,%v)", raw, limit, window, err, want.limit, want.window)
		}
	}
}

func TestRateLimitBucketSelection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		method, path, want string
	}{
		{http.MethodPost, "/v1/files", "files_upload"},
		{http.MethodGet, "/v1/files/abc/content", "files_read"},
		{http.MethodPost, "/v1/chat/completions", "requests"},
	}
	for _, tc := range cases {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(tc.method, tc.path, nil)
		if got := rateLimitBucket(c); got != tc.want {
			t.Fatalf("%s %s => %q, want %q", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestLocalRateCounter(t *testing.T) {
	l := newLocalRateCounter()
	ctx := context.Background()
	for i := int64(1); i <= 3; i++ {
		n, _ := l.Increment(ctx, "k", time.Minute)
		if n != i {
			t.Fatalf("increment %d = %d", i, n)
		}
	}
	v, ok, _ := l.Get(ctx, "k")
	if !ok || v != "3" {
		t.Fatalf("get = %q,%v want 3,true", v, ok)
	}
	// Expired entry reads as absent.
	_, _ = l.Increment(ctx, "short", time.Nanosecond)
	time.Sleep(time.Millisecond)
	if _, ok, _ := l.Get(ctx, "short"); ok {
		t.Fatal("expired entry returned as present")
	}
}

func TestRateLimitConcurrent(t *testing.T) {
	r := newRateLimitEngine(newFakeRateCache(), "open", "1000/min")
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fire(r)
		}()
	}
	wg.Wait()
}
