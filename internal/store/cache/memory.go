package cache

import (
	"context"
	"strconv"
	"sync"
	"time"
)

type Memory struct {
	items sync.Map
}

type memoryItem struct {
	mu        sync.Mutex
	value     string
	expiresAt time.Time
}

func NewMemory() *Memory {
	return &Memory{}
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
	return nil
}

func (i *memoryItem) expired(now time.Time) bool {
	return !i.expiresAt.IsZero() && now.After(i.expiresAt)
}
