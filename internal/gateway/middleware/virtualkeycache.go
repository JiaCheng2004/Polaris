package middleware

import (
	"sync"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/store"
)

type VirtualKeyCache struct {
	ttl   time.Duration
	mu    sync.RWMutex
	items map[string]cachedVirtualKey
}

type cachedVirtualKey struct {
	key       store.VirtualKey
	expiresAt time.Time
}

func NewVirtualKeyCache(ttl time.Duration) *VirtualKeyCache {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &VirtualKeyCache{
		ttl:   ttl,
		items: make(map[string]cachedVirtualKey),
	}
}

func (c *VirtualKeyCache) Get(hash string) (*store.VirtualKey, bool) {
	if c == nil || hash == "" {
		return nil, false
	}
	c.mu.RLock()
	entry, ok := c.items[hash]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		c.Delete(hash)
		return nil, false
	}
	key := entry.key
	return &key, true
}

func (c *VirtualKeyCache) Set(hash string, key *store.VirtualKey) {
	if c == nil || hash == "" || key == nil {
		return
	}
	copyKey := *key
	c.mu.Lock()
	c.items[hash] = cachedVirtualKey{
		key:       copyKey,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}

func (c *VirtualKeyCache) Delete(hash string) {
	if c == nil || hash == "" {
		return
	}
	c.mu.Lock()
	delete(c.items, hash)
	c.mu.Unlock()
}

func (c *VirtualKeyCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	clear(c.items)
	c.mu.Unlock()
}
