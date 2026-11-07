package goalie_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goMod = "module example.com/m\n\ngo 1.25\n"

// decide is on lines 3 to 19 of its file. Its complexity is 6: the function, three ifs, one &&, and
// one non-default case.
const decide = `func Decide(a, b int) int {
	if a > 0 {
		return 1
	}
	if b > 0 && a < 0 {
		return 2
	}
	if a == b {
		return 3
	}
	switch a {
	case 7:
		return 4
	default:
		return 5
	}
}
`

const (
	decideFile = "package m\n\n" + decide
	// emptyProfile gives every function 0% coverage, so Decide scores 6² + 6 = 42.
	emptyProfile = "mode: set\n"
	// halfCovered lists statements of Decide: 1 of 2 covered gives 6² × 0.5³ + 6 = 10.5.
	halfCovered = "mode: set\nexample.com/m/a.go:4.2,4.10 1 1\nexample.com/m/a.go:7.2,7.10 1 0\n"

	decideOverDefault = "a.go:3: fail crap: Decide scores 42.0 (complexity 6, coverage 0.0%, limit 30)\n"
	decideWarn        = "a.go:3: warn crap: Decide scores 42.0 (complexity 6, coverage 0.0%, limit 30)\n"
)

// repoWithNewFunction has only a go.mod in Git, so Decide counts as changed code.
func repoWithNewFunction(t *testing.T, settings, profile string) string {
	t.Helper()
	dir := gitRepo(t, map[string]string{"go.mod": goMod})
	write(t, dir, "a.go", decideFile)
	write(t, dir, "p.out", profile)
	write(t, dir, ".goalierc", settings)
	return dir
}

func TestEachModeDecidesWhatACrapResultDoes(t *testing.T) {
	cases := []struct {
		name       string
		settings   string
		wantCode   int
		wantStdout string
	}{
		{"a rule with no mode fails", `{"rules": {"crap": {"coverProfiles": ["p.out"]}}}`, 1, decideOverDefault},
		{"fail fails", `{"rules": {"crap": {"mode": "fail", "coverProfiles": ["p.out"]}}}`, 1, decideOverDefault},
		{"warn reports and passes", `{"rules": {"crap": {"mode": "warn", "coverProfiles": ["p.out"]}}}`, 0, decideWarn},
		{"off runs as warn when named", `{"rules": {"crap": {"mode": "off", "coverProfiles": ["p.out"]}}}`, 0, decideWarn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := repoWithNewFunction(t, tc.settings, emptyProfile)
			code, stdout, stderr := runGoalie(t, dir, "check", "crap", "--base", "HEAD")
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d; stderr: %s", code, tc.wantCode, stderr)
			}
			if stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

func TestCrapSaysNothingAndRunsNoTestsWhenItIsOff(t *testing.T) {
	dir := repoWithNewFunction(t, `{"rules": {"crap": {"mode": "off", "coverCommand": "touch started"}}}`, emptyProfile)
	code, stdout, stderr := runGoalie(t, dir, "check", "--base", "HEAD")
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, "started")); err == nil {
		t.Error("the cover command ran")
	}
}

func TestOnlyAScoreAboveTheLimitIsAResult(t *testing.T) {
	cases := []struct {
		name       string
		limit      string
		wantCode   int
		wantStdout string
	}{
		{"one step below the score", "10", 1, "a.go:3: fail crap: Decide scores 10.5 (complexity 6, coverage 50.0%, limit 10)\n"},
		{"equal to the score", "10.5", 0, ""},
		{"a limit of 0 is a normal limit", "0", 1, "a.go:3: fail crap: Decide scores 10.5 (complexity 6, coverage 50.0%, limit 0)\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := `{"rules": {"crap": {"limit": ` + tc.limit + `, "coverProfiles": ["p.out"]}}}`
			code, stdout, stderr := runGoalie(t, repoWithNewFunction(t, settings, halfCovered), "check", "crap", "--base", "HEAD")
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d; stderr: %s", code, tc.wantCode, stderr)
			}
			if stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

func TestTheSharedLimitAppliesOnlyToSharedCode(t *testing.T) {
	dir := gitRepo(t, map[string]string{"go.mod": goMod})
	write(t, dir, "shared/a.go", decideFile)
	write(t, dir, "product/a.go", decideFile)
	write(t, dir, "p.out", emptyProfile)
	write(t, dir, ".goalierc", `{"shared": ["shared/**"],
		"rules": {"crap": {"limit": 50, "sharedLimit": 41, "coverProfiles": ["p.out"]}}}`)
	code, stdout, stderr := runGoalie(t, dir, "check", "crap", "--base", "HEAD")
	if code != 1 {
		t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr)
	}
	want := "shared/a.go:3: fail crap: Decide scores 42.0 (complexity 6, coverage 0.0%, limit 41)\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// repoWithOldFunction has Decide in Git, so only a check of the whole repo can see it.
func repoWithOldFunction(t *testing.T, settings string) string {
	t.Helper()
	return gitRepo(t, map[string]string{
		"go.mod":    goMod,
		"a.go":      decideFile,
		"p.out":     emptyProfile,
		".goalierc": settings,
	})
}

func TestEachScopeDecidesWhichFunctionsAreScored(t *testing.T) {
	cases := []struct {
		name       string
		settings   string
		args       []string
		wantCode   int
		wantStdout string
	}{
		{"changes skips old code and runs no tests", `{"rules": {"crap": {"scope": "changes", "coverCommand": "exit 1"}}}`,
			[]string{"check", "crap", "--base", "HEAD"}, 0, ""},
		{"all sees old code with no revision", `{"rules": {"crap": {"scope": "all", "coverProfiles": ["p.out"]}}}`,
			[]string{"check", "crap"}, 1, decideOverDefault},
		{"all sees old code despite a revision", `{"rules": {"crap": {"scope": "all", "coverProfiles": ["p.out"]}}}`,
			[]string{"check", "crap", "--base", "HEAD"}, 1, decideOverDefault},
		{"all follows the mode", `{"rules": {"crap": {"scope": "all", "mode": "warn", "coverProfiles": ["p.out"]}}}`,
			[]string{"check", "crap"}, 0, decideWarn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runGoalie(t, repoWithOldFunction(t, tc.settings), tc.args...)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d; stderr: %s", code, tc.wantCode, stderr)
			}
			if stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

func TestCrapTakesItsRevisionFromTheFlagThenTheFile(t *testing.T) {
	cases := []struct {
		name     string
		settings string
		args     []string
		wantCode int
	}{
		{"the file alone", `{"base": "HEAD", "rules": {"crap": {"coverProfiles": ["p.out"]}}}`, []string{"check", "crap"}, 1},
		{"the flag replaces the file", `{"base": "no-such-rev", "rules": {"crap": {"coverProfiles": ["p.out"]}}}`,
			[]string{"check", "crap", "--base", "HEAD"}, 1},
		{"neither", `{"rules": {"crap": {"coverProfiles": ["p.out"]}}}`, []string{"check", "crap"}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := runGoalie(t, repoWithNewFunction(t, tc.settings, emptyProfile), tc.args...)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d; stderr: %s", code, tc.wantCode, stderr)
			}
		})
	}
}

func TestTheCoverCommandSuppliesCoverage(t *testing.T) {
	dir := repoWithNewFunction(t, `{"rules": {"crap": {"limit": 10, "coverCommand": "`+writeHalf+`"}}}`, emptyProfile)
	code, stdout, stderr := runGoalie(t, dir, "check", "crap", "--base", "HEAD")
	if code != 1 {
		t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr)
	}
	want := "a.go:3: fail crap: Decide scores 10.5 (complexity 6, coverage 50.0%, limit 10)\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestWithNoCoverSettingsTheDefaultCommandRunsTheTests(t *testing.T) {
	dir := gitRepo(t, map[string]string{"go.mod": goMod})
	write(t, dir, "a.go", "package m\n\nfunc One() int {\n\treturn 1\n}\n")
	write(t, dir, "a_test.go", "package m\n\nimport \"testing\"\n\nfunc TestOne(t *testing.T) {\n\tOne()\n}\n")
	// With no coverage, One scores 2. Only a real test run brings it down to 1.
	write(t, dir, ".goalierc", `{"rules": {"crap": {"limit": 1}}}`)
	code, stdout, stderr := runGoalie(t, dir, "check", "crap", "--base", "HEAD")
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
}

func TestAFailedCoverCommandIsAnErrorInEveryMode(t *testing.T) {
	dir := repoWithNewFunction(t, `{"rules": {"crap": {"mode": "warn", "coverCommand": "exit 3"}}}`, emptyProfile)
	code, stdout, stderr := runGoalie(t, dir, "check", "crap", "--base", "HEAD")
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	if !strings.Contains(stderr, "coverage command failed") {
		t.Errorf("stderr = %q, want it to name the failure", stderr)
	}
}

func TestRepoSettingsApplyToCrap(t *testing.T) {
	dir := repoWithNewFunction(t, `{"skipFiles": ["a.go"], "rules": {"crap": {"coverProfiles": ["p.out"]}}}`, emptyProfile)
	code, stdout, stderr := runGoalie(t, dir, "check", "crap", "--base", "HEAD")
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	if !strings.Contains(stderr, "No Go files found.") {
		t.Errorf("stderr = %q, want it to say that it found no Go files", stderr)
	}
}

func TestBadCrapSettingsAreErrors(t *testing.T) {
	cases := []struct {
		name     string
		settings string
		want     string
	}{
		{"unknown mode", `{"rules": {"crap": {"mode": "loud"}}}`, "loud"},
		{"unknown scope", `{"rules": {"crap": {"scope": "some"}}}`, "some"},
		{"unknown setting", `{"rules": {"crap": {"warnOnly": true}}}`, "warnOnly"},
		{"negative limit", `{"rules": {"crap": {"limit": -1, "coverProfiles": ["p.out"]}}}`, "limit"},
		{"negative shared limit", `{"shared": ["a.go"], "rules": {"crap": {"sharedLimit": -1, "coverProfiles": ["p.out"]}}}`, "shared limit"},
		{"shared limit with no shared code", `{"rules": {"crap": {"sharedLimit": 15, "coverProfiles": ["p.out"]}}}`, "shared"},
		{"blank cover command", `{"rules": {"crap": {"coverCommand": "  "}}}`, "coverCommand"},
		{"both cover settings", `{"rules": {"crap": {"coverCommand": "true", "coverProfiles": ["p.out"]}}}`, "not both"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := repoWithNewFunction(t, tc.settings, emptyProfile)
			code, stdout, stderr := runGoalie(t, dir, "check", "crap", "--base", "HEAD")
			if code != 2 {
				t.Errorf("exit code = %d, want 2; stdout: %s", code, stdout)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr = %q, want it to hold %q", stderr, tc.want)
			}
		})
	}
}
