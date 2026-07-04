package middleware

import (
	"time"

	"github.com/JiaCheng2004/Polaris/internal/store"
)

// VirtualKeyCache caches resolved virtual keys by hash. It is a thin
// pointer-returning wrapper over the generic store.TTLCache.
type VirtualKeyCache struct {
	inner *store.TTLCache[store.VirtualKey]
}

func NewVirtualKeyCache(ttl time.Duration) *VirtualKeyCache {
	return &VirtualKeyCache{inner: store.NewTTLCache[store.VirtualKey](ttl)}
}

func (c *VirtualKeyCache) Get(hash string) (*store.VirtualKey, bool) {
	if c == nil {
		return nil, false
	}
	value, ok := c.inner.Get(hash)
	if !ok {
		return nil, false
	}
	return &value, true
}

func (c *VirtualKeyCache) Set(hash string, key *store.VirtualKey) {
	if c == nil || key == nil {
		return
	}
	c.inner.Set(hash, *key)
}

func (c *VirtualKeyCache) Delete(hash string) {
	if c != nil {
		c.inner.Delete(hash)
	}
}

func (c *VirtualKeyCache) Clear() {
	if c != nil {
		c.inner.Clear()
	}
}
