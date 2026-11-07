package goalie_test

import (
	"strings"
	"testing"
)

func TestTheLimitFlagsReplaceTheSettingsForOneRun(t *testing.T) {
	cases := []struct {
		name       string
		settings   string
		args       []string
		wantCode   int
		wantStdout string
	}{
		{"crap limit", `{"rules": {"crap": {"limit": 50, "coverProfiles": ["p.out"]}}}`, []string{"--limit", "0"}, 1,
			"a.go:3: fail crap: Decide scores 42.0 (complexity 6, coverage 0.0%, limit 0)\n"},
		{"crap limit above the score", `{"rules": {"crap": {"coverProfiles": ["p.out"]}}}`, []string{"--limit", "42"}, 0, ""},
		{"crap shared limit", `{"shared": ["a.go"], "rules": {"crap": {"sharedLimit": 50, "coverProfiles": ["p.out"]}}}`,
			[]string{"--shared-limit", "41"}, 1, "a.go:3: fail crap: Decide scores 42.0 (complexity 6, coverage 0.0%, limit 41)\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := repoWithNewFunction(t, tc.settings, emptyProfile)
			code, stdout, stderr := runGoalie(t, dir, append([]string{"check", "crap", "--base", "HEAD"}, tc.args...)...)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d; stderr: %s", code, tc.wantCode, stderr)
			}
			if stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

func TestTheLimitFlagSetsTheDupThreshold(t *testing.T) {
	dir := repoWithNewCopy(t, `{"rules": {"dup": {"threshold": 0.9}, `+crapOff+`}}`)
	code, stdout, stderr := runGoalie(t, dir, "check", "dup", "--base", "HEAD", "--limit", "0.5")
	want := "b.go:3: fail dup: duplicates a.go:3 (score 1.00, limit 0.5, literals 1.00, anchors 0)\n"
	if code != 1 || stdout != want {
		t.Errorf("exit code = %d, stdout = %q, want 1 and %q; stderr: %s", code, stdout, want, stderr)
	}
}

func TestWarnOnlyReportsAFailRuleAsWarnAndExitsWithZero(t *testing.T) {
	cases := []struct {
		name string
		dir  func(t *testing.T) string
		rule string
		want string
	}{
		{"crap", func(t *testing.T) string { return repoWithNewFunction(t, coverSettings, emptyProfile) }, "crap", decideWarn},
		{"dup", func(t *testing.T) string { return repoWithNewCopy(t, `{"rules": {`+crapOff+`}}`) }, "dup", copiedWarn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runGoalie(t, tc.dir(t), "check", tc.rule, "--base", "HEAD", "--warn-only")
			if code != 0 || stdout != tc.want {
				t.Errorf("exit code = %d, stdout = %q, want 0 and %q; stderr: %s", code, stdout, tc.want, stderr)
			}
		})
	}
}

func TestWarnOnlyStillExitsWithTwoWhenTheTestsFail(t *testing.T) {
	dir := repoWithNewFunction(t, `{"rules": {"crap": {"coverCommand": "exit 1"}}}`, emptyProfile)
	if code, _, stderr := runGoalie(t, dir, "check", "crap", "--base", "HEAD", "--warn-only"); code != 2 {
		t.Errorf("exit code = %d, want 2; stderr: %s", code, stderr)
	}
}

func TestTheOneRuleFlagsNeedExactlyOneRule(t *testing.T) {
	dir := repoWithNewFunction(t, coverSettings, emptyProfile)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"limit with no rule", []string{"--limit", "0"}, "--limit needs exactly one rule name"},
		{"limit with two rules", []string{"dup", "crap", "--limit", "0"}, "--limit needs exactly one rule name"},
		{"shared limit with no rule", []string{"--shared-limit", "0"}, "--shared-limit needs exactly one rule name"},
		{"warn-only with two rules", []string{"dup", "crap", "--warn-only"}, "--warn-only needs exactly one rule name"},
		{"shared limit with dup", []string{"dup", "--shared-limit", "0"}, "--shared-limit works only with crap"},
		{"a limit that isn't a number", []string{"crap", "--limit", "high"}, "limit"},
		{"a negative limit", []string{"crap", "--limit", "-1"}, "must not be negative"},
		{"a negative shared limit", []string{"crap", "--shared-limit", "-1"}, "must not be negative"},
		{"a dup limit above 1", []string{"dup", "--limit", "1.5"}, "between 0 and 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runGoalie(t, dir, append([]string{"check", "--base", "HEAD"}, tc.args...)...)
			if code != 2 || stdout != "" {
				t.Errorf("exit code = %d, stdout = %q, want 2 and nothing", code, stdout)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr = %q, want it to hold %q", stderr, tc.want)
			}
		})
	}
}
