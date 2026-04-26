package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type gosecReport struct {
	Issues []gosecIssue `json:"Issues"`
}

type gosecIssue struct {
	RuleID     string `json:"rule_id"`
	Severity   string `json:"severity"`
	Confidence string `json:"confidence"`
	Details    string `json:"details"`
	File       string `json:"file"`
	Line       string `json:"line"`
	Code       string `json:"code"`
}

type allowlist struct {
	Entries []allowlistEntry `json:"entries"`
}

type allowlistEntry struct {
	RuleID    string `json:"rule_id"`
	File      string `json:"file"`
	Symbol    string `json:"symbol"`
	Snippet   string `json:"snippet"`
	Rationale string `json:"rationale"`
}

type enrichedIssue struct {
	gosecIssue
	RelFile string
	LineNo  int
	Symbol  string
	Snippet string
}

func main() {
	reportPath := flag.String("report", "", "gosec JSON report path")
	allowlistPath := flag.String("allowlist", "config/security/gosec_allowlist.json", "security allowlist path")
	flag.Parse()

	if strings.TrimSpace(*reportPath) == "" {
		exitf("missing -report")
	}

	report, err := readReport(*reportPath)
	if err != nil {
		exitf("read gosec report: %v", err)
	}
	allowed, err := readAllowlist(*allowlistPath)
	if err != nil {
		exitf("read allowlist: %v", err)
	}

	entries := make([]allowlistEntry, 0, len(allowed.Entries))
	for _, entry := range allowed.Entries {
		entry.RuleID = strings.TrimSpace(entry.RuleID)
		entry.File = normalizePath(entry.File)
		entry.Symbol = strings.TrimSpace(entry.Symbol)
		entry.Snippet = normalizeSnippet(entry.Snippet)
		entry.Rationale = strings.TrimSpace(entry.Rationale)
		if entry.RuleID == "" || entry.File == "" || entry.Symbol == "" || entry.Snippet == "" || entry.Rationale == "" {
			exitf("allowlist entries must include rule_id, file, symbol, snippet, and rationale")
		}
		entries = append(entries, entry)
	}

	matched := make([]bool, len(entries))
	var unsuppressed []enrichedIssue
	for _, rawIssue := range report.Issues {
		issue := enrichIssue(rawIssue)
		index := matchAllowlist(issue, entries)
		if index >= 0 {
			matched[index] = true
			continue
		}
		unsuppressed = append(unsuppressed, issue)
	}

	var stale []allowlistEntry
	for index, entry := range entries {
		if !matched[index] {
			stale = append(stale, entry)
		}
	}

	sortIssues(unsuppressed)
	if len(unsuppressed) > 0 {
		fmt.Fprintf(os.Stderr, "security-check failed: %d unsuppressed gosec finding(s)\n", len(unsuppressed))
		for _, issue := range unsuppressed {
			fmt.Fprintf(os.Stderr, "- %s %s/%s %s:%d %s\n", issue.RuleID, issue.Severity, issue.Confidence, issue.RelFile, issue.LineNo, issue.Details)
			if issue.Symbol != "" {
				fmt.Fprintf(os.Stderr, "  symbol: %s\n", issue.Symbol)
			}
			if issue.Snippet != "" {
				fmt.Fprintf(os.Stderr, "  code: %s\n", issue.Snippet)
			}
			fmt.Fprintln(os.Stderr, "  allowlist: missing")
		}
		os.Exit(1)
	}
	if len(stale) > 0 {
		fmt.Fprintf(os.Stderr, "security-check failed: %d stale allowlist entry/entries\n", len(stale))
		for _, entry := range stale {
			fmt.Fprintf(os.Stderr, "- %s %s %s\n", entry.RuleID, entry.File, entry.Symbol)
			fmt.Fprintf(os.Stderr, "  snippet: %s\n", entry.Snippet)
			fmt.Fprintf(os.Stderr, "  rationale: %s\n", entry.Rationale)
		}
		os.Exit(1)
	}

	fmt.Printf("security-check passed: %d gosec finding(s), %d exact allowlist match(es), 0 unsuppressed\n", len(report.Issues), len(entries))
}

func readReport(path string) (gosecReport, error) {
	data, err := readRepoFile(path)
	if err != nil {
		return gosecReport{}, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return gosecReport{}, nil
	}
	var report gosecReport
	if err := json.Unmarshal(data, &report); err != nil {
		return gosecReport{}, err
	}
	return report, nil
}

func readAllowlist(path string) (allowlist, error) {
	data, err := readRepoFile(path)
	if err != nil {
		return allowlist{}, err
	}
	var allowed allowlist
	if err := json.Unmarshal(data, &allowed); err != nil {
		return allowlist{}, err
	}
	return allowed, nil
}

func enrichIssue(issue gosecIssue) enrichedIssue {
	lineNo, _ := strconv.Atoi(strings.TrimSpace(issue.Line))
	relFile := normalizePath(issue.File)
	snippet := snippetAt(relFile, lineNo)
	return enrichedIssue{
		gosecIssue: issue,
		RelFile:    relFile,
		LineNo:     lineNo,
		Symbol:     symbolAt(relFile, lineNo),
		Snippet:    snippet,
	}
}

func matchAllowlist(issue enrichedIssue, entries []allowlistEntry) int {
	for index, entry := range entries {
		if issue.RuleID != entry.RuleID {
			continue
		}
		if issue.RelFile != entry.File {
			continue
		}
		if issue.Symbol != entry.Symbol {
			continue
		}
		if !strings.Contains(issue.Snippet, entry.Snippet) {
			continue
		}
		return index
	}
	return -1
}

func symbolAt(path string, lineNo int) string {
	lines, err := readLines(path)
	if err != nil || lineNo <= 0 || lineNo > len(lines) {
		return ""
	}
	funcPattern := regexp.MustCompile(`^\s*func\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	for i := lineNo - 1; i >= 0; i-- {
		matches := funcPattern.FindStringSubmatch(lines[i])
		if len(matches) == 2 {
			return matches[1]
		}
	}
	return ""
}

func snippetAt(path string, lineNo int) string {
	lines, err := readLines(path)
	if err != nil || lineNo <= 0 || lineNo > len(lines) {
		return ""
	}
	return normalizeSnippet(lines[lineNo-1])
}

func readLines(path string) ([]string, error) {
	data, err := readRepoFile(path)
	if err != nil {
		return nil, err
	}
	return strings.Split(string(data), "\n"), nil
}

func readRepoFile(path string) ([]byte, error) {
	cleanPath, err := cleanRepoPath(path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(cleanPath)
}

func cleanRepoPath(path string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	cleanPath := filepath.Clean(path)
	rel, err := filepath.Rel(cwd, cleanPath)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path %s is outside repository root", path)
	}
	return cleanPath, nil
}

func normalizePath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	cwd, err := os.Getwd()
	if err == nil {
		cwd = filepath.ToSlash(cwd)
		path = strings.TrimPrefix(path, cwd+"/")
	}
	return strings.TrimPrefix(path, "./")
}

func normalizeSnippet(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func sortIssues(issues []enrichedIssue) {
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].RuleID != issues[j].RuleID {
			return issues[i].RuleID < issues[j].RuleID
		}
		if issues[i].RelFile != issues[j].RelFile {
			return issues[i].RelFile < issues[j].RelFile
		}
		return issues[i].LineNo < issues[j].LineNo
	})
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
