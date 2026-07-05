package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/store"
)

func TestSQLiteIdempotency(t *testing.T) {
	ctx := context.Background()
	st, err := New(config.StoreConfig{
		Driver:           "sqlite",
		DSN:              filepath.Join(t.TempDir(), "p.db"),
		MaxConnections:   1,
		LogRetentionDays: 30,
		LogBufferSize:    10,
		LogFlushInterval: time.Second,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = st.Close() }()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Absent key.
	if _, ok, err := st.CheckIdempotencyKey(ctx, "k", "p", "/e"); err != nil || ok {
		t.Fatalf("absent key: ok=%v err=%v", ok, err)
	}

	// Store then read back.
	now := time.Now().UTC()
	rec := store.IdempotencyKey{Key: "k", ProjectID: "p", Endpoint: "/e", RequestHash: "h", ResponseStatus: 200, ResponseBody: []byte(`{"id":"1"}`), CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := st.PutIdempotencyKey(ctx, rec); err != nil {
		t.Fatalf("PutIdempotencyKey() error = %v", err)
	}
	got, ok, err := st.CheckIdempotencyKey(ctx, "k", "p", "/e")
	if err != nil || !ok {
		t.Fatalf("present key: ok=%v err=%v", ok, err)
	}
	if got.RequestHash != "h" || got.ResponseStatus != 200 || string(got.ResponseBody) != `{"id":"1"}` {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	// ON CONFLICT DO NOTHING: a second put with a different hash keeps the first.
	_ = st.PutIdempotencyKey(ctx, store.IdempotencyKey{Key: "k", ProjectID: "p", Endpoint: "/e", RequestHash: "other", ResponseStatus: 500, CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
	got2, _, _ := st.CheckIdempotencyKey(ctx, "k", "p", "/e")
	if got2.RequestHash != "h" {
		t.Fatalf("conflict overwrote existing record: hash=%s", got2.RequestHash)
	}

	// Scoping: same key under a different project/endpoint misses.
	if _, ok, _ := st.CheckIdempotencyKey(ctx, "k", "other-project", "/e"); ok {
		t.Fatal("cross-project idempotency leak")
	}

	// Expired is treated as absent and purged.
	expired := store.IdempotencyKey{Key: "e2", ProjectID: "p", Endpoint: "/e", RequestHash: "h", ResponseStatus: 200, CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour)}
	if err := st.PutIdempotencyKey(ctx, expired); err != nil {
		t.Fatalf("put expired: %v", err)
	}
	if _, ok, _ := st.CheckIdempotencyKey(ctx, "e2", "p", "/e"); ok {
		t.Fatal("expired key returned as present")
	}
	n, err := st.PurgeExpiredIdempotencyKeys(ctx, now)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n < 1 {
		t.Fatalf("purged %d rows, want ≥1", n)
	}
}
