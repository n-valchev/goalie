package goalie_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const coverSettings = `{"rules": {"crap": {"coverProfiles": ["p.out"]}}}`

// repoWithIgnore holds two new copies of Decide, so that both rules report on the one in a.go until
// the directives above it excuse it.
func repoWithIgnore(t *testing.T, directives ...string) string {
	t.Helper()
	dir := repoWithNewFunction(t, coverSettings, emptyProfile)
	above := strings.Join(append(directives, ""), "\n")
	write(t, dir, "a.go", "package m\n\n"+above+decide)
	write(t, dir, "b.go", "package m\n\n"+strings.ReplaceAll(decide, "Decide", "Other"))
	return dir
}

func wantClean(t *testing.T, dir string, args ...string) {
	t.Helper()
	code, stdout, stderr := runGoalie(t, dir, args...)
	if code != 0 || stdout != "" {
		t.Errorf("exit code = %d, stdout = %q, want 0 and nothing; stderr: %s", code, stdout, stderr)
	}
}

func TestGoalieIgnoreExcusesAFunctionFromTheRulesItNames(t *testing.T) {
	cases := []struct {
		name       string
		directives []string
		wantCrap   bool
		wantDup    bool
	}{
		{"no directive", nil, true, true},
		{"crap alone leaves dup", []string{"//goalie:ignore crap The table needs every branch."}, false, true},
		{"dup alone leaves crap", []string{"//goalie:ignore dup Wiring repeats on purpose."}, true, false},
		{"two rules", []string{"//goalie:ignore crap,dup Generated code."}, false, false},
		{"two directives", []string{"//goalie:ignore crap The table needs every branch.", "//goalie:ignore dup Wiring repeats on purpose."}, false, false},
		{"under a doc comment", []string{"// Decide picks a number.", "//goalie:ignore crap The table needs every branch."}, false, true},
		{"another tool's directive does nothing", []string{"//lint:ignore crap,dup Other tool."}, true, true},
	}
	for _, tc := range cases {
		for _, scope := range [][]string{{"--base", "HEAD"}, {"--all"}} {
			t.Run(tc.name+" "+scope[0], func(t *testing.T) {
				args := append([]string{"check"}, scope...)
				code, stdout, stderr := runGoalie(t, repoWithIgnore(t, tc.directives...), args...)
				if code != 1 {
					t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr)
				}
				if got := strings.Contains(stdout, "crap: Decide scores"); got != tc.wantCrap {
					t.Errorf("crap result for Decide = %v, want %v; stdout: %s", got, tc.wantCrap, stdout)
				}
				if got := strings.Contains(stdout, " dup: "); got != tc.wantDup {
					t.Errorf("dup result = %v, want %v; stdout: %s", got, tc.wantDup, stdout)
				}
			})
		}
	}
}

func TestAMistakeInGoalieIgnoreExitsWithTwo(t *testing.T) {
	cases := []struct {
		name       string
		directive  string
		wantStderr string
	}{
		{"no reason", "//goalie:ignore crap", "a.go:3: //goalie:ignore needs a reason"},
		{"no reason after two rules", "//goalie:ignore crap,dup", "a.go:3: //goalie:ignore needs a reason"},
		{"nothing at all", "//goalie:ignore", "a.go:3: //goalie:ignore needs rule names and a reason"},
		{"unknown name", `//goalie:ignore dupe Copied on purpose.`, `a.go:3: //goalie:ignore names unknown rule "dupe"`},
		{"unknown name beside a real one", "//goalie:ignore crap,dupe Copied on purpose.", `names unknown rule "dupe"`},
		{"unknown name with no reason", "//goalie:ignore dupe", `names unknown rule "dupe"`},
		{"a reason with no names", "//goalie:ignore Copied on purpose.", `names unknown rule "Copied"`},
		{"a space after the comma", "//goalie:ignore crap, dup Copied on purpose.", `names unknown rule ""`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runGoalie(t, repoWithIgnore(t, tc.directive), "check", "--base", "HEAD")
			if code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want nothing", stdout)
			}
			if !strings.Contains(stderr, tc.wantStderr) {
				t.Errorf("stderr = %q, want %q", stderr, tc.wantStderr)
			}
		})
	}
}

func TestAMistakeInGoalieIgnoreStopsARuleThatItDoesNotName(t *testing.T) {
	code, _, stderr := runGoalie(t, repoWithIgnore(t, "//goalie:ignore crap"), "check", "dup", "--base", "HEAD")
	if code != 2 {
		t.Errorf("exit code = %d, want 2; stderr: %s", code, stderr)
	}
}

func TestAMistakeInGoalieIgnoreAboveAFunctionWithNoBodyExitsWithTwo(t *testing.T) {
	dir := gitRepo(t, map[string]string{"go.mod": goMod})
	write(t, dir, "a.go", "package m\n\n//goalie:ignore crap\nfunc Assembly(a int) int\n")
	wantPartial(t, dir, 2, "")
}

func TestAPartialCheckFollowsGoalieIgnore(t *testing.T) {
	settings := `{"rules": {"crap": {"limit": 5}}}`
	dir := repoWithNewFunction(t, settings, emptyProfile)
	wantPartial(t, dir, 1, "a.go:3: fail crap: Decide can't pass at any coverage, so split it (complexity 6, limit 5)\n")
	write(t, dir, "a.go", "package m\n\n//goalie:ignore crap The table needs every branch.\n"+decide)
	wantPartial(t, dir, 0, "")
}

func TestAChangeToAnExcusedFunctionAloneRunsNoTests(t *testing.T) {
	dir := repoWithNewFunction(t, `{"rules": {"crap": {"coverCommand": "touch started"}}}`, emptyProfile)
	write(t, dir, "a.go", "package m\n\n//goalie:ignore crap The table needs every branch.\n"+decide)
	wantClean(t, dir, "check", "crap", "--base", "HEAD")
	if _, err := os.Stat(filepath.Join(dir, "started")); err == nil {
		t.Error("the cover command ran")
	}
}

// A copy of an excused function is no duplicate either, because the directive says the code repeats
// on purpose.
func TestDupIgnoreCoversTheOldSideOfAPair(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"a.go":      "package a\n\n//goalie:ignore dup Wiring repeats on purpose.\n" + sumFunc("Total"),
		".goalierc": `{"rules": {` + otherRulesOff + `}}`,
	})
	write(t, dir, "b.go", "package a\n\n"+sumFunc("Copied"))
	wantClean(t, dir, "check", "--base", "HEAD")
}
