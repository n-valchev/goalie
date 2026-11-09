package goalie_test

import (
	"fmt"
	"strings"
	"testing"
)

var onlyMeanComplexity = `{"rules": {` + rulesOff("high-mean-complexity") + `}}`

func complexFunc(name string, complexity int) string {
	return fmt.Sprintf("func %s() {\n%s}\n\n", name, strings.Repeat("\tif true {\n\t}\n", complexity-1))
}

func goFile(funcs ...string) string {
	return "package a\n\n" + strings.Join(funcs, "")
}

func TestHighMeanComplexityReportsAMeanOverSeven(t *testing.T) {
	cases := []struct {
		name       string
		src        string
		wantStdout string
	}{
		{"a mean of 7", goFile(complexFunc("A", 7), complexFunc("B", 7)), ""},
		{"a mean of 7.5", goFile(complexFunc("A", 7), complexFunc("B", 8)), meanWarn("7.5", "2 functions")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkMeanComplexity(t, "package a\n", tc.src, onlyMeanComplexity, tc.wantStdout)
		})
	}
}

func checkMeanComplexity(t *testing.T, base, now, settings, wantStdout string) {
	t.Helper()
	dir := gitRepo(t, map[string]string{"a.go": base, ".goalierc": settings})
	write(t, dir, "a.go", now)
	code, stdout, stderr := runGoalie(t, dir, "check", "--base", "HEAD")
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	if stdout != wantStdout {
		t.Errorf("stdout = %q, want %q", stdout, wantStdout)
	}
}

func meanWarn(mean string, functions string) string {
	return "a.go:1: warn high-mean-complexity: a.go has mean complexity " + mean + " over " + functions + " (limit 7)\n"
}

// A limit of 0 would report the file, if it had a mean above 0.
func TestAFileWithNoFunctionsHasNoMeanComplexity(t *testing.T) {
	settings := `{"rules": {` + rulesOff("high-mean-complexity") + `, "high-mean-complexity": {"limit": 0}}}`
	checkMeanComplexity(t, "package a\n", "package a\n\nvar x = 1\n", settings, "")
}

func TestWhatCountsTowardsTheMeanComplexity(t *testing.T) {
	closure := "func A() {\n\tf := func() {\n" + strings.Repeat("\t\tif true {\n\t\t}\n", 7) + "\t}\n\tf()\n}\n"
	cases := []struct {
		name       string
		src        string
		wantStdout string
	}{
		{"a function with no body doesn't count", goFile(complexFunc("A", 8), "func B()\n"), meanWarn("8.0", "1 function")},
		{"a closure adds to its function", goFile(closure), meanWarn("8.0", "1 function")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkMeanComplexity(t, "package a\n", tc.src, onlyMeanComplexity, tc.wantStdout)
		})
	}
}

func TestHighMeanComplexityReportsOnlyAChangeThatCrossesTheLimit(t *testing.T) {
	cases := []struct {
		name       string
		base, now  string
		wantStdout string
	}{
		{"an old file over the limit gives no result",
			goFile(complexFunc("A", 8)), goFile(complexFunc("A", 8), complexFunc("B", 9)), ""},
		{"a change from 7 to 7.5 reports",
			goFile(complexFunc("A", 7)), goFile(complexFunc("A", 7), complexFunc("B", 8)), meanWarn("7.5", "2 functions")},
		{"a base file that doesn't parse gives no result", "package a\n\nfunc {\n", goFile(complexFunc("A", 8)), ""},
		{"a base file with no functions reports as a new file",
			"package a\n\nvar x = 1\n", goFile(complexFunc("A", 8)), meanWarn("8.0", "1 function")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkMeanComplexity(t, tc.base, tc.now, onlyMeanComplexity, tc.wantStdout)
		})
	}
}

func TestAHighMeanComplexityResultHasAJSONForm(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n", ".goalierc": onlyMeanComplexity})
	write(t, dir, "a.go", goFile(complexFunc("A", 7), complexFunc("B", 8)))
	code, stdout, stderr := runGoalie(t, dir, "check", "--format", "json", "--base", "HEAD")
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	results := parseResults(t, stdout)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1: %s", len(results), stdout)
	}
	wantFields(t, results[0], map[string]any{
		"rule": "high-mean-complexity", "mode": "warn", "value": 7.5, "limit": 7.0,
		"file": "a.go", "line": 1.0, "functions": 2.0,
	})
}
