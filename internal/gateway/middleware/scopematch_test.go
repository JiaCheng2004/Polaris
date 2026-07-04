package middleware

import (
	"regexp"
	"strings"
	"testing"
)

// referenceRegexMatch is the previous ModelAllowed matching implementation,
// kept here only to prove wildcardMatch is byte-for-byte equivalent (B3).
func referenceRegexMatch(pattern, candidate string) bool {
	regex := "^" + regexp.QuoteMeta(pattern) + "$"
	regex = strings.ReplaceAll(regex, `\*`, ".*")
	matched, _ := regexp.MatchString(regex, candidate)
	return matched
}

func TestWildcardMatchMatchesLegacyRegex(t *testing.T) {
	patterns := []string{
		"openai/gpt-4o", "openai/*", "*", "*-mini", "*gpt*", "anthropic/claude-*-6",
		"", "a", "a*b*c", "**", "x/y/z", "prefix*", "*suffix",
	}
	candidates := []string{
		"openai/gpt-4o", "openai/gpt-4o-mini", "anthropic/claude-sonnet-4-6",
		"", "a", "abc", "aXbYc", "x/y/z", "prefix-thing", "thing-suffix", "no", "gpt",
	}
	for _, p := range patterns {
		for _, c := range candidates {
			if got, want := wildcardMatch(p, c), referenceRegexMatch(p, c); got != want {
				t.Errorf("wildcardMatch(%q,%q)=%v, legacy regex=%v", p, c, got, want)
			}
		}
	}
}

func TestModelAllowed(t *testing.T) {
	cases := []struct {
		patterns  []string
		candidate string
		want      bool
	}{
		{nil, "openai/gpt-4o", false},
		{[]string{"*"}, "anything/at-all", true},
		{[]string{"openai/*"}, "openai/gpt-4o", true},
		{[]string{"openai/*"}, "anthropic/claude", false},
		{[]string{"anthropic/claude", "openai/*"}, "openai/gpt-4o", true},
		{[]string{"gpt-4"}, "gpt-4o", false},
		{[]string{"*-mini"}, "openai/gpt-4o-mini", true},
	}
	for _, tc := range cases {
		if got := ModelAllowed(tc.patterns, tc.candidate); got != tc.want {
			t.Errorf("ModelAllowed(%v,%q)=%v, want %v", tc.patterns, tc.candidate, got, tc.want)
		}
	}
}
