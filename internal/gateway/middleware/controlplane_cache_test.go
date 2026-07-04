package middleware

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/store"
)

type policyCountingStore struct {
	store.Store
	calls    int32
	policies []store.Policy
}

func (s *policyCountingStore) ListPolicies(_ context.Context, _ string) ([]store.Policy, error) {
	atomic.AddInt32(&s.calls, 1)
	return s.policies, nil
}

func TestAggregateProjectPoliciesCaches(t *testing.T) {
	fake := &policyCountingStore{policies: []store.Policy{{AllowedModels: []string{"openai/gpt-4o"}}}}
	cache := store.NewTTLCache[ProjectPolicies](time.Minute)

	models1, _, _, _, err := aggregateProjectPolicies(context.Background(), fake, "proj_1", cache)
	if err != nil {
		t.Fatalf("aggregate error = %v", err)
	}
	if len(models1) != 1 || models1[0] != "openai/gpt-4o" {
		t.Fatalf("models = %v", models1)
	}

	// Second call for the same project must hit the cache (no extra ListPolicies).
	models2, _, _, _, _ := aggregateProjectPolicies(context.Background(), fake, "proj_1", cache)
	if len(models2) != 1 || models2[0] != "openai/gpt-4o" {
		t.Fatalf("cached models = %v", models2)
	}
	if got := atomic.LoadInt32(&fake.calls); got != 1 {
		t.Fatalf("ListPolicies called %d times, want 1 (second call should be cached)", got)
	}

	// A different project is a cache miss.
	_, _, _, _, _ = aggregateProjectPolicies(context.Background(), fake, "proj_2", cache)
	if got := atomic.LoadInt32(&fake.calls); got != 2 {
		t.Fatalf("ListPolicies called %d times, want 2 (distinct project)", got)
	}
}

func TestAggregateProjectPoliciesCachesEmpty(t *testing.T) {
	fake := &policyCountingStore{policies: nil}
	cache := store.NewTTLCache[ProjectPolicies](time.Minute)

	_, _, _, _, _ = aggregateProjectPolicies(context.Background(), fake, "proj", cache)
	_, _, _, _, _ = aggregateProjectPolicies(context.Background(), fake, "proj", cache)
	if got := atomic.LoadInt32(&fake.calls); got != 1 {
		t.Fatalf("empty policy result should be cached; ListPolicies called %d times, want 1", got)
	}
}
