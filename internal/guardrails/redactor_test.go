package guardrails

import (
	"strings"
	"testing"
)

func redactStream(r *StreamRedactor, text string, chunkSize int) (string, bool) {
	if chunkSize < 1 {
		chunkSize = 1
	}
	var out strings.Builder
	for i := 0; i < len(text); i += chunkSize {
		end := i + chunkSize
		if end > len(text) {
			end = len(text)
		}
		emit, blocked := r.Process(text[i:end])
		if blocked {
			return out.String(), true
		}
		out.WriteString(emit)
	}
	emit, blocked := r.Flush()
	out.WriteString(emit)
	return out.String(), blocked
}

func redactPolicies() []Policy {
	return []Policy{{Name: "p", Phase: PhaseResponse, Action: ActionRedact, Detectors: []DetectorSpec{{Name: "pii"}, {Name: "secrets"}}}}
}

func TestStreamRedactorNoLeakAtAnyBoundary(t *testing.T) {
	text := "please reach bob@example.com and use key AKIAIOSFODNN7EXAMPLE right away"
	for chunk := 1; chunk <= len(text); chunk++ {
		r := NewStreamRedactor(NewEngine(nil), redactPolicies(), 64)
		out, blocked := redactStream(r, text, chunk)
		if blocked {
			t.Fatalf("chunk=%d unexpectedly blocked", chunk)
		}
		if strings.Contains(out, "bob@example.com") {
			t.Fatalf("email leaked at chunk=%d: %q", chunk, out)
		}
		if strings.Contains(out, "AKIAIOSFODNN7EXAMPLE") {
			t.Fatalf("secret leaked at chunk=%d: %q", chunk, out)
		}
		if !strings.Contains(out, "[REDACTED:pii.email]") || !strings.Contains(out, "[REDACTED:secrets.aws_access_key]") {
			t.Fatalf("expected redaction markers at chunk=%d: %q", chunk, out)
		}
		// Non-sensitive text is preserved.
		if !strings.Contains(out, "please reach ") || !strings.Contains(out, " right away") {
			t.Fatalf("chunk=%d dropped benign text: %q", chunk, out)
		}
	}
}

func TestStreamRedactorPassthroughWhenObserve(t *testing.T) {
	r := NewStreamRedactor(NewEngine(nil), []Policy{{Name: "p", Phase: PhaseResponse, Action: ActionObserve, Detectors: []DetectorSpec{{Name: "pii"}}}}, 8)
	if r.Active() {
		t.Fatal("observe-only redactor should be inactive")
	}
	out, _ := redactStream(r, "email bob@example.com", 3)
	if out != "email bob@example.com" {
		t.Fatalf("observe passthrough altered text: %q", out)
	}
}

func TestStreamRedactorBlock(t *testing.T) {
	r := NewStreamRedactor(NewEngine(nil), []Policy{{Name: "p", Phase: PhaseResponse, Action: ActionBlock, Detectors: []DetectorSpec{{Name: "pii"}}}}, 64)
	_, blocked := redactStream(r, "here is an email bob@example.com in the stream", 4)
	if !blocked {
		t.Fatal("block policy did not block the stream")
	}
}

func TestStreamRedactorNilSafe(t *testing.T) {
	var r *StreamRedactor
	if r.Active() {
		t.Fatal("nil redactor should be inactive")
	}
	out, blocked := r.Process("hello")
	if out != "hello" || blocked {
		t.Fatalf("nil redactor Process = %q,%v", out, blocked)
	}
}

func FuzzStreamRedactorNoLeak(f *testing.F) {
	for _, seed := range []int{1, 2, 3, 5, 7, 13} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, chunkSize int) {
		if chunkSize < 1 || chunkSize > 128 {
			chunkSize = 1
		}
		text := "intro reach bob@example.com then key AKIAIOSFODNN7EXAMPLE end"
		r := NewStreamRedactor(NewEngine(nil), redactPolicies(), 64)
		out, _ := redactStream(r, text, chunkSize)
		if strings.Contains(out, "bob@example.com") {
			t.Fatalf("email leaked at chunkSize=%d: %q", chunkSize, out)
		}
		if strings.Contains(out, "AKIAIOSFODNN7EXAMPLE") {
			t.Fatalf("secret leaked at chunkSize=%d: %q", chunkSize, out)
		}
	})
}
