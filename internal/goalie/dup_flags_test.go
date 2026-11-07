package goalie_test

import (
	"strings"
	"testing"
)

func TestWarnOnlyReportsDuplicatesAndExitsZero(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + sumFunc("One"),
		"b.go": "package a\n\n" + sumFunc("Two"),
	})

	code, stdout, _ := runDup(t, dir, "--warn-only")

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if want := "a.go:3: warn dup: duplicates b.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestWarnOnlyStillExitsTwoOnErrors(t *testing.T) {
	tests := []struct {
		name     string
		settings string
		args     []string
	}{
		{"parse error", `{}`, nil},
		{"missing path", `{"paths": ["missing"]}`, nil},
		{"invalid flag", `{}`, []string{"--limit", "many"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{"bad.go": "package a\n\nfunc broken( {\n", ".goalierc": test.settings})
			code, _, stderr := runDup(t, dir, append([]string{"--warn-only"}, test.args...)...)
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
		{"--limit", "many"},
		{"--limit", "1.5"},
		{"--limit", "-0.1"},
		{"--limit", "NaN"},
		{"--format", "xml"},
		{"--base"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			code, stdout, stderr := runDup(t, dir, args...)
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
func TestEveryFlagOfCheckIsParsed(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.go": "package a\n"})

	tests := [][]string{
		{"--base", "main"},
		{"--limit", "0.9"},
		{"--format", "text"},
		{"--format", "json"},
		{"--warn-only"},
		{"--partial"},
		{"--help"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, _, stderr := runDup(t, dir, args...)
			if strings.Contains(stderr, "not defined") {
				t.Errorf("stderr = %q", stderr)
			}
		})
	}
}

func TestFlagsMayComeBeforeAndAfterTheRuleName(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + sumFunc("A"),
		"b.go": "package a\n\n" + sumFunc("B"),
	})

	code, stdout, _ := runGoalie(t, dir, "check", "--all", "dup", "--warn-only")

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if want := "a.go:3: warn dup: duplicates b.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestDoubleDashEndsTheFlags(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + sumFunc("A"),
		"b.go": "package a\n\n" + sumFunc("B"),
	})

	if code, _, stderr := runGoalie(t, dir, "check", "--all", "--", "dup"); code != 1 {
		t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr)
	}
	if code, _, stderr := runGoalie(t, dir, "check", "--all", "--", "--warn-only"); code != 2 || !strings.Contains(stderr, `"--warn-only"`) {
		t.Errorf("exit code = %d, stderr = %q, want the flag read as an unknown rule", code, stderr)
	}
}

func TestShortHelpExitsZero(t *testing.T) {
	code, stdout, _ := runGoalie(t, t.TempDir(), "check", "-h")
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "goalie check") {
		t.Errorf("stdout = %q", stdout)
	}
}
