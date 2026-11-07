package goalie_test

import (
	"fmt"
	"strings"
	"testing"
)

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

			code, _, _ := runDup(t, dir)

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

	code, _, _ := runDup(t, dir)

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestMinLinesChangesTheLimit(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + linesFunc("One", 6),
		"b.go": "package a\n\n" + linesFunc("Two", 6),
		"c.go": "package a\n\n" + sumFunc("Three"),
		"d.go": "package a\n\n" + sumFunc("Four"),
	})

	write(t, dir, ".goalierc", `{"rules": {"dup": {"minLines": 6}}}`)
	_, stdout, _ := runDup(t, dir)
	if want := "a.go:3: fail dup: duplicates b.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 1)\nc.go:3: fail dup: duplicates d.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"; stdout != want {
		t.Errorf("minLines 6: stdout = %q, want %q", stdout, want)
	}

	write(t, dir, ".goalierc", `{"rules": {"dup": {"minLines": 13}}}`)
	_, stdout, _ = runDup(t, dir)
	if want := ""; stdout != want {
		t.Errorf("minLines 13: stdout = %q, want %q", stdout, want)
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

	code, _, _ := runDup(t, dir)
	if code != 0 {
		t.Errorf("default limit: exit code = %d, want 0", code)
	}

	write(t, dir, ".goalierc", `{"rules": {"dup": {"minNodes": 0}}}`)
	code, _, _ = runDup(t, dir)
	if code != 1 {
		t.Errorf("minNodes 0: exit code = %d, want 1", code)
	}
}

func TestMinNodesRaisesTheLimit(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go":      "package a\n\n" + sumFunc("One"),
		"b.go":      "package a\n\n" + sumFunc("Two"),
		".goalierc": `{"rules": {"dup": {"minNodes": 1000}}}`,
	})

	code, _, _ := runDup(t, dir)

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestThresholdChangesTheLimit(t *testing.T) {
	near := strings.Replace(sumFunc("Two"), "return total\n", "total++\n\treturn total\n", 1)
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + sumFunc("One"),
		"b.go": "package a\n\n" + near,
	})

	write(t, dir, ".goalierc", `{"rules": {"dup": {"threshold": 0.5}}}`)
	code, stdout, _ := runDup(t, dir)
	if code != 1 || !strings.HasPrefix(stdout, "a.go:3: fail dup: duplicates b.go:3 (score 0.") {
		t.Errorf("threshold 0.5: exit code = %d, stdout = %q", code, stdout)
	}

	write(t, dir, ".goalierc", `{"rules": {"dup": {"threshold": 1}}}`)
	code, _, _ = runDup(t, dir)
	if code != 0 {
		t.Errorf("threshold 1: exit code = %d, want 0", code)
	}
}
