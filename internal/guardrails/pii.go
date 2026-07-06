package guardrails

import (
	"regexp"
	"strconv"
	"strings"
)

type piiDetector struct {
	patterns map[string]*regexp.Regexp
	order    []string
}

func newPIIDetector() *piiDetector {
	pats := map[string]*regexp.Regexp{
		"email":       regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
		"phone":       regexp.MustCompile(`\+[1-9]\d{7,14}`), // E.164: leading + and 8..15 digits
		"ssn":         regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
		"credit_card": regexp.MustCompile(`\b(?:\d[ -]?){13,19}\b`),
		"ipv4":        regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`),
		"ipv6":        regexp.MustCompile(`\b(?:[0-9a-fA-F]{1,4}:){2,7}[0-9a-fA-F]{1,4}\b`),
		"iban":        regexp.MustCompile(`\b[A-Z]{2}\d{2}[A-Z0-9]{11,30}\b`),
	}
	return &piiDetector{
		patterns: pats,
		order:    []string{"email", "phone", "ssn", "credit_card", "iban", "ipv4", "ipv6"},
	}
}

func (d *piiDetector) Name() string { return "pii" }

func (d *piiDetector) Detect(text string, spec DetectorSpec) []Finding {
	enabled := typeSet(spec.Types, d.order...)
	var findings []Finding
	for _, typ := range d.order {
		if !enabled[typ] {
			continue
		}
		re := d.patterns[typ]
		for _, loc := range re.FindAllStringIndex(text, -1) {
			if !validatePII(typ, text[loc[0]:loc[1]]) {
				continue
			}
			findings = append(findings, Finding{Detector: "pii", Type: typ, Start: loc[0], End: loc[1], Severity: piiSeverity(typ)})
		}
	}
	return findings
}

func validatePII(typ, match string) bool {
	switch typ {
	case "credit_card":
		return luhnValid(match)
	case "iban":
		return ibanValid(match)
	case "ipv4":
		return ipv4Valid(match)
	default:
		return true
	}
}

func piiSeverity(typ string) Severity {
	switch typ {
	case "credit_card", "ssn", "iban":
		return SeverityHigh
	case "email", "phone":
		return SeverityMedium
	default:
		return SeverityLow
	}
}

func luhnValid(s string) bool {
	var digits []int
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits = append(digits, int(r-'0'))
		}
	}
	if len(digits) < 13 || len(digits) > 19 {
		return false
	}
	sum, alt := 0, false
	for i := len(digits) - 1; i >= 0; i-- {
		n := digits[i]
		if alt {
			if n *= 2; n > 9 {
				n -= 9
			}
		}
		sum += n
		alt = !alt
	}
	return sum%10 == 0
}

func ibanValid(s string) bool {
	s = strings.ToUpper(strings.ReplaceAll(s, " ", ""))
	if len(s) < 15 || len(s) > 34 {
		return false
	}
	rearranged := s[4:] + s[:4]
	rem := 0
	for _, r := range rearranged {
		var v int
		switch {
		case r >= 'A' && r <= 'Z':
			v = int(r - 'A' + 10)
		case r >= '0' && r <= '9':
			v = int(r - '0')
		default:
			return false
		}
		if v >= 10 {
			rem = (rem*100 + v) % 97
		} else {
			rem = (rem*10 + v) % 97
		}
	}
	return rem == 1
}

func ipv4Valid(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n > 255 {
			return false
		}
	}
	return true
}

// typeSet returns the set of enabled sub-types: the caller's list when non-empty,
// otherwise every default.
func typeSet(requested []string, defaults ...string) map[string]bool {
	set := make(map[string]bool)
	if len(requested) == 0 {
		for _, d := range defaults {
			set[d] = true
		}
		return set
	}
	for _, r := range requested {
		set[strings.ToLower(strings.TrimSpace(r))] = true
	}
	return set
}
