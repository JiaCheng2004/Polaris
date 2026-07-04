package files

import "sync"

// lockTable is a keyed mutex with reference counting: it serializes work per key
// (a file+provider materialization) while keeping the map bounded by the number
// of *concurrent* keys, not the total number of keys ever seen. The previous
// implementation used a package-global sync.Map that never evicted, so it grew
// unbounded over the process lifetime (B5b).
type lockTable struct {
	mu    sync.Mutex
	locks map[string]*refLock
}

type refLock struct {
	mu   sync.Mutex
	refs int
}

func newLockTable() *lockTable {
	return &lockTable{locks: make(map[string]*refLock)}
}

// acquire locks key and returns a release function. The entry is removed once no
// goroutine holds or is waiting on it.
func (t *lockTable) acquire(key string) func() {
	t.mu.Lock()
	rl, ok := t.locks[key]
	if !ok {
		rl = &refLock{}
		t.locks[key] = rl
	}
	rl.refs++
	t.mu.Unlock()

	rl.mu.Lock()
	return func() {
		rl.mu.Unlock()
		t.mu.Lock()
		rl.refs--
		if rl.refs == 0 {
			delete(t.locks, key)
		}
		t.mu.Unlock()
	}
}

// size reports the number of live lock entries (test/metric helper).
func (t *lockTable) size() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.locks)
}

var materializeLocks = newLockTable()
