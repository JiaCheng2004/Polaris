package guardrails

import "strings"

// contentDetector flags operator-configured literal terms (case-insensitive).
// No ML classification is claimed — it is a term/phrase list.
type contentDetector struct{}

func newContentDetector() *contentDetector { return &contentDetector{} }

func (d *contentDetector) Name() string { return "content" }

func (d *contentDetector) Detect(text string, spec DetectorSpec) []Finding {
	if len(spec.Terms) == 0 {
		return nil
	}
	lower := strings.ToLower(text)
	var findings []Finding
	for _, term := range spec.Terms {
		t := strings.ToLower(strings.TrimSpace(term))
		if t == "" {
			continue
		}
		from := 0
		for {
			idx := strings.Index(lower[from:], t)
			if idx < 0 {
				break
			}
			start := from + idx
			findings = append(findings, Finding{Detector: "content", Type: "term", Start: start, End: start + len(t), Severity: SeverityMedium})
			from = start + len(t)
		}
	}
	return findings
}
