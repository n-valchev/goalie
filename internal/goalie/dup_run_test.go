package goalie_test

import (
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestCopyInAnotherPackageIsReported(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"internal/a/a.go": "package a\n\n" + sumFunc("Sum"),
		"internal/b/b.go": "package b\n\nvar limit = 1\n\n" + sumFunc("Total"),
	})

	code, stdout, stderr := runDup(t, dir)

	if code != 1 {
		t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr)
	}
	want := "internal/a/a.go:3: fail dup: duplicates internal/b/b.go:5 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestCommentsInsideAFunctionDoNotChangeTheScore(t *testing.T) {
	commented := strings.Replace(sumFunc("Two"), "\ttotal := 0\n",
		"\t// The running sum.\n\tvar total int // Starts at zero.\n", 1)
	plain := strings.Replace(sumFunc("One"), "\ttotal := 0\n", "\tvar total int\n", 1)
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + plain,
		"b.go": "package a\n\n" + commented,
	})

	_, stdout, _ := runDup(t, dir)

	if want := "a.go:3: fail dup: duplicates b.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestCleanScanExitsZero(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + sumFunc("Sum") + "\n" + joinFunc("Join"),
	})

	code, stdout, stderr := runDup(t, dir)

	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestCopiedMethodIsReported(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\ntype A struct{}\n\n" + sumFunc("(a A) Sum"),
		"b.go": "package a\n\ntype B struct{}\n\n" + sumFunc("(b *B) Total"),
	})

	code, stdout, _ := runDup(t, dir)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if want := "a.go:5: fail dup: duplicates b.go:5 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestEachPairIsReportedOnce(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + sumFunc("One"),
		"b.go": "package b\n\n" + sumFunc("Two"),
		"c.go": "package c\n\n" + sumFunc("Three"),
	})

	_, stdout, _ := runDup(t, dir)

	want := "a.go:3: fail dup: duplicates b.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n" +
		"a.go:3: fail dup: duplicates c.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n" +
		"b.go:3: fail dup: duplicates c.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestOverlappingPathsDoNotDoubleAFunction(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"pkg/a.go": "package a\n\n" + sumFunc("Sum"),
		"pkg/b.go": "package a\n\n" + joinFunc("Join"),
	})

	write(t, dir, ".goalierc", `{"paths": [".", "pkg", "pkg/a.go"]}`)

	code, stdout, _ := runDup(t, dir)

	if code != 0 || stdout != "" {
		t.Errorf("exit code = %d, stdout = %q", code, stdout)
	}
}

func TestScanWorksWithoutGit(t *testing.T) {
	t.Setenv("PATH", "")
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + sumFunc("One"),
		"b.go": "package a\n\n" + sumFunc("Two"),
	})

	code, _, stderr := runDup(t, dir)

	if code != 1 {
		t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr)
	}
}

func TestPathsLimitTheScan(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"one/a.go":   "package one\n\n" + sumFunc("A"),
		"one/b.go":   "package one\n\n" + sumFunc("B"),
		"two/c.go":   "package two\n\n" + sumFunc("C"),
		"three/d.go": "package three\n\n" + sumFunc("D"),
	})

	tests := []struct {
		name  string
		paths string
		want  string
	}{
		{"one folder", `"one"`, "one/a.go:3: fail dup: duplicates one/b.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"},
		{"two files", `"two/c.go", "three/d.go"`, "three/d.go:3: fail dup: duplicates two/c.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"},
		{"absolute path", strconv.Quote(filepath.Join(dir, "one")), "one/a.go:3: fail dup: duplicates one/b.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			write(t, dir, ".goalierc", `{"paths": [`+test.paths+`]}`)
			_, stdout, _ := runDup(t, dir)
			if stdout != test.want {
				t.Errorf("stdout = %q, want %q", stdout, test.want)
			}
		})
	}
}

func TestMissingPathIsAnError(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.go": "package a\n", ".goalierc": `{"paths": ["missing"]}`})

	code, stdout, stderr := runDup(t, dir)

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if stdout != "" || !strings.Contains(stderr, "missing") {
		t.Errorf("stdout = %q, stderr = %q", stdout, stderr)
	}
}

func TestParseErrorIsAnError(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go":       "package a\n\n" + sumFunc("One"),
		"b.go":       "package a\n\n" + sumFunc("Two"),
		"pkg/bad.go": "package a\n\nfunc broken( {\n",
	})

	code, stdout, stderr := runDup(t, dir)

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	if !strings.Contains(stderr, "pkg/bad.go:3") {
		t.Errorf("stderr = %q, want the file and line", stderr)
	}
}

func TestNoFilesPrintsAMessageAndExitsZero(t *testing.T) {
	dir := writeTree(t, map[string]string{"README.md": "# Not Go\n"})

	code, stdout, stderr := runDup(t, dir)

	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	if stdout != "" || stderr != "goalie: dup: No Go files found.\n" {
		t.Errorf("stdout = %q, stderr = %q", stdout, stderr)
	}
}

func TestScanWritesNoFiles(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + sumFunc("One"),
		"b.go": "package a\n\n" + sumFunc("Two"),
	})
	before := listTree(t, dir)

	runDup(t, dir)

	if after := listTree(t, dir); !reflect.DeepEqual(before, after) {
		t.Errorf("files changed: before %v, after %v", before, after)
	}
}
