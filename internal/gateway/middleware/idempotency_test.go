package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/gin-gonic/gin"
)

type fakeIdemStore struct {
	store.Store
	mu sync.Mutex
	m  map[string]store.IdempotencyKey
}

func newFakeIdemStore() *fakeIdemStore {
	return &fakeIdemStore{m: map[string]store.IdempotencyKey{}}
}

func (f *fakeIdemStore) key(k, p, e string) string { return k + "\x00" + p + "\x00" + e }

func (f *fakeIdemStore) CheckIdempotencyKey(_ context.Context, k, p, e string) (*store.IdempotencyKey, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.m[f.key(k, p, e)]
	if !ok {
		return nil, false, nil
	}
	return &rec, true, nil
}

func (f *fakeIdemStore) PutIdempotencyKey(_ context.Context, rec store.IdempotencyKey) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := f.key(rec.Key, rec.ProjectID, rec.Endpoint)
	if _, exists := f.m[k]; !exists { // DO NOTHING semantics
		f.m[k] = rec
	}
	return nil
}

func (f *fakeIdemStore) PurgeExpiredIdempotencyKeys(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func newIdemTestEngine(st store.Store, calls *int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := config.Default()
	holder := gwruntime.NewHolder(&cfg, nil)
	coord := NewIdempotencyCoordinator()
	r := gin.New()
	r.POST("/job",
		func(c *gin.Context) {
			SetRuntimeSnapshot(c, holder.Current())
			SetAuthContext(c, AuthContext{ProjectID: "proj"})
			c.Next()
		},
		Idempotency(holder, st, coord, nil, "/v1/job"),
		func(c *gin.Context) {
			*calls++
			c.JSON(http.StatusOK, gin.H{"id": "job-1", "n": *calls})
		},
	)
	return r
}

func doIdem(r *gin.Engine, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/job", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestIdempotencyReplaysStoredResponse(t *testing.T) {
	calls := 0
	r := newIdemTestEngine(newFakeIdemStore(), &calls)

	w1 := doIdem(r, "abc", `{"x":1}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("first status = %d", w1.Code)
	}
	w2 := doIdem(r, "abc", `{"x":1}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("replay status = %d", w2.Code)
	}
	if calls != 1 {
		t.Fatalf("handler ran %d times, want 1 (second must replay)", calls)
	}
	if w2.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatal("replay missing Idempotency-Replayed header")
	}
	if w1.Body.String() != w2.Body.String() {
		t.Fatalf("replay body %q != original %q", w2.Body.String(), w1.Body.String())
	}
}

func TestIdempotencyMismatchReturns409(t *testing.T) {
	calls := 0
	r := newIdemTestEngine(newFakeIdemStore(), &calls)

	if w := doIdem(r, "abc", `{"x":1}`); w.Code != http.StatusOK {
		t.Fatalf("first status = %d", w.Code)
	}
	w := doIdem(r, "abc", `{"x":2}`) // same key, different body
	if w.Code != http.StatusConflict {
		t.Fatalf("mismatch status = %d, want 409", w.Code)
	}
	if !strings.Contains(w.Body.String(), "idempotency_key_reuse") {
		t.Fatalf("409 body missing code: %s", w.Body.String())
	}
	if calls != 1 {
		t.Fatalf("handler ran %d times on mismatch, want 1", calls)
	}
}

func TestIdempotencyNoHeaderPassesThrough(t *testing.T) {
	calls := 0
	r := newIdemTestEngine(newFakeIdemStore(), &calls)
	doIdem(r, "", `{"x":1}`)
	doIdem(r, "", `{"x":1}`)
	if calls != 2 {
		t.Fatalf("handler ran %d times without a key, want 2 (no idempotency)", calls)
	}
}

func TestIdempotencyConcurrentDuplicatesCollapse(t *testing.T) {
	calls := 0
	var callMu sync.Mutex
	gin.SetMode(gin.TestMode)
	cfg := config.Default()
	holder := gwruntime.NewHolder(&cfg, nil)
	coord := NewIdempotencyCoordinator()
	st := newFakeIdemStore()
	r := gin.New()
	r.POST("/job",
		func(c *gin.Context) {
			SetRuntimeSnapshot(c, holder.Current())
			SetAuthContext(c, AuthContext{ProjectID: "proj"})
			c.Next()
		},
		Idempotency(holder, st, coord, nil, "/v1/job"),
		func(c *gin.Context) {
			callMu.Lock()
			calls++
			callMu.Unlock()
			time.Sleep(10 * time.Millisecond)
			c.JSON(http.StatusOK, gin.H{"id": "job-1"})
		},
	)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			doIdem(r, "same", `{"x":1}`)
		}()
	}
	wg.Wait()
	if calls != 1 {
		t.Fatalf("concurrent duplicates ran the handler %d times, want 1", calls)
	}
}
