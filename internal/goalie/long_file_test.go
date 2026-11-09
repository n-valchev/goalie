package goalie_test

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// onlyLongFile turns off the other rules, because these repos have no module for crap to test.
var onlyLongFile = `{"rules": {` + rulesOff("long-file") + `}}`

func withLongFile(settings string) string {
	return `{"rules": {` + rulesOff("long-file") + `, "long-file": ` + settings + `}}`
}

func fileOfLines(lines int) string {
	return "package a\n" + strings.Repeat("//\n", lines-1)
}

func longFileWarn(file string, lines int) string {
	return fmt.Sprintf("%s:1: warn long-file: %s has %d lines (limit 1000)\n", file, file, lines)
}

func TestLongFileReportsAFileOverOneThousandLines(t *testing.T) {
	cases := []struct {
		lines      int
		wantStdout string
	}{
		{1000, ""},
		{1001, longFileWarn("a.go", 1001)},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.lines), func(t *testing.T) {
			dir := gitRepo(t, map[string]string{"a.go": fileOfLines(tc.lines), ".goalierc": withLongFile(`{"scope": "all"}`)})
			code, stdout, stderr := runGoalie(t, dir, "check")
			if code != 0 {
				t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
			}
			if stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

func TestLongFileReportsOnlyAChangeThatCrossesTheLimit(t *testing.T) {
	cases := []struct {
		name       string
		base, now  int
		wantStdout string
	}{
		{"an old file over the limit stays quiet", 1200, 1201, ""},
		{"a change from 1000 to 1001 lines reports", 1000, 1001, longFileWarn("a.go", 1001)},
		{"a change that stays at the limit stays quiet", 900, 1000, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := gitRepo(t, map[string]string{"a.go": fileOfLines(tc.base), ".goalierc": onlyLongFile})
			write(t, dir, "a.go", fileOfLines(tc.now))
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

// A wrong path at the base would make the old file look new, and then it would report.
func TestAnOldLongFileStaysQuietFromASubfolder(t *testing.T) {
	root := gitRepo(t, map[string]string{"sub/a.go": fileOfLines(1200), "sub/.goalierc": onlyLongFile})
	write(t, root, "sub/a.go", fileOfLines(1201))
	code, stdout, stderr := runGoalie(t, filepath.Join(root, "sub"), "check", "--base", "HEAD")
	if code != 0 || stdout != "" {
		t.Errorf("got exit code %d and stdout %q, want 0 and nothing; stderr: %s", code, stdout, stderr)
	}
}

// repoWithCrossingFile takes a.go from 1,000 lines at the base to 1,001 lines now.
func repoWithCrossingFile(t *testing.T, settings string) string {
	t.Helper()
	dir := gitRepo(t, map[string]string{"a.go": fileOfLines(1000), ".goalierc": settings})
	write(t, dir, "a.go", fileOfLines(1001))
	return dir
}

// The stand-in git fails for cat-file alone, which is the call that reads the files at the base.
func TestAFailedReadAtTheBaseGivesNoResult(t *testing.T) {
	dir := repoWithCrossingFile(t, onlyLongFile)
	wrapGit(t, `case "$*" in *cat-file*) exit 1;; esac`)
	code, stdout, stderr := runGoalie(t, dir, "check", "--base", "HEAD")
	if code != 0 || stdout != "" {
		t.Errorf("got exit code %d and stdout %q, want 0 and nothing; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "can't read the files at --base") {
		t.Errorf("stderr = %q, want it to say why file rules report nothing", stderr)
	}
}

// At the base, b.go is a folder, so it has no line count. a.go shows that the other files still
// report.
func TestAFileWithNoBaseValueGivesNoResult(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": fileOfLines(1000), "b.go/notes.txt": "notes\n", ".goalierc": onlyLongFile})
	git(t, dir, "rm", "-r", "--quiet", "b.go")
	git(t, dir, "commit", "--message", "second")
	write(t, dir, "a.go", fileOfLines(1001))
	write(t, dir, "b.go", fileOfLines(1001))
	code, stdout, stderr := runGoalie(t, dir, "check", "--base", "HEAD~1")
	if code != 0 || stdout != longFileWarn("a.go", 1001) {
		t.Errorf("got exit code %d and stdout %q, want 0 and %q; stderr: %s", code, stdout, longFileWarn("a.go", 1001), stderr)
	}
}

func TestEachModeDecidesWhatALongFileDoes(t *testing.T) {
	cases := []struct {
		name       string
		settings   string
		wantCode   int
		wantStdout string
	}{
		{"off says nothing", `{"mode": "off"}`, 0, ""},
		{"warn reports and passes", `{"mode": "warn"}`, 0, longFileWarn("a.go", 1001)},
		{"fail fails", `{"mode": "fail"}`, 1, "a.go:1: fail long-file: a.go has 1001 lines (limit 1000)\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := repoWithCrossingFile(t, withLongFile(tc.settings))
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

func TestAllReportsAnOldLongFileInARuleSetToChanges(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": fileOfLines(1200), ".goalierc": withLongFile(`{"scope": "changes"}`)})
	code, stdout, stderr := runGoalie(t, dir, "check", "--all")
	if code != 0 || stdout != longFileWarn("a.go", 1200) {
		t.Errorf("got exit code %d and stdout %q, want 0 and %q; stderr: %s", code, stdout, longFileWarn("a.go", 1200), stderr)
	}
}

func TestTheLongFileLimitComesFromTheFlag(t *testing.T) {
	dir := repoWithCrossingFile(t, onlyLongFile)
	code, stdout, stderr := runGoalie(t, dir, "check", "long-file", "--limit", "1001", "--base", "HEAD")
	if code != 0 || stdout != "" {
		t.Errorf("got exit code %d and stdout %q, want 0 and nothing; stderr: %s", code, stdout, stderr)
	}
}

func TestLongFileSkipsTestFiles(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n", ".goalierc": onlyLongFile})
	write(t, dir, "a_test.go", fileOfLines(1001))
	code, stdout, stderr := runGoalie(t, dir, "check", "--base", "HEAD")
	if code != 0 || stdout != "" {
		t.Errorf("got exit code %d and stdout %q, want 0 and nothing; stderr: %s", code, stdout, stderr)
	}
}

// A function rule runs too, because only a function rule reads the directive.
func TestGoalieIgnoreLongFileIsAnUnknownRule(t *testing.T) {
	settings := `{"rules": {` + rulesOff("long-file", "long-function") + `}}`
	dir := repoWithNewCode(t, settings, "//goalie:ignore long-file The file holds one table.\n"+longFunc("Parse", 3))
	code, stdout, stderr := runGoalie(t, dir, "check", "--base", "HEAD")
	if code != 2 || stdout != "" || !strings.Contains(stderr, `names unknown rule "long-file"`) {
		t.Errorf("got exit code %d, stdout %q, stderr %q; want 2, nothing, and an unknown rule", code, stdout, stderr)
	}
}

func TestALongFileResultHasAJSONForm(t *testing.T) {
	dir := repoWithCrossingFile(t, onlyLongFile)
	code, stdout, stderr := runGoalie(t, dir, "check", "--format", "json", "--base", "HEAD")
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	results := parseResults(t, stdout)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1: %s", len(results), stdout)
	}
	wantFields(t, results[0], map[string]any{
		"rule": "long-file", "mode": "warn", "value": 1001.0, "limit": 1000.0, "file": "a.go", "line": 1.0,
	})
	if _, ok := results[0]["function"]; ok {
		t.Errorf("result = %v, want no function field", results[0])
	}
}

var commitHash = regexp.MustCompile(`\b[0-9a-f]{40}\b`)

// The log of one changed file and of three must hold the same commands, so a large change adds no
// Git call.
func TestAPartialCheckRunsGitAFixedNumberOfTimes(t *testing.T) {
	logs := map[int]string{}
	for _, changed := range []int{1, 3} {
		dir := gitRepo(t, map[string]string{"a.go": "package a\n", ".goalierc": `{}`})
		for i := range changed {
			write(t, dir, fmt.Sprintf("f%d.go", i), fileOfLines(1001))
		}
		log := wrapGit(t, "")
		code, stdout, stderr := runGoalie(t, dir, "check", "--partial", "--base", "HEAD")
		if code != 0 || strings.Count(stdout, "long-file") != changed {
			t.Errorf("%d files: got exit code %d and stdout %q, want 0 and %d results; stderr: %s",
				changed, code, stdout, changed, stderr)
		}
		// The commit hash differs between the two repos when their commits fall in different seconds.
		logs[changed] = commitHash.ReplaceAllString(readLog(t, log), "<hash>")
	}
	if logs[1] != logs[3] {
		t.Errorf("git ran\n%s\nfor 1 file, and\n%s\nfor 3 files; want the same commands", logs[1], logs[3])
	}
	if reads := strings.Count(logs[3], "cat-file"); reads != 1 {
		t.Errorf("git cat-file ran %d times, want 1:\n%s", reads, logs[3])
	}
}

func TestANewLongFileReports(t *testing.T) {
	cases := []struct {
		name   string
		commit bool
	}{
		{"untracked", false},
		{"committed after the base", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := gitRepo(t, map[string]string{"a.go": "package a\n", ".goalierc": onlyLongFile})
			write(t, dir, "b.go", fileOfLines(1001))
			base := "HEAD"
			if tc.commit {
				git(t, dir, "add", "b.go")
				git(t, dir, "commit", "--message", "second")
				base = "HEAD~1"
			}
			code, stdout, stderr := runGoalie(t, dir, "check", "--base", base)
			if code != 0 || stdout != longFileWarn("b.go", 1001) {
				t.Errorf("got exit code %d and stdout %q, want 0 and %q; stderr: %s", code, stdout, longFileWarn("b.go", 1001), stderr)
			}
		})
	}
}
