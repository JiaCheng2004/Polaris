package middleware

import (
	"time"

	"github.com/JiaCheng2004/Polaris/internal/store"
)

// APIKeyCache caches resolved API keys by hash. It is a thin pointer-returning
// wrapper over the generic store.TTLCache.
type APIKeyCache struct {
	inner *store.TTLCache[store.APIKey]
}

func NewAPIKeyCache(ttl time.Duration) *APIKeyCache {
	return &APIKeyCache{inner: store.NewTTLCache[store.APIKey](ttl)}
}

func (c *APIKeyCache) Get(hash string) (*store.APIKey, bool) {
	if c == nil {
		return nil, false
	}
	value, ok := c.inner.Get(hash)
	if !ok {
		return nil, false
	}
	return &value, true
}

func (c *APIKeyCache) Set(hash string, key *store.APIKey) {
	if c == nil || key == nil {
		return
	}
	c.inner.Set(hash, *key)
}

func (c *APIKeyCache) Delete(hash string) {
	if c != nil {
		c.inner.Delete(hash)
	}
}

func (c *APIKeyCache) Clear() {
	if c != nil {
		c.inner.Clear()
	}
}
