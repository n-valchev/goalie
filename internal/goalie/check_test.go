package goalie_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/n-valchev/goalie/internal/goalie"
)

func runGoalie(t *testing.T, dir string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = goalie.Run(args, dir, strings.NewReader(""), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestCheckWithNoRevisionStopsWithAnError(t *testing.T) {
	code, stdout, stderr := runGoalie(t, t.TempDir(), "check", "dup")
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	if !strings.Contains(stderr, "revision") {
		t.Errorf("stderr = %q, want it to say that the check needs a revision", stderr)
	}
}

func TestEveryCommandLineMistakeExitsWithTwo(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n\n" + sumFunc("Total")})
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no command", nil, "command"},
		{"unknown command", []string{"lint"}, `"lint"`},
		{"unknown rule", []string{"check", "dupe", "--base", "HEAD"}, `"dupe"`},
		{"unknown flag", []string{"check", "dup", "--bse", "HEAD"}, "bse"},
		{"empty revision", []string{"check", "dup", "--base", ""}, "revision"},
		{"unknown revision", []string{"check", "dup", "--base", "no-such-rev"}, "no-such-rev"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := runGoalie(t, dir, tc.args...)
			if code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr = %q, want it to hold %s", stderr, tc.want)
			}
		})
	}
}

func TestDupIsCleanWhenTheChangeCopiesNothing(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n\n" + sumFunc("Total")})
	write(t, dir, "b.go", "package a\n\nfunc Small() int {\n\treturn 1\n}\n")

	code, _, stderr := runGoalie(t, dir, "check", "dup", "--base", "HEAD")

	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
}

func TestDupIgnoresOldCopiesThatTheChangeDoesNotTouch(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"a.go": "package a\n\n" + sumFunc("Total"),
		"b.go": "package a\n\n" + sumFunc("OldCopy"),
	})

	code, stdout, stderr := runGoalie(t, dir, "check", "dup", "--base", "HEAD")

	if code != 0 {
		t.Errorf("exit code = %d, want 0; stdout: %s; stderr: %s", code, stdout, stderr)
	}
}

func TestDupFollowsTheRepoSettingsAndIgnoreFile(t *testing.T) {
	cases := []struct {
		name string
		file string
		text string
	}{
		{"settings file", ".goalierc", `{"skipFiles": ["b.go"]}`},
		{"ignore file", ".goalieignore", "b.go\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := gitRepo(t, map[string]string{"a.go": "package a\n\n" + sumFunc("Total")})
			write(t, dir, "b.go", "package a\n\n"+sumFunc("Copied"))
			write(t, dir, tc.file, tc.text)

			code, stdout, stderr := runGoalie(t, dir, "check", "dup", "--base", "HEAD")

			if code != 0 {
				t.Errorf("exit code = %d, want 0; stdout: %s; stderr: %s", code, stdout, stderr)
			}
		})
	}
}

func TestABrokenSettingsFileExitsWithTwo(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n\n" + sumFunc("Total")})
	write(t, dir, ".goalierc", `{"skipFiless": []}`)

	code, _, stderr := runGoalie(t, dir, "check", "dup", "--base", "HEAD")

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, ".goalierc") {
		t.Errorf("stderr = %q, want it to name .goalierc", stderr)
	}
}

func TestHelpPrintsUsage(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"check", "--help"}} {
		code, stdout, stderr := runGoalie(t, t.TempDir(), args...)
		if code != 0 {
			t.Errorf("%v: exit code = %d, want 0; stderr: %s", args, code, stderr)
		}
		if !strings.Contains(stdout, "goalie check") {
			t.Errorf("%v: stdout = %q, want the usage", args, stdout)
		}
	}
}

func TestDupReportsANewCopyOfAnExistingFunction(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n\n" + sumFunc("Total")})
	write(t, dir, "b.go", "package a\n\n"+sumFunc("Copied"))

	code, stdout, stderr := runGoalie(t, dir, "check", "dup", "--base", "HEAD")

	if code != 1 {
		t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr)
	}
	want := "b.go:3: fail dup: duplicates a.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}
