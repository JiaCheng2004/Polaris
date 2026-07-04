package store

import (
	"testing"
	"time"
)

func TestTTLCacheGetSetDelete(t *testing.T) {
	c := NewTTLCache[string](time.Minute)
	if _, ok := c.Get("missing"); ok {
		t.Fatal("empty cache should miss")
	}
	c.Set("k", "v")
	if v, ok := c.Get("k"); !ok || v != "v" {
		t.Fatalf("Get after Set = %q,%v", v, ok)
	}
	c.Delete("k")
	if _, ok := c.Get("k"); ok {
		t.Fatal("Get after Delete should miss")
	}
}

func TestTTLCacheExpiry(t *testing.T) {
	c := NewTTLCache[int](time.Nanosecond)
	c.Set("k", 42)
	time.Sleep(time.Millisecond)
	if _, ok := c.Get("k"); ok {
		t.Fatal("entry should have expired")
	}
}

func TestTTLCacheClearAndDefaults(t *testing.T) {
	c := NewTTLCache[int](0) // defaults to 60s
	c.Set("a", 1)
	c.Set("b", 2)
	c.Clear()
	if _, ok := c.Get("a"); ok {
		t.Fatal("Clear should evict all")
	}
	c.Set("", 9) // empty key is a no-op
	if _, ok := c.Get(""); ok {
		t.Fatal("empty key must not be stored")
	}
}

func TestTTLCacheNilSafe(t *testing.T) {
	var c *TTLCache[int]
	c.Set("k", 1)
	if _, ok := c.Get("k"); ok {
		t.Fatal("nil cache Get should miss")
	}
	c.Delete("k")
	c.Clear()
}
