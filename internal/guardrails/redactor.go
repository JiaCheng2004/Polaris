package guardrails

import "unicode/utf8"

// defaultHoldback is the number of trailing runes buffered between chunks so a
// pattern that straddles a chunk boundary is caught before any part of it is
// emitted.
const defaultHoldback = 64

// StreamRedactor applies response-phase policies to a token stream. It buffers a
// hold-back window so a match spanning chunk boundaries is never partially
// emitted, then redacts or blocks. When the matched policies are observe-only it
// passes text through without buffering.
type StreamRedactor struct {
	engine    *Engine
	policies  []Policy
	holdback  int
	buffer    string
	enforcing bool // any policy redacts or blocks
}

// NewStreamRedactor builds a redactor for the response phase. A nil engine or no
// enforcing policy yields a pass-through redactor.
func NewStreamRedactor(engine *Engine, policies []Policy, holdback int) *StreamRedactor {
	if holdback <= 0 {
		holdback = defaultHoldback
	}
	enforcing := false
	for _, p := range policies {
		if !p.Phase.covers(PhaseResponse) {
			continue
		}
		if p.Action == ActionRedact || p.Action == ActionBlock {
			enforcing = true
			break
		}
	}
	return &StreamRedactor{engine: engine, policies: policies, holdback: holdback, enforcing: enforcing}
}

// Active reports whether the redactor will buffer/transform the stream.
func (r *StreamRedactor) Active() bool {
	return r != nil && r.engine != nil && r.enforcing && len(r.policies) > 0
}

// Process consumes the next delta and returns the safe-to-emit (redacted) text
// so far, plus whether the stream is blocked (a block policy matched).
func (r *StreamRedactor) Process(delta string) (string, bool) {
	if !r.Active() {
		return delta, false
	}
	r.buffer += delta
	return r.drain(false)
}

// Flush returns any remaining buffered text (redacted) at stream end.
func (r *StreamRedactor) Flush() (string, bool) {
	if !r.Active() {
		return "", false
	}
	return r.drain(true)
}

func (r *StreamRedactor) drain(final bool) (string, bool) {
	verdict := r.engine.Evaluate(PhaseResponse, r.buffer, r.policies)
	if verdict.Blocked {
		return "", true
	}

	// Emit everything except the trailing hold-back window (unless flushing).
	emitLen := len(r.buffer)
	if !final {
		emitLen = len(r.buffer) - holdbackBytes(r.buffer, r.holdback)
		if emitLen < 0 {
			emitLen = 0
		}
	}

	// Never emit past a finding that straddles the emit boundary — hold it whole.
	boundary := emitLen
	for _, f := range verdict.Findings {
		if f.Start < boundary && f.End > emitLen {
			boundary = f.Start
		}
	}
	if boundary <= 0 {
		return "", false
	}

	head := r.buffer[:boundary]
	var relevant []Finding
	if verdict.Action == ActionRedact {
		for _, f := range verdict.Findings {
			if f.End <= boundary {
				relevant = append(relevant, f)
			}
		}
	}
	out := head
	if len(relevant) > 0 {
		out = redact(head, relevant)
	}
	r.buffer = r.buffer[boundary:]
	return out, false
}

// holdbackBytes returns the byte length of the last n runes of s.
func holdbackBytes(s string, n int) int {
	bytes := 0
	count := 0
	for i := len(s); i > 0; {
		_, size := utf8.DecodeLastRuneInString(s[:i])
		i -= size
		bytes += size
		if count++; count >= n {
			break
		}
	}
	return bytes
}
