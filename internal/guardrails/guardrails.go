// Package guardrails is Polaris's content-safety policy engine. It runs
// operator-configured detectors (PII, secrets, prompt injection, content terms)
// over request and response text and returns a verdict — observe (log only),
// redact (mask matches), or block. It is not Gin middleware: handlers call it
// with typed, already-parsed text so detection sees decoded content, not wire
// bytes. Everything is disabled by default; a policy must opt a route in.
package guardrails

import (
	"sort"
	"strings"
)

// Action is what a policy does when a detector matches.
type Action string

const (
	// ActionObserve logs findings but does not alter the request/response.
	ActionObserve Action = "observe"
	// ActionRedact masks matched spans in place.
	ActionRedact Action = "redact"
	// ActionBlock rejects the request/response.
	ActionBlock Action = "block"
)

// Phase is when a policy applies.
type Phase string

const (
	PhaseRequest  Phase = "request"
	PhaseResponse Phase = "response"
	PhaseBoth     Phase = "both"
)

func (p Phase) covers(target Phase) bool {
	return p == PhaseBoth || p == target
}

// Severity ranks a finding.
type Severity string

const (
	SeverityLow    Severity = "low"
	SeverityMedium Severity = "medium"
	SeverityHigh   Severity = "high"
)

// Finding is one detector match. Start/End are byte offsets into the evaluated
// text. The matched text itself is never stored on the finding (privacy).
type Finding struct {
	Detector string
	Type     string
	Start    int
	End      int
	Severity Severity
}

// DetectorSpec configures one detector within a policy.
type DetectorSpec struct {
	Name      string   // "pii", "secrets", "prompt_injection", "content", "webhook", "llm_judge"
	Types     []string // enabled sub-types (empty = detector default)
	Terms     []string // literal terms/patterns (content detector)
	Threshold float64  // score threshold (prompt_injection)

	// Remote-detector options.
	URL       string // webhook endpoint
	TimeoutMs int    // remote call timeout (webhook, llm_judge)
	Model     string // llm_judge model (provider/model)
	Prompt    string // llm_judge policy description appended to the classifier prompt
}

// Policy is a compiled guardrail policy.
type Policy struct {
	Name      string
	Phase     Phase
	Action    Action
	FailMode  string // "open" | "closed" (behavior when a remote detector errors)
	Detectors []DetectorSpec
}

// Verdict is the result of evaluating text against policies for one phase.
type Verdict struct {
	Action    Action    // strongest action triggered (empty = nothing matched)
	Blocked   bool      // true when a block policy matched
	Findings  []Finding // all findings, for audit/metrics
	Redacted  string    // redacted text when Action == ActionRedact
	PolicyHit string    // name of the policy that set the action
}

// actionRank orders actions by strength so the strongest wins.
func actionRank(a Action) int {
	switch a {
	case ActionBlock:
		return 3
	case ActionRedact:
		return 2
	case ActionObserve:
		return 1
	default:
		return 0
	}
}

// redact replaces each finding span with a [REDACTED:detector.type] token,
// applying from the end so earlier offsets stay valid. Overlapping findings are
// merged.
func redact(text string, findings []Finding) string {
	if len(findings) == 0 {
		return text
	}
	merged := mergeFindings(findings)
	var b strings.Builder
	prev := 0
	for _, f := range merged {
		if f.Start < prev || f.Start > len(text) || f.End > len(text) || f.End < f.Start {
			continue
		}
		b.WriteString(text[prev:f.Start])
		b.WriteString("[REDACTED:" + f.Detector + "." + f.Type + "]")
		prev = f.End
	}
	b.WriteString(text[prev:])
	return b.String()
}

// mergeFindings sorts findings by start and merges overlaps (keeping the first
// detector/type label for the merged span).
func mergeFindings(findings []Finding) []Finding {
	sorted := append([]Finding(nil), findings...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Start != sorted[j].Start {
			return sorted[i].Start < sorted[j].Start
		}
		return sorted[i].End > sorted[j].End
	})
	out := make([]Finding, 0, len(sorted))
	for _, f := range sorted {
		if n := len(out); n > 0 && f.Start <= out[n-1].End {
			if f.End > out[n-1].End {
				out[n-1].End = f.End
			}
			continue
		}
		out = append(out, f)
	}
	return out
}
