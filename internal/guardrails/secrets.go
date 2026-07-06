package guardrails

import (
	"context"
	"math"
	"regexp"
)

type secretsDetector struct {
	patterns map[string]*regexp.Regexp
	order    []string
}

func newSecretsDetector() *secretsDetector {
	pats := map[string]*regexp.Regexp{
		"aws_access_key": regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`),
		"github_pat":     regexp.MustCompile(`\b(?:ghp_|github_pat_)[A-Za-z0-9_]{20,255}`),
		"slack_token":    regexp.MustCompile(`\bxox[baprs]-[0-9A-Za-z\-]{10,}`),
		"jwt":            regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{6,}\.[A-Za-z0-9_\-]{6,}\.[A-Za-z0-9_\-]{6,}`),
		"pem":            regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
		"gcp_sa":         regexp.MustCompile(`"type"\s*:\s*"service_account"`),
	}
	return &secretsDetector{
		patterns: pats,
		// high_entropy is intentionally not a default (noisy); opt in via types.
		order: []string{"aws_access_key", "github_pat", "slack_token", "jwt", "pem", "gcp_sa"},
	}
}

func (d *secretsDetector) Name() string { return "secrets" }

func (d *secretsDetector) Detect(_ context.Context, text string, spec DetectorSpec) ([]Finding, error) {
	enabled := typeSet(spec.Types, d.order...)
	var findings []Finding
	for _, typ := range d.order {
		if !enabled[typ] {
			continue
		}
		for _, loc := range d.patterns[typ].FindAllStringIndex(text, -1) {
			findings = append(findings, Finding{Detector: "secrets", Type: typ, Start: loc[0], End: loc[1], Severity: SeverityHigh})
		}
	}
	if enabled["high_entropy"] {
		findings = append(findings, highEntropyFindings(text)...)
	}
	return findings, nil
}

var entropyTokenRe = regexp.MustCompile(`[A-Za-z0-9+/=_\-]{20,}`)

func highEntropyFindings(text string) []Finding {
	var findings []Finding
	for _, loc := range entropyTokenRe.FindAllStringIndex(text, -1) {
		if shannonEntropy(text[loc[0]:loc[1]]) >= 4.0 {
			findings = append(findings, Finding{Detector: "secrets", Type: "high_entropy", Start: loc[0], End: loc[1], Severity: SeverityMedium})
		}
	}
	return findings
}

func shannonEntropy(s string) float64 {
	if s == "" {
		return 0
	}
	counts := make(map[rune]int)
	for _, r := range s {
		counts[r]++
	}
	n := float64(len([]rune(s)))
	e := 0.0
	for _, c := range counts {
		p := float64(c) / n
		e -= p * math.Log2(p)
	}
	return e
}
