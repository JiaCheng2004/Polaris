package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/store/cache"
)

func TestRedisCacheOperations(t *testing.T) {
	url := os.Getenv("POLARIS_TEST_REDIS_URL")
	if url == "" {
		t.Skip("POLARIS_TEST_REDIS_URL is not set")
	}

	redisCache, err := cache.NewRedis(url)
	if err != nil {
		t.Fatalf("NewRedis() error = %v", err)
	}
	defer func() {
		_ = redisCache.Close()
	}()

	ctx := context.Background()
	if err := redisCache.Ping(ctx); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}

	keyPrefix := "polaris-test:" + randomHex(t)

	if err := redisCache.Set(ctx, keyPrefix+":value", "hello", time.Second); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	value, ok, err := redisCache.Get(ctx, keyPrefix+":value")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || value != "hello" {
		t.Fatalf("expected cached value hello, got ok=%t value=%q", ok, value)
	}

	time.Sleep(1100 * time.Millisecond)
	_, ok, err = redisCache.Get(ctx, keyPrefix+":value")
	if err != nil {
		t.Fatalf("Get() after ttl error = %v", err)
	}
	if ok {
		t.Fatalf("expected cached value to expire")
	}

	count, err := redisCache.Increment(ctx, keyPrefix+":counter", 5*time.Second)
	if err != nil {
		t.Fatalf("Increment() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("expected first increment to return 1, got %d", count)
	}
	count, err = redisCache.Increment(ctx, keyPrefix+":counter", 5*time.Second)
	if err != nil {
		t.Fatalf("Increment() second call error = %v", err)
	}
	if count != 2 {
		t.Fatalf("expected second increment to return 2, got %d", count)
	}
}
