package goalie_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// onlyLongFunction turns off the stage 1 rules, because these repos have no module for crap to test,
// and the bodies of two long functions are duplicates.
const onlyLongFunction = `{"rules": {"dup": {"mode": "off"}, "crap": {"mode": "off"}}}`

const longWarn = "a.go:3: warn long-function: Parse has 81 lines (limit 80)\n"

func withLongFunction(settings string) string {
	return `{"rules": {"dup": {"mode": "off"}, "crap": {"mode": "off"}, "many-arguments": {"mode": "off"}, "many-dependencies": {"mode": "off"}, "long-function": ` + settings + `}}`
}

// longFunc has exactly lines lines, from the func keyword to the closing brace.
func longFunc(name string, lines int) string {
	return fmt.Sprintf("func %s() {\n%s}\n", name, strings.Repeat("\t_ = 0\n", lines-2))
}

// repoWithNewLongFunction commits an empty package and then adds fn as a change, at line 3 of a.go.
func repoWithNewLongFunction(t *testing.T, settings, fn string) string {
	t.Helper()
	dir := gitRepo(t, map[string]string{"a.go": "package a\n", ".goalierc": settings})
	write(t, dir, "a.go", "package a\n\n"+fn)
	return dir
}

func TestLongFunctionReportsAFunctionOverEightyLines(t *testing.T) {
	cases := []struct {
		lines      int
		wantStdout string
	}{
		{80, ""},
		{81, longWarn},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.lines), func(t *testing.T) {
			dir := repoWithNewLongFunction(t, onlyLongFunction, longFunc("Parse", tc.lines))
			code, stdout, stderr := runGoalie(t, dir, "check", "--base", "HEAD")
			if code != 0 {
				t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
			}
			if stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

func TestEachModeDecidesWhatALongFunctionDoes(t *testing.T) {
	cases := []struct {
		name       string
		settings   string
		wantCode   int
		wantStdout string
	}{
		{"off says nothing", `{"mode": "off"}`, 0, ""},
		{"warn reports and passes", `{"mode": "warn"}`, 0, longWarn},
		{"fail fails", `{"mode": "fail"}`, 1, "a.go:3: fail long-function: Parse has 81 lines (limit 80)\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := repoWithNewLongFunction(t, withLongFunction(tc.settings), longFunc("Parse", 81))
			code, stdout, stderr := runGoalie(t, dir, "check", "--base", "HEAD")
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d; stderr: %s", code, tc.wantCode, stderr)
			}
			if stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

func TestEachScopeDecidesWhichLongFunctionsAreChecked(t *testing.T) {
	cases := []struct {
		name       string
		settings   string
		args       []string
		wantStdout string
	}{
		{"changes skips old code", `{"scope": "changes"}`, []string{"check", "--base", "HEAD"}, ""},
		{"all sees old code with no revision", `{"scope": "all"}`, []string{"check"}, longWarn},
		{"--all sees old code in a rule set to changes", `{"scope": "changes"}`, []string{"check", "--all"}, longWarn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := gitRepo(t, map[string]string{
				"a.go":      "package a\n\n" + longFunc("Parse", 81),
				".goalierc": withLongFunction(tc.settings),
			})
			code, stdout, stderr := runGoalie(t, dir, tc.args...)
			if code != 0 {
				t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
			}
			if stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

func TestTheLongFunctionLimitComesFromTheFlagThenTheFile(t *testing.T) {
	cases := []struct {
		name       string
		settings   string
		args       []string
		wantStdout string
	}{
		{"0 is a normal limit", `{"limit": 0}`, []string{"check", "--base", "HEAD"},
			"a.go:3: warn long-function: Parse has 81 lines (limit 0)\n"},
		{"the file raises the limit", `{"limit": 81}`, []string{"check", "--base", "HEAD"}, ""},
		{"the flag raises the limit", `{}`, []string{"check", "long-function", "--limit", "81", "--base", "HEAD"}, ""},
		{"the flag replaces the file", `{"limit": 100}`, []string{"check", "long-function", "--limit", "50", "--base", "HEAD"},
			"a.go:3: warn long-function: Parse has 81 lines (limit 50)\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := repoWithNewLongFunction(t, withLongFunction(tc.settings), longFunc("Parse", 81))
			code, stdout, stderr := runGoalie(t, dir, tc.args...)
			if code != 0 {
				t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
			}
			if stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

func TestALimitMistakeForLongFunctionExitsWithTwo(t *testing.T) {
	cases := []struct {
		name     string
		settings string
		args     []string
		want     string
	}{
		{"a negative limit", `{"limit": -1}`, []string{"check", "--base", "HEAD"}, "must not be negative"},
		{"an unknown setting", `{"threshold": 0.5}`, []string{"check", "--base", "HEAD"}, "threshold"},
		{"a shared limit", `{}`, []string{"check", "long-function", "--shared-limit", "5", "--base", "HEAD"},
			"--shared-limit works only with crap"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := repoWithNewLongFunction(t, withLongFunction(tc.settings), longFunc("Parse", 81))
			code, stdout, stderr := runGoalie(t, dir, tc.args...)
			if code != 2 || stdout != "" || !strings.Contains(stderr, tc.want) {
				t.Errorf("got exit code %d, stdout %q, stderr %q; want 2, nothing, and %q", code, stdout, stderr, tc.want)
			}
		})
	}
}

func TestGoalieIgnoreLongFunctionHidesThatRuleOnly(t *testing.T) {
	cases := []struct {
		name       string
		directive  string
		wantStdout string
	}{
		{"its own name hides it", "//goalie:ignore long-function The table reads better as one.\n", ""},
		{"another name doesn't", "//goalie:ignore dup Wiring repeats on purpose.\n",
			"a.go:4: warn long-function: Parse has 81 lines (limit 80)\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := repoWithNewLongFunction(t, onlyLongFunction, tc.directive+longFunc("Parse", 81))
			code, stdout, stderr := runGoalie(t, dir, "check", "--base", "HEAD")
			if code != 0 {
				t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
			}
			if stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

// A limit of 0 would report the 1-line stub, if the rule counted it.
func TestLongFunctionSkipsTestFilesAndFunctionsWithNoBody(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n", ".goalierc": withLongFunction(`{"limit": 0}`)})
	write(t, dir, "a_test.go", "package a\n\n"+longFunc("TestParse", 81))
	write(t, dir, "asm.go", "package a\n\n// Parse is written in assembly.\nfunc Parse()\n")
	code, stdout, stderr := runGoalie(t, dir, "check", "--base", "HEAD")
	if code != 0 || stdout != "" {
		t.Errorf("got exit code %d and stdout %q, want 0 and nothing; stderr: %s", code, stdout, stderr)
	}
}

func TestAnUnmatchedSharedPatternWarnsOnceForEveryRule(t *testing.T) {
	dir := repoWithNewLongFunction(t, `{"shared": ["pkgs/**"], "rules": {"crap": {"coverProfiles": ["p.out"]}}}`, longFunc("Parse", 81))
	write(t, dir, "p.out", "mode: set\n")
	code, _, stderr := runGoalie(t, dir, "check", "--partial", "--base", "HEAD")
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	if warnings := strings.Count(stderr, `shared pattern "pkgs/**" matches no file`); warnings != 1 {
		t.Errorf("got the warning %d times, want 1; stderr: %s", warnings, stderr)
	}
}

func TestALongFunctionResultHasAJSONForm(t *testing.T) {
	dir := repoWithNewLongFunction(t, onlyLongFunction, longFunc("Parse", 81))
	code, stdout, stderr := runGoalie(t, dir, "check", "--format", "json", "--base", "HEAD")
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	results := parseResults(t, stdout)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1: %s", len(results), stdout)
	}
	wantFields(t, results[0], map[string]any{
		"rule": "long-function", "mode": "warn", "value": 81.0, "limit": 80.0,
		"file": "a.go", "line": 3.0, "function": "Parse",
	})
}

// A wrapper on PATH logs each git command, so the test sees how often a run asks for the changes.
// Each ask runs merge-base once.
func TestAPartialCheckRunsGitOnceForEveryRule(t *testing.T) {
	dir := repoWithNewLongFunction(t, `{}`, longFunc("Parse", 81))
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin, log := t.TempDir(), filepath.Join(t.TempDir(), "git.log")
	write(t, bin, "git", fmt.Sprintf("#!/bin/sh\necho \"$*\" >> %q\nexec %q \"$@\"\n", log, realGit))
	if err := os.Chmod(filepath.Join(bin, "git"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	code, stdout, stderr := runGoalie(t, dir, "check", "--partial", "--base", "HEAD")
	if code != 0 || stdout != longWarn {
		t.Errorf("got exit code %d and stdout %q, want 0 and %q; stderr: %s", code, stdout, longWarn, stderr)
	}
	logged, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if asks := strings.Count(string(logged), " merge-base "); asks != 1 {
		t.Errorf("git merge-base ran %d times, want 1:\n%s", asks, logged)
	}
}
