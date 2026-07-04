// Package store defines Polaris's persistence contract (the Store interface and
// its role interfaces), the data models, the never-block async request/audit
// loggers, and small in-memory helpers such as the generic TTL cache.
package store

import (
	"sync"
	"time"
)

// TTLCache is a concurrency-safe, size-agnostic in-memory cache with a fixed
// per-entry TTL and value (copy) semantics. It single-sources the small caches
// used by the auth middleware (API keys, virtual keys) and control-plane lookups.
// A nil *TTLCache is a valid no-op cache.
type TTLCache[T any] struct {
	ttl   time.Duration
	mu    sync.RWMutex
	items map[string]ttlEntry[T]
}

type ttlEntry[T any] struct {
	value     T
	expiresAt time.Time
}

// NewTTLCache builds a cache with the given TTL (defaulting to 60s when <= 0).
func NewTTLCache[T any](ttl time.Duration) *TTLCache[T] {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &TTLCache[T]{
		ttl:   ttl,
		items: make(map[string]ttlEntry[T]),
	}
}

// Get returns the value for key and whether it was present and unexpired. An
// expired entry is evicted on read.
func (c *TTLCache[T]) Get(key string) (T, bool) {
	var zero T
	if c == nil || key == "" {
		return zero, false
	}
	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return zero, false
	}
	if time.Now().After(entry.expiresAt) {
		c.Delete(key)
		return zero, false
	}
	return entry.value, true
}

// Set stores value under key with a fresh TTL.
func (c *TTLCache[T]) Set(key string, value T) {
	if c == nil || key == "" {
		return
	}
	c.mu.Lock()
	c.items[key] = ttlEntry[T]{value: value, expiresAt: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

// Delete removes key.
func (c *TTLCache[T]) Delete(key string) {
	if c == nil || key == "" {
		return
	}
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

// Clear removes all entries.
func (c *TTLCache[T]) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	clear(c.items)
	c.mu.Unlock()
}
