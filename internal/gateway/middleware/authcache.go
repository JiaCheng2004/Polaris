package middleware

import (
	"sync"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/store"
)

type APIKeyCache struct {
	ttl   time.Duration
	mu    sync.RWMutex
	items map[string]cachedAPIKey
}

type cachedAPIKey struct {
	key       store.APIKey
	expiresAt time.Time
}

func NewAPIKeyCache(ttl time.Duration) *APIKeyCache {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &APIKeyCache{
		ttl:   ttl,
		items: make(map[string]cachedAPIKey),
	}
}

func (c *APIKeyCache) Get(hash string) (*store.APIKey, bool) {
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

func (c *APIKeyCache) Set(hash string, key *store.APIKey) {
	if c == nil || hash == "" || key == nil {
		return
	}

	copyKey := *key

	c.mu.Lock()
	c.items[hash] = cachedAPIKey{
		key:       copyKey,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}

func (c *APIKeyCache) Delete(hash string) {
	if c == nil || hash == "" {
		return
	}

	c.mu.Lock()
	delete(c.items, hash)
	c.mu.Unlock()
}

func (c *APIKeyCache) Clear() {
	if c == nil {
		return
	}

	c.mu.Lock()
	clear(c.items)
	c.mu.Unlock()
}
