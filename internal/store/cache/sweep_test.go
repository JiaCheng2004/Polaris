package cache

import (
	"context"
	"testing"
	"time"
)

func countItems(m *Memory) int {
	count := 0
	m.items.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

func TestMemorySweepEvictsExpired(t *testing.T) {
	m := newMemory(5 * time.Millisecond)
	defer func() { _ = m.Close() }()

	ctx := context.Background()
	if err := m.Set(ctx, "short", "v", time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := m.Set(ctx, "long", "v", time.Hour); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if countItems(m) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := countItems(m); got != 1 {
		t.Fatalf("after sweep, item count = %d, want 1 (only the long-TTL entry)", got)
	}
	if _, ok, _ := m.Get(ctx, "long"); !ok {
		t.Fatal("long-TTL entry should survive the sweep")
	}
}

func TestMemoryCloseStopsSweeperIdempotently(t *testing.T) {
	m := newMemory(time.Millisecond)
	if err := m.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("second Close() = %v", err)
	}
}
