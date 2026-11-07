package goalie_test

import (
	"fmt"
	"strings"
	"testing"
)

func TestEachDupSettingApplies(t *testing.T) {
	copied := "package a\n\n" + sumFunc("Copied")
	similar := strings.Replace(copied, "value > 0", "value > 0 && value < 9", 1)
	otherLiterals := strings.NewReplacer("100", "200", "> 0", "> 7").Replace(copied)
	cases := []struct {
		name     string
		settings string
		file     string
		content  string
		wantCode int
	}{
		{"minLines", `{"rules": {"dup": {"minLines": 13}}}`, "b.go", copied, 0},
		{"minNodes", `{"rules": {"dup": {"minNodes": 1000}}}`, "b.go", copied, 0},
		{"default threshold", `{}`, "b.go", similar, 0},
		{"threshold", `{"rules": {"dup": {"threshold": 0.5}}}`, "b.go", similar, 1},
		{"default weak limits", `{}`, "b.go", otherLiterals, 1},
		{"weakAnchors and weakLiterals", `{"rules": {"dup": {"weakAnchors": 1, "weakLiterals": 1}}}`, "b.go", otherLiterals, 0},
		{"default includeTests", `{}`, "b_test.go", copied, 0},
		{"includeTests", `{"rules": {"dup": {"includeTests": true}}}`, "b_test.go", copied, 1},
		{"null means not set", `{"rules": {"dup": {"mode": null, "scope": null, "minLines": null}}}`, "b.go", copied, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := gitRepo(t, map[string]string{"a.go": "package a\n\n" + sumFunc("Total")})
			write(t, dir, tc.file, tc.content)
			write(t, dir, ".goalierc", tc.settings)

			code, stdout, stderr := runGoalie(t, dir, "check", "dup", "--base", "HEAD")

			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d; stdout: %s; stderr: %s", code, tc.wantCode, stdout, stderr)
			}
		})
	}
}

func TestALimitOfZeroKeepsTheRuleOn(t *testing.T) {
	small := "func %s() int {\n\treturn 1\n}\n"
	cases := []struct {
		name     string
		settings string
		old, new string
	}{
		{"threshold", `{"rules": {"dup": {"threshold": 0}}}`, sumFunc("Total"), strings.Replace(sumFunc("Other"), "value > 0", "value > 0 && value < 9", 1)},
		{"minLines and minNodes", `{"rules": {"dup": {"minLines": 0, "minNodes": 0}}}`, fmt.Sprintf(small, "One"), fmt.Sprintf(small, "Two")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := gitRepo(t, map[string]string{"a.go": "package a\n\n" + tc.old})
			write(t, dir, "b.go", "package a\n\n"+tc.new)
			write(t, dir, ".goalierc", tc.settings)

			code, stdout, stderr := runGoalie(t, dir, "check", "dup", "--base", "HEAD")

			if code != 1 {
				t.Errorf("exit code = %d, want 1; stdout: %s; stderr: %s", code, stdout, stderr)
			}
		})
	}
}

func TestABadRuleSettingExitsWithTwo(t *testing.T) {
	cases := []struct{ name, settings, want string }{
		{"unknown rule", `{"rules": {"dupe": {}}}`, `"dupe"`},
		{"unknown mode", `{"rules": {"dup": {"mode": "warning"}}}`, `"warning"`},
		{"unknown scope", `{"rules": {"dup": {"scope": "new"}}}`, `"new"`},
		{"empty mode", `{"rules": {"dup": {"mode": ""}}}`, "mode"},
		{"mode of the wrong type", `{"rules": {"dup": {"mode": 1}}}`, "mode"},
		{"unknown dup setting", `{"rules": {"dup": {"limit": 1}}}`, `"limit"`},
		{"warnOnly, which mode replaces", `{"rules": {"dup": {"warnOnly": true}}}`, `"warnOnly"`},
		{"threshold too high", `{"rules": {"dup": {"threshold": 2}}}`, ".goalierc"},
		{"negative lines", `{"rules": {"dup": {"minLines": -1}}}`, ".goalierc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runGoalie(t, repoWithNewCopy(t, tc.settings), "check", "--base", "HEAD")
			if code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want nothing", stdout)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr = %q, want it to hold %s", stderr, tc.want)
			}
		})
	}
}

// The error names the new place before the strict decoder could name another unknown field, so
// that a section of a removed command can't vanish without a word.
func TestASectionOfARemovedCommandNamesTheRuleThatTookIt(t *testing.T) {
	cases := []struct{ name, settings, want string }{
		{"the dup section", `{"godry": {"minLines": 13}}`, `goalie: .goalierc: move "godry" settings to "rules.dup"`},
		{"the crap section", `{"gocrap": {"limit": 8}}`, `goalie: .goalierc: move "gocrap" settings to "rules.crap"`},
		{"an empty section", `{"gocrap": {}}`, `"rules.crap"`},
		{"beside an unknown setting", `{"weakLabels": 1, "godry": {}}`, `"rules.dup"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runGoalie(t, repoWithNewCopy(t, tc.settings), "check", "--base", "HEAD")
			if code != 2 || stdout != "" || !strings.Contains(stderr, tc.want) {
				t.Errorf("exit code = %d, stdout = %q, stderr = %q; want exit 2 with %s", code, stdout, stderr, tc.want)
			}
		})
	}
}
