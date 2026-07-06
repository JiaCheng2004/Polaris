package guardrails

import (
	"context"
	"regexp"
	"unicode"
	"unicode/utf8"
)

type promptInjectionDetector struct {
	override  *regexp.Regexp
	jailbreak *regexp.Regexp
	roleplay  *regexp.Regexp
}

func newPromptInjectionDetector() *promptInjectionDetector {
	// Each group is a single combined regex (one full-text scan per group) so the
	// detector stays well under the 1ms/4KB budget.
	return &promptInjectionDetector{
		override: regexp.MustCompile(`(?i)(?:` +
			`ignore\s+(?:all\s+)?(?:previous|prior|above|earlier)\s+(?:instructions?|prompts?|messages?)|` +
			`disregard\s+(?:your\s+|the\s+|all\s+)?(?:system\s+)?(?:prompt|instructions?|rules?)|` +
			`forget\s+(?:all\s+)?(?:previous|prior|your|the)\s+(?:instructions?|training|rules?)|` +
			`ignora(?:r)?\s+(?:las\s+|todas\s+las\s+)?instrucciones|` + // Spanish
			`ignore[rz]?\s+(?:les\s+|toutes\s+les\s+)?instructions|` + // French
			`vergiss\s+(?:alle\s+|die\s+)?(?:vorherigen\s+)?anweisungen` + // German
			`)`),
		jailbreak: regexp.MustCompile(`\bDAN\b|(?i:developer\s+mode|jailbreak|do\s+anything\s+now|unfiltered\s+(?:mode|response))`),
		roleplay: regexp.MustCompile(`(?i)(?:` +
			`\byou\s+are\s+now\s+(?:an?\s+)?\w+|` +
			`\bact\s+as\s+(?:a|an|if|though)\b|` +
			`\bpretend\s+(?:to\s+be|you\s+are|that)|` +
			`\bfrom\s+now\s+on,?\s+you\b` +
			`)`),
	}
}

func (d *promptInjectionDetector) Name() string { return "prompt_injection" }

func (d *promptInjectionDetector) Detect(_ context.Context, text string, spec DetectorSpec) ([]Finding, error) {
	threshold := spec.Threshold
	if threshold <= 0 {
		threshold = 0.5
	}
	var findings []Finding
	score := 0.0

	scan := func(re *regexp.Regexp, typ string, weight float64) {
		for _, loc := range re.FindAllStringIndex(text, -1) {
			findings = append(findings, Finding{Detector: "prompt_injection", Type: typ, Start: loc[0], End: loc[1], Severity: SeverityHigh})
			score += weight
		}
	}
	scan(d.override, "instruction_override", 0.6)
	scan(d.jailbreak, "jailbreak", 0.6)
	scan(d.roleplay, "role_confusion", 0.35)

	if inv := invisibleUnicodeSpans(text); len(inv) > 0 {
		findings = append(findings, inv...)
		score += 0.5
	}

	if score < threshold {
		return nil, nil
	}
	return findings, nil
}

// invisibleUnicodeSpans flags runs of Unicode format characters (Cf category:
// zero-width joiners, bidi overrides, etc.) commonly used to smuggle hidden
// instructions.
func invisibleUnicodeSpans(text string) []Finding {
	var findings []Finding
	runStart, inRun := -1, false
	for i, r := range text {
		// Fast path: ASCII has no format/bidi-control characters, so skip the
		// (relatively expensive) range-table lookups for the common case.
		invisible := r >= utf8.RuneSelf && (unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Bidi_Control, r))
		if invisible {
			if !inRun {
				runStart, inRun = i, true
			}
		} else if inRun {
			findings = append(findings, Finding{Detector: "prompt_injection", Type: "invisible_unicode", Start: runStart, End: i, Severity: SeverityMedium})
			inRun = false
		}
	}
	if inRun {
		findings = append(findings, Finding{Detector: "prompt_injection", Type: "invisible_unicode", Start: runStart, End: len(text), Severity: SeverityMedium})
	}
	return findings
}
