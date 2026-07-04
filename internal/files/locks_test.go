package files

import (
	"sync"
	"testing"
)

func TestLockTableSerializesPerKey(t *testing.T) {
	lt := newLockTable()
	var counter int
	var peak int
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release := lt.acquire("same-key")
			defer release()
			// Inside the per-key lock, only one goroutine may be present.
			mu.Lock()
			counter++
			if counter > peak {
				peak = counter
			}
			mu.Unlock()
			mu.Lock()
			counter--
			mu.Unlock()
		}()
	}
	wg.Wait()
	if peak != 1 {
		t.Fatalf("concurrent holders of one key = %d, want 1", peak)
	}
}

func TestLockTableEvictsWhenIdle(t *testing.T) {
	lt := newLockTable()
	// Acquire and release many distinct keys sequentially; the table must not
	// grow (B5b): each entry is removed when its last holder releases.
	for i := 0; i < 1000; i++ {
		release := lt.acquire(string(rune('a'+i%26)) + "-" + string(rune('0'+i%10)))
		release()
	}
	if got := lt.size(); got != 0 {
		t.Fatalf("lock table size after all releases = %d, want 0 (unbounded growth)", got)
	}
}

func TestLockTableConcurrentDistinctKeys(t *testing.T) {
	lt := newLockTable()
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			<-start
			release := lt.acquire(string(rune('A' + n)))
			// hold briefly
			release()
		}(i)
	}
	close(start)
	wg.Wait()
	if got := lt.size(); got != 0 {
		t.Fatalf("lock table not drained: size = %d", got)
	}
}
