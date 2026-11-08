package goalie_test

import (
	"strings"
	"testing"
)

const (
	copiedPair = "b.go:3: fail dup: duplicates a.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"
	copiedWarn = "b.go:3: warn dup: duplicates a.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"
	oldPair    = "a.go:3: fail dup: duplicates b.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"
	oldWarn    = "a.go:3: warn dup: duplicates b.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"

	// otherRulesOff keeps these tests on dup, because these repos have no module for crap to test,
	// and some of them run with no revision.
	otherRulesOff = `"crap": {"mode": "off"}, "long-function": {"mode": "off"}, "many-arguments": {"mode": "off"}, "many-dependencies": {"mode": "off"}`
)

func repoWithNewCopy(t *testing.T, settings string) string {
	t.Helper()
	dir := gitRepo(t, map[string]string{"a.go": "package a\n\n" + sumFunc("Total")})
	write(t, dir, "b.go", "package a\n\n"+sumFunc("Copied"))
	write(t, dir, ".goalierc", settings)
	return dir
}

func TestEachModeDecidesWhatADuplicateDoes(t *testing.T) {
	cases := []struct {
		name       string
		settings   string
		args       []string
		wantCode   int
		wantStdout string
	}{
		{"no dup settings fails", `{"rules": {` + otherRulesOff + `}}`, []string{"check", "--base", "HEAD"}, 1, copiedPair},
		{"a rule with no mode fails", `{"rules": {` + otherRulesOff + `, "dup": {}}}`, []string{"check", "--base", "HEAD"}, 1, copiedPair},
		{"fail fails", `{"rules": {` + otherRulesOff + `, "dup": {"mode": "fail"}}}`, []string{"check", "--base", "HEAD"}, 1, copiedPair},
		{"warn reports and passes", `{"rules": {` + otherRulesOff + `, "dup": {"mode": "warn"}}}`, []string{"check", "--base", "HEAD"}, 0, copiedWarn},
		{"off says nothing", `{"rules": {` + otherRulesOff + `, "dup": {"mode": "off"}}}`, []string{"check", "--base", "HEAD"}, 0, ""},
		{"off needs no revision", `{"rules": {` + otherRulesOff + `, "dup": {"mode": "off"}}}`, []string{"check"}, 0, ""},
		{"off runs as warn when named", `{"rules": {` + otherRulesOff + `, "dup": {"mode": "off"}}}`, []string{"check", "dup", "--base", "HEAD"}, 0, copiedWarn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runGoalie(t, repoWithNewCopy(t, tc.settings), tc.args...)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d; stderr: %s", code, tc.wantCode, stderr)
			}
			if stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

// repoWithOldCopy has two committed copies, so only a check of the whole repo can see them.
func repoWithOldCopy(t *testing.T, settings string) string {
	t.Helper()
	return gitRepo(t, map[string]string{
		"a.go":      "package a\n\n" + sumFunc("Total"),
		"b.go":      "package a\n\n" + sumFunc("Copied"),
		".goalierc": settings,
	})
}

func TestEachScopeDecidesWhichCodeIsChecked(t *testing.T) {
	cases := []struct {
		name       string
		settings   string
		args       []string
		wantCode   int
		wantStdout string
	}{
		{"changes skips old code", `{"rules": {` + otherRulesOff + `, "dup": {"scope": "changes"}}}`, []string{"check", "--base", "HEAD"}, 0, ""},
		{"all sees old code with no revision", `{"rules": {` + otherRulesOff + `, "dup": {"scope": "all"}}}`, []string{"check"}, 1, oldPair},
		{"all sees old code despite a revision", `{"rules": {` + otherRulesOff + `, "dup": {"scope": "all"}}}`, []string{"check", "--base", "HEAD"}, 1, oldPair},
		{"all never reads the revision", `{"rules": {` + otherRulesOff + `, "dup": {"scope": "all"}}}`, []string{"check", "--base", "no-such-rev"}, 1, oldPair},
		{"all follows the mode", `{"rules": {` + otherRulesOff + `, "dup": {"scope": "all", "mode": "warn"}}}`, []string{"check"}, 0, oldWarn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runGoalie(t, repoWithOldCopy(t, tc.settings), tc.args...)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d; stderr: %s", code, tc.wantCode, stderr)
			}
			if stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

func TestTheRevisionComesFromTheFlagThenTheFile(t *testing.T) {
	cases := []struct {
		name       string
		settings   string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{"the file alone", `{"base": "HEAD", "rules": {` + otherRulesOff + `}}`, []string{"check"}, 1, ""},
		{"the flag replaces the file", `{"base": "no-such-rev", "rules": {` + otherRulesOff + `}}`, []string{"check", "--base", "HEAD"}, 1, ""},
		{"a bad revision in the file", `{"base": "no-such-rev", "rules": {` + otherRulesOff + `}}`, []string{"check"}, 2, "no-such-rev"},
		{"an empty revision in the file", `{"base": "", "rules": {` + otherRulesOff + `}}`, []string{"check"}, 2, "revision"},
		{"neither", `{"rules": {` + otherRulesOff + `}}`, []string{"check"}, 2, ".goalierc"},
		{"neither for a named rule that is off", `{"rules": {` + otherRulesOff + `, "dup": {"mode": "off"}}}`, []string{"check", "dup"}, 2, "revision"},
		{"an empty flag doesn't fall back to the file", `{"base": "HEAD", "rules": {` + otherRulesOff + `}}`, []string{"check", "--base", ""}, 2, "revision"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := runGoalie(t, repoWithNewCopy(t, tc.settings), tc.args...)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d; stderr: %s", code, tc.wantCode, stderr)
			}
			if !strings.Contains(stderr, tc.wantStderr) {
				t.Errorf("stderr = %q, want it to hold %q", stderr, tc.wantStderr)
			}
		})
	}
}
