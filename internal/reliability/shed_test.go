package reliability

import (
	"testing"
	"time"
)

func TestRetryBudgetColdAllowsMinTokens(t *testing.T) {
	b := newRetryBudget(0.2)
	now := time.Unix(0, 0)
	// From cold (no successes) the floor lets a bounded number through.
	allowed := 0
	for i := 0; i < int(retryBudgetMinTokens)+50; i++ {
		if b.allowRetry(now) {
			allowed++
		}
	}
	if allowed < int(retryBudgetMinTokens) || allowed > int(retryBudgetMinTokens)+1 {
		t.Fatalf("cold budget allowed %d, want ~%d", allowed, int(retryBudgetMinTokens))
	}
}

func TestRetryBudgetReplenishesWithSuccess(t *testing.T) {
	b := newRetryBudget(0.5)
	now := time.Unix(0, 0)
	// Exhaust the floor.
	for b.allowRetry(now) {
	}
	if b.allowRetry(now) {
		t.Fatal("budget not exhausted")
	}
	// 100 successes at ratio 0.5 add 50 tokens of headroom.
	for i := 0; i < 100; i++ {
		b.recordSuccess(now)
	}
	allowed := 0
	for i := 0; i < 60; i++ {
		if b.allowRetry(now) {
			allowed++
		}
	}
	if allowed < 45 || allowed > 55 {
		t.Fatalf("after 100 successes @0.5 allowed %d more, want ~50", allowed)
	}
}

func TestRetryBudgetDecays(t *testing.T) {
	b := newRetryBudget(0.2)
	now := time.Unix(0, 0)
	for i := 0; i < 1000; i++ {
		b.recordSuccess(now)
	}
	for i := 0; i < 100; i++ {
		b.allowRetry(now)
	}
	// After several half-lives the retry counter decays back toward the floor.
	later := now.Add(10 * retryBudgetHalfLife)
	if !b.allowRetry(later) {
		t.Fatal("budget did not recover after decay")
	}
}

func TestRetryBudgetReconfigure(t *testing.T) {
	b := newRetryBudget(0.2)
	b.reconfigure(0.9)
	if b.ratio != 0.9 {
		t.Fatalf("ratio = %f, want 0.9", b.ratio)
	}
	b.reconfigure(0) // invalid falls back to default
	if b.ratio != 0.2 {
		t.Fatalf("ratio = %f, want 0.2 default", b.ratio)
	}
}
