package cache

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestRedisCache runs against a real Redis when REDIS_URL is set (CI provides a
// redis:7 service); it is skipped otherwise.
func TestRedisCache(t *testing.T) {
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL not set; skipping Redis integration test")
	}
	c, err := NewRedis(url)
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	defer func() { _ = c.Close() }()

	ctx := context.Background()
	if err := c.Ping(ctx); err != nil {
		t.Skipf("Redis unavailable: %v", err)
	}

	if err := c.Set(ctx, "rk", "rv", time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	v, ok, err := c.Get(ctx, "rk")
	if err != nil || !ok || v != "rv" {
		t.Fatalf("Get = %q,%v,%v", v, ok, err)
	}
	n1, err := c.Increment(ctx, "rcnt", time.Minute)
	if err != nil {
		t.Fatalf("Increment: %v", err)
	}
	n2, _ := c.Increment(ctx, "rcnt", time.Minute)
	if n2 != n1+1 {
		t.Fatalf("Increment not monotonic: %d then %d", n1, n2)
	}
	if _, ok, _ := c.Get(ctx, "definitely-missing-key"); ok {
		t.Fatal("missing key reported present")
	}
}
