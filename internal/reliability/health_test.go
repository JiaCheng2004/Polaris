package reliability

import (
	"testing"
	"time"
)

func TestHealthWindowErrorRate(t *testing.T) {
	h := newHealthWindow()
	now := time.Unix(1000, 0)
	for i := 0; i < 6; i++ {
		h.record(true, 100*time.Millisecond, now)
	}
	for i := 0; i < 4; i++ {
		h.record(false, 100*time.Millisecond, now)
	}
	samples, errRate, _ := h.stats(now)
	if samples != 10 {
		t.Fatalf("samples = %d, want 10", samples)
	}
	if errRate < 0.399 || errRate > 0.401 {
		t.Fatalf("errRate = %f, want ~0.4", errRate)
	}
}

func TestHealthWindowDecays(t *testing.T) {
	h := newHealthWindow()
	now := time.Unix(2000, 0)
	for i := 0; i < 10; i++ {
		h.record(false, time.Second, now)
	}
	if s, _, _ := h.stats(now); s != 10 {
		t.Fatalf("samples = %d, want 10", s)
	}
	// After the full window elapses, all buckets expire.
	later := now.Add((healthWindowSeconds + 1) * time.Second)
	samples, errRate, _ := h.stats(later)
	if samples != 0 || errRate != 0 {
		t.Fatalf("after decay samples=%d errRate=%f, want 0/0", samples, errRate)
	}
}

func TestHealthWindowEWMA(t *testing.T) {
	h := newHealthWindow()
	now := time.Unix(3000, 0)
	_, _, ewma := h.record(true, 100*time.Millisecond, now)
	if ewma != 100 {
		t.Fatalf("first ewma = %f, want 100", ewma)
	}
	// EWMA moves toward new sample by alpha.
	_, _, ewma = h.record(true, 200*time.Millisecond, now)
	want := latencyAlpha*200 + (1-latencyAlpha)*100
	if ewma < want-0.01 || ewma > want+0.01 {
		t.Fatalf("ewma = %f, want %f", ewma, want)
	}
}

func TestHealthScore(t *testing.T) {
	// Perfect health.
	if s := healthScore(0, 50); s < 0.98 {
		t.Fatalf("healthy score = %f, want ~1", s)
	}
	// All failures = 0.
	if s := healthScore(1, 50); s != 0 {
		t.Fatalf("all-fail score = %f, want 0", s)
	}
	// Latency penalizes but never below success bound.
	high := healthScore(0, 50)
	low := healthScore(0, 10000)
	if low >= high {
		t.Fatalf("latency did not penalize: low=%f high=%f", low, high)
	}
	if low < 0 || high > 1 {
		t.Fatalf("score out of [0,1]: low=%f high=%f", low, high)
	}
}
