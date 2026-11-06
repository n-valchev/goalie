package goalie_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const partialBudget = time.Second

func wantPartial(t *testing.T, dir string, wantCode int, wantStdout string) {
	t.Helper()
	code, stdout, stderr := runGoalie(t, dir, "check", "crap", "--partial", "--base", "HEAD")
	if code != wantCode {
		t.Errorf("exit code = %d, want %d; stderr: %s", code, wantCode, stderr)
	}
	if stdout != wantStdout {
		t.Errorf("stdout = %q, want %q", stdout, wantStdout)
	}
}

// Decide has complexity 6, so c² + c is 42.
func TestEachPartialCaseStartsAtItsExactBoundary(t *testing.T) {
	cases := []struct {
		name       string
		limit      string
		wantCode   int
		wantStdout string
	}{
		{"c² + c = L is safe", "42", 0, ""},
		{"just under c² + c needs coverage", "41", 0, "a.go:3: warn crap: Decide needs at least 1% coverage (complexity 6, limit 41)\n"},
		{"c = L needs full coverage", "6", 0, "a.go:3: warn crap: Decide needs at least 100% coverage (complexity 6, limit 6)\n"},
		{"c = L + 1 can't pass", "5", 1, "a.go:3: fail crap: Decide can't pass at any coverage, so split it (complexity 6, limit 5)\n"},
		{"a limit of 0 can't pass", "0", 1, "a.go:3: fail crap: Decide can't pass at any coverage, so split it (complexity 6, limit 0)\n"},
		// 27% scores 20.005, so a value rounded to the nearest percent would still fail.
		{"the value rounds up", "20", 0, "a.go:3: warn crap: Decide needs at least 28% coverage (complexity 6, limit 20)\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := `{"rules": {"crap": {"limit": ` + tc.limit + `}}}`
			wantPartial(t, repoWithNewFunction(t, settings, emptyProfile), tc.wantCode, tc.wantStdout)
		})
	}
}

func withComplexity(name string, complexity int) string {
	branches := strings.Repeat("\tif a > 0 {\n\t\ta--\n\t}\n", complexity-1)
	return fmt.Sprintf("func %s(a int) int {\n%s\treturn a\n}\n", name, branches)
}

func TestThePRDExamplesHoldAtTheDefaultLimit(t *testing.T) {
	cases := []struct {
		complexity int
		wantCode   int
		wantStdout string
	}{
		{5, 0, ""},
		{10, 0, "a.go:3: warn crap: F needs at least 42% coverage (complexity 10, limit 30)\n"},
		{20, 0, "a.go:3: warn crap: F needs at least 71% coverage (complexity 20, limit 30)\n"},
		{31, 1, "a.go:3: fail crap: F can't pass at any coverage, so split it (complexity 31, limit 30)\n"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint("complexity ", tc.complexity), func(t *testing.T) {
			dir := gitRepo(t, map[string]string{"go.mod": goMod})
			write(t, dir, "a.go", "package m\n\n"+withComplexity("F", tc.complexity))
			wantPartial(t, dir, tc.wantCode, tc.wantStdout)
		})
	}
}

func TestOnlyACantPassResultFollowsTheMode(t *testing.T) {
	const (
		needsCoverage = "a.go:3: warn crap: Decide needs at least 28% coverage (complexity 6, limit 20)\n"
		cantPass      = "a.go:3: fail crap: Decide can't pass at any coverage, so split it (complexity 6, limit 5)\n"
		cantPassWarn  = "a.go:3: warn crap: Decide can't pass at any coverage, so split it (complexity 6, limit 5)\n"
	)
	cases := []struct {
		mode       string
		limit      string
		wantCode   int
		wantStdout string
	}{
		{"fail", "20", 0, needsCoverage},
		{"warn", "20", 0, needsCoverage},
		{"off", "20", 0, needsCoverage},
		{"fail", "5", 1, cantPass},
		{"warn", "5", 0, cantPassWarn},
		{"off", "5", 0, cantPassWarn},
	}
	for _, tc := range cases {
		t.Run(tc.mode+" at limit "+tc.limit, func(t *testing.T) {
			settings := `{"rules": {"crap": {"mode": "` + tc.mode + `", "limit": ` + tc.limit + `}}}`
			wantPartial(t, repoWithNewFunction(t, settings, emptyProfile), tc.wantCode, tc.wantStdout)
		})
	}
}

func TestAPartialCheckUsesNoCoverage(t *testing.T) {
	cases := []struct {
		name     string
		settings string
	}{
		{"a cover command", `{"rules": {"crap": {"limit": 20, "coverCommand": "touch started; exit 1"}}}`},
		{"the default command", `{"rules": {"crap": {"limit": 20}}}`},
		{"a profile that doesn't exist", `{"rules": {"crap": {"limit": 20, "coverProfiles": ["missing.out"]}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := repoWithNewFunction(t, tc.settings, emptyProfile)
			// A real test run can't compile this file, so it would end the check with an error.
			write(t, dir, "a_test.go", "package m\n\nnot Go code\n")
			wantPartial(t, dir, 0, "a.go:3: warn crap: Decide needs at least 28% coverage (complexity 6, limit 20)\n")
			if _, err := os.Stat(filepath.Join(dir, "started")); err == nil {
				t.Error("the cover command ran")
			}
		})
	}
}

func TestAPartialCheckReportsOnlyWhatTheScopeCovers(t *testing.T) {
	cases := []struct {
		name       string
		settings   string
		wantStdout string
	}{
		{"changes skips old code", `{"rules": {"crap": {"limit": 20}}}`, ""},
		{"all sees old code", `{"rules": {"crap": {"limit": 20, "scope": "all"}}}`,
			"a.go:3: warn crap: Decide needs at least 28% coverage (complexity 6, limit 20)\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantPartial(t, repoWithOldFunction(t, tc.settings), 0, tc.wantStdout)
		})
	}
}

func TestAPartialCheckGivesNoResultForFunctionsWithNoComplexity(t *testing.T) {
	dir := gitRepo(t, map[string]string{"go.mod": goMod})
	write(t, dir, "a.go", "package m\n\nfunc Assembly(a int) int\n\n//goalie:ignore crap generated\n"+withComplexity("Ignored", 31))
	wantPartial(t, dir, 0, "")
}

func TestAPartialCheckUsesTheSharedLimit(t *testing.T) {
	dir := gitRepo(t, map[string]string{"go.mod": goMod})
	write(t, dir, "shared/a.go", decideFile)
	write(t, dir, "product/a.go", decideFile)
	write(t, dir, ".goalierc", `{"shared": ["shared/**"], "rules": {"crap": {"limit": 42, "sharedLimit": 5}}}`)
	wantPartial(t, dir, 1, "shared/a.go:3: fail crap: Decide can't pass at any coverage, so split it (complexity 6, limit 5)\n")
}

func TestPartialResultsStartWithTheHighestComplexityOverLimit(t *testing.T) {
	dir := gitRepo(t, map[string]string{"go.mod": goMod})
	write(t, dir, "a.go", "package m\n\n"+withComplexity("Near", 20)+"\n"+withComplexity("Nearer", 30))
	write(t, dir, "z.go", "package m\n\n"+withComplexity("Over", 31))
	wantPartial(t, dir, 1, "z.go:3: fail crap: Over can't pass at any coverage, so split it (complexity 31, limit 30)\n"+
		"a.go:64: warn crap: Nearer needs at least 100% coverage (complexity 30, limit 30)\n"+
		"a.go:3: warn crap: Near needs at least 71% coverage (complexity 20, limit 30)\n")
}

func TestACantPassResultComesFirstEvenAtALimitOfZero(t *testing.T) {
	dir := gitRepo(t, map[string]string{"go.mod": goMod})
	write(t, dir, "a.go", decideFile)
	write(t, dir, "shared/z.go", "package shared\n\n"+withComplexity("One", 1))
	write(t, dir, ".goalierc", `{"shared": ["shared/**"], "rules": {"crap": {"limit": 6, "sharedLimit": 0}}}`)
	wantPartial(t, dir, 1, "shared/z.go:3: fail crap: One can't pass at any coverage, so split it (complexity 1, limit 0)\n"+
		"a.go:3: warn crap: Decide needs at least 100% coverage (complexity 6, limit 6)\n")
}

func TestDupGivesTheSameResultInAFullAndAPartialCheck(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n\n" + sumFunc("Total")})
	write(t, dir, "b.go", "package a\n\n"+sumFunc("Sum"))
	fullCode, fullStdout, _ := runGoalie(t, dir, "check", "dup", "--base", "HEAD")
	code, stdout, stderr := runGoalie(t, dir, "check", "dup", "--partial", "--base", "HEAD")
	if fullCode != 1 || code != fullCode {
		t.Errorf("exit codes = %d and %d, want 1 and 1; stderr: %s", fullCode, code, stderr)
	}
	if stdout != fullStdout {
		t.Errorf("stdout = %q, want %q", stdout, fullStdout)
	}
}

func TestCrapParsesOnlyChangedFilesWhenItHasARevision(t *testing.T) {
	dir := gitRepo(t, map[string]string{"go.mod": goMod, "old.go": "package m\n\nfunc broken( {\n"})
	write(t, dir, "a.go", decideFile)
	write(t, dir, ".goalierc", `{"rules": {"crap": {"limit": 20}}}`)
	wantPartial(t, dir, 0, "a.go:3: warn crap: Decide needs at least 28% coverage (complexity 6, limit 20)\n")

	write(t, dir, "a.go", "package m\n\nfunc broken( {\n")
	code, _, stderr := runGoalie(t, dir, "check", "crap", "--partial", "--base", "HEAD")
	if code != 2 {
		t.Errorf("exit code = %d, want 2 for a changed file that doesn't parse", code)
	}
	if !strings.Contains(stderr, "a.go") {
		t.Errorf("stderr = %q, want it to name a.go", stderr)
	}
}

func TestAPartialCheckStillNeedsARevision(t *testing.T) {
	code, _, stderr := runGoalie(t, repoWithNewFunction(t, `{}`, emptyProfile), "check", "crap", "--partial")
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "revision") {
		t.Errorf("stderr = %q, want it to say that the check needs a revision", stderr)
	}
}

// The size of the monorepo in the spec, so that a time here says something about that repo.
func TestOneEditedFileChecksWithinTheBudget(t *testing.T) {
	const fileCount, functionCount = 8825, 11521
	texts := make([]strings.Builder, fileCount)
	for i := range texts {
		fmt.Fprintf(&texts[i], "package pkg%d\n\n", i/10)
	}
	for i := range functionCount {
		texts[i%fileCount].WriteString(withComplexity(fmt.Sprintf("F%05d", i), 1+i%20) + "\n")
	}
	repo := map[string]string{"go.mod": goMod, "total/total.go": "package total\n\n" + sumFunc("Total")}
	for i := range texts {
		repo[fmt.Sprintf("pkg%d/file%d.go", i/10, i)] = texts[i].String()
	}
	dir := gitRepo(t, repo)
	write(t, dir, "edited/edited.go", "package edited\n\n"+withComplexity("Over", 31)+"\n"+sumFunc("Sum"))

	for _, names := range [][]string{{"crap"}, {"dup"}, nil} {
		args := append(append([]string{"check"}, names...), "--partial", "--base", "HEAD")
		// The best of three, because the budget is for a machine that is not busy, and the file cache
		// is cold for a first run.
		best := time.Hour
		for range 3 {
			start := time.Now()
			code, stdout, stderr := runGoalie(t, dir, args...)
			best = min(best, time.Since(start))

			if code != 1 {
				t.Errorf("%v: exit code = %d, want 1; stderr: %s", names, code, stderr)
			}
			if !strings.Contains(stdout, "edited/edited.go") {
				t.Errorf("%v: stdout = %.200q, want a result for the edited file", names, stdout)
			}
		}
		t.Logf("%v: %v", names, best)
		if best > partialBudget {
			t.Errorf("%v took %v, want at most %v", names, best, partialBudget)
		}
	}
}
