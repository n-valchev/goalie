package cli_test

import (
	"strings"
	"testing"
)

func TestWarnOnlyReportsDuplicatesAndExitsZero(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + sumFunc("One"),
		"b.go": "package a\n\n" + sumFunc("Two"),
	})

	code, stdout, _ := runGodry(t, dir, "--warn-only")

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if want := "a.go:3: duplicates b.go:3 (score 1.00, literals 1.00, anchors 0)\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestWarnOnlyStillExitsTwoOnErrors(t *testing.T) {
	dir := writeTree(t, map[string]string{"bad.go": "package a\n\nfunc broken( {\n"})

	tests := []struct {
		name string
		args []string
	}{
		{"parse error", []string{"--warn-only"}},
		{"missing path", []string{"--warn-only", "missing"}},
		{"invalid flag", []string{"--warn-only", "--threshold", "many"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, _, stderr := runGodry(t, dir, test.args...)
			if code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			if stderr == "" {
				t.Error("stderr is empty")
			}
		})
	}
}

func TestInvalidFlagsExitTwo(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.go": "package a\n"})

	tests := [][]string{
		{"--unknown"},
		{"--threshold", "many"},
		{"--threshold", "1.5"},
		{"--threshold", "-0.1"},
		{"--threshold", "NaN"},
		{"--min-lines", "many"},
		{"--min-lines", "-1"},
		{"--min-nodes", "many"},
		{"--min-nodes", "-1"},
		{"--format", "xml"},
		{"--base"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			code, stdout, stderr := runGodry(t, dir, args...)
			if code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want nothing", stdout)
			}
			if stderr == "" {
				t.Error("stderr is empty")
			}
		})
	}
}

// This checks only that parsing accepts each flag, so that later behavior for a flag can't break it.
func TestEveryFlagInTheSpecIsParsed(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.go": "package a\n"})

	tests := [][]string{
		{"--base", "main"},
		{"--threshold", "0.9"},
		{"--min-lines", "1"},
		{"--min-nodes", "1"},
		{"--format", "text"},
		{"--format", "json"},
		{"--warn-only"},
		{"--include-tests"},
		{"--skip-tags", "integration,e2e"},
		{"--help"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, _, stderr := runGodry(t, dir, args...)
			if strings.Contains(stderr, "not defined") {
				t.Errorf("stderr = %q", stderr)
			}
		})
	}
}

func TestFlagsMayFollowPaths(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"one/a.go": "package one\n\n" + sumFunc("A"),
		"one/b.go": "package one\n\n" + sumFunc("B"),
		"two/c.go": "package two\n\n" + joinFunc("C"),
	})

	code, stdout, _ := runGodry(t, dir, "two", "--warn-only", "one", "--min-lines", "1")

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if want := "one/a.go:3: duplicates one/b.go:3 (score 1.00, literals 1.00, anchors 0)\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestDoubleDashEndsTheFlags(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"-odd/a.go":  "package a\n\n" + sumFunc("A"),
		"-odd/b.go":  "package a\n\n" + sumFunc("B"),
		"-even/c.go": "package a\n\n" + sumFunc("C"),
	})

	_, stdout, stderr := runGodry(t, dir, "--", "-odd")

	if want := "-odd/a.go:3: duplicates -odd/b.go:3 (score 1.00, literals 1.00, anchors 0)\n"; stdout != want {
		t.Errorf("stdout = %q, want %q; stderr: %s", stdout, want, stderr)
	}
}

func TestHelpExitsZero(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		t.Run(flag, func(t *testing.T) {
			code, stdout, _ := runGodry(t, t.TempDir(), flag)
			if code != 0 {
				t.Errorf("exit code = %d, want 0", code)
			}
			if !strings.Contains(stdout, "godry") {
				t.Errorf("stdout = %q", stdout)
			}
		})
	}
}
