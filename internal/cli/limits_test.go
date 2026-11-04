package cli_test

import (
	"fmt"
	"strings"
	"testing"
)

// linesFunc has exactly the given number of lines, and the same structure for a given count.
func linesFunc(name string, lines int) string {
	var body strings.Builder
	for i := 0; i < lines-3; i++ {
		fmt.Fprintf(&body, "\ttotal += compute(total, %d)\n", i)
	}
	return fmt.Sprintf("func %s(total int) int {\n%s\treturn total\n}\n", name, body.String())
}

func TestDefaultMinimumLinesIsNine(t *testing.T) {
	tests := []struct {
		lines int
		code  int
	}{
		{8, 0},
		{9, 1},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%d lines", test.lines), func(t *testing.T) {
			dir := writeTree(t, map[string]string{
				"a.go": "package a\n\n" + linesFunc("One", test.lines),
				"b.go": "package a\n\n" + linesFunc("Two", test.lines),
			})

			code, _, _ := runGodry(t, dir)

			if code != test.code {
				t.Errorf("exit code = %d, want %d", code, test.code)
			}
		})
	}
}

func TestDocCommentDoesNotCountTowardLines(t *testing.T) {
	comment := "// One adds.\n// It adds more.\n"
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + comment + linesFunc("One", 8),
		"b.go": "package a\n\n" + comment + linesFunc("Two", 8),
	})

	code, _, _ := runGodry(t, dir)

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestMinLinesFlagChangesTheLimit(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + linesFunc("One", 6),
		"b.go": "package a\n\n" + linesFunc("Two", 6),
		"c.go": "package a\n\n" + sumFunc("Three"),
		"d.go": "package a\n\n" + sumFunc("Four"),
	})

	_, stdout, _ := runGodry(t, dir, "--min-lines", "6")
	if want := "a.go:3: duplicates b.go:3 (score 1.00, literals 1.00, anchors 1)\nc.go:3: duplicates d.go:3 (score 1.00, literals 1.00, anchors 0)\n"; stdout != want {
		t.Errorf("--min-lines 6: stdout = %q, want %q", stdout, want)
	}

	_, stdout, _ = runGodry(t, dir, "--min-lines", "13")
	if want := "No duplicates found.\n"; stdout != want {
		t.Errorf("--min-lines 13: stdout = %q, want %q", stdout, want)
	}
}

func TestDefaultMinimumNodesFiltersThinFunctions(t *testing.T) {
	thin := func(name string) string {
		return "func " + name + "() {\n\n\n\n\n\n\n\n\n}\n"
	}
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + thin("One"),
		"b.go": "package a\n\n" + thin("Two"),
	})

	code, _, _ := runGodry(t, dir)
	if code != 0 {
		t.Errorf("default limit: exit code = %d, want 0", code)
	}

	code, _, _ = runGodry(t, dir, "--min-nodes", "0")
	if code != 1 {
		t.Errorf("--min-nodes 0: exit code = %d, want 1", code)
	}
}

func TestMinNodesFlagRaisesTheLimit(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + sumFunc("One"),
		"b.go": "package a\n\n" + sumFunc("Two"),
	})

	code, _, _ := runGodry(t, dir, "--min-nodes", "1000")

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestThresholdFlagChangesTheLimit(t *testing.T) {
	near := strings.Replace(sumFunc("Two"), "return total\n", "total++\n\treturn total\n", 1)
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + sumFunc("One"),
		"b.go": "package a\n\n" + near,
	})

	code, stdout, _ := runGodry(t, dir, "--threshold", "0.5")
	if code != 1 || !strings.HasPrefix(stdout, "a.go:3: duplicates b.go:3 (score 0.") {
		t.Errorf("--threshold 0.5: exit code = %d, stdout = %q", code, stdout)
	}

	code, _, _ = runGodry(t, dir, "--threshold", "1")
	if code != 0 {
		t.Errorf("--threshold 1: exit code = %d, want 0", code)
	}
}
