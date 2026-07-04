package cache

import (
	"context"
	"strconv"
	"sync"
	"time"
)

const defaultSweepInterval = time.Minute

type Memory struct {
	items sync.Map
	stop  chan struct{}
	stopO sync.Once
}

type memoryItem struct {
	mu        sync.Mutex
	value     string
	expiresAt time.Time
}

func NewMemory() *Memory {
	return newMemory(defaultSweepInterval)
}

func newMemory(sweepInterval time.Duration) *Memory {
	m := &Memory{stop: make(chan struct{})}
	if sweepInterval > 0 {
		go m.sweepLoop(sweepInterval)
	}
	return m
}

func (m *Memory) Get(_ context.Context, key string) (string, bool, error) {
	raw, ok := m.items.Load(key)
	if !ok {
		return "", false, nil
	}

	item := raw.(*memoryItem)
	item.mu.Lock()
	defer item.mu.Unlock()

	if item.expired(time.Now()) {
		m.items.Delete(key)
		return "", false, nil
	}
	return item.value, true, nil
}

func (m *Memory) Set(_ context.Context, key, value string, ttl time.Duration) error {
	item := &memoryItem{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
	m.items.Store(key, item)
	return nil
}

func (m *Memory) Increment(_ context.Context, key string, ttl time.Duration) (int64, error) {
	now := time.Now()
	raw, _ := m.items.LoadOrStore(key, &memoryItem{value: "0", expiresAt: now.Add(ttl)})
	item := raw.(*memoryItem)

	item.mu.Lock()
	defer item.mu.Unlock()

	if item.expired(now) {
		item.value = "0"
	}
	current, err := strconv.ParseInt(item.value, 10, 64)
	if err != nil {
		current = 0
	}
	current++
	item.value = strconv.FormatInt(current, 10)
	item.expiresAt = now.Add(ttl)
	return current, nil
}

func (m *Memory) Ping(context.Context) error {
	return nil
}

func (m *Memory) Close() error {
	m.stopO.Do(func() { close(m.stop) })
	return nil
}

// sweepLoop periodically evicts expired entries. Without it, keys that embed a
// rotating component (rate-limit window starts) accumulate forever, since Get
// only evicts the specific key it reads (B5a).
func (m *Memory) sweepLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case now := <-ticker.C:
			m.sweep(now)
		}
	}
}

func (m *Memory) sweep(now time.Time) {
	m.items.Range(func(key, raw any) bool {
		item := raw.(*memoryItem)
		item.mu.Lock()
		expired := item.expired(now)
		item.mu.Unlock()
		if expired {
			// CompareAndDelete only removes the exact entry we examined, so a
			// concurrent Set that replaced it is preserved.
			m.items.CompareAndDelete(key, raw)
		}
		return true
	})
}

func (i *memoryItem) expired(now time.Time) bool {
	return !i.expiresAt.IsZero() && now.After(i.expiresAt)
}
