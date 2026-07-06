package cache

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryCacheSetGet(t *testing.T) {
	c := NewMemory()
	defer func() { _ = c.Close() }()
	ctx := context.Background()

	if err := c.Set(ctx, "k", "v", time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	v, ok, err := c.Get(ctx, "k")
	if err != nil || !ok || v != "v" {
		t.Fatalf("Get = %q,%v,%v", v, ok, err)
	}
	if _, ok, _ := c.Get(ctx, "missing"); ok {
		t.Fatal("missing key reported present")
	}
	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestMemoryCacheIncrement(t *testing.T) {
	c := NewMemory()
	defer func() { _ = c.Close() }()
	ctx := context.Background()
	for i := int64(1); i <= 5; i++ {
		n, err := c.Increment(ctx, "cnt", time.Minute)
		if err != nil || n != i {
			t.Fatalf("Increment %d = %d,%v", i, n, err)
		}
	}
}

func TestMemoryCacheExpiry(t *testing.T) {
	c := NewMemory()
	defer func() { _ = c.Close() }()
	ctx := context.Background()
	if err := c.Set(ctx, "short", "x", 5*time.Millisecond); err != nil {
		t.Fatalf("Set: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, ok, _ := c.Get(ctx, "short"); ok {
		t.Fatal("expired key still present")
	}
	// An expired counter resets on the next increment.
	if _, err := c.Increment(ctx, "e", 5*time.Millisecond); err != nil {
		t.Fatalf("Increment: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	n, _ := c.Increment(ctx, "e", time.Minute)
	if n != 1 {
		t.Fatalf("expired counter did not reset: %d", n)
	}
}

func TestMemoryCacheConcurrent(t *testing.T) {
	c := NewMemory()
	defer func() { _ = c.Close() }()
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Increment(ctx, "shared", time.Minute)
		}()
	}
	wg.Wait()
	v, ok, _ := c.Get(ctx, "shared")
	if !ok || v != "50" {
		t.Fatalf("concurrent increment total = %q, want 50", v)
	}
}
