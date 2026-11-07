package goalie_test

import (
	"fmt"
	"strings"
	"testing"
)

const ignoreLine = "//goalie:ignore dup Kept apart on purpose.\n"

func TestIgnoreCommentOnEitherSideRemovesThePair(t *testing.T) {
	tests := []struct {
		name          string
		first, second string
	}{
		{"first function", ignoreLine, ""},
		{"second function", "", ignoreLine},
		{"both functions", ignoreLine, ignoreLine},
		{"last line of a doc comment", "// One adds values.\n" + ignoreLine, ""},
		{"first line of a doc comment", ignoreLine + "// One adds values.\n", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{
				"a.go": "package a\n\n" + test.first + sumFunc("One"),
				"b.go": "package a\n\n" + test.second + sumFunc("Two"),
			})

			code, stdout, stderr := runDup(t, dir)

			if code != 0 {
				t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
			}
			if stdout != "" {
				t.Errorf("stdout = %q", stdout)
			}
		})
	}
}

func TestIgnoredFunctionLeavesOtherPairsInPlace(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + sumFunc("One"),
		"b.go": "package a\n\n" + ignoreLine + sumFunc("Two"),
		"c.go": "package a\n\n" + sumFunc("Three"),
	})

	code, stdout, _ := runDup(t, dir)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if want := "a.go:3: fail dup: duplicates c.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestCommentThatIsNotADirectiveAboveTheFunctionSuppressesNothing(t *testing.T) {
	tests := []struct {
		name    string
		comment string
		line    int
	}{
		{"blank line before the function", ignoreLine + "\n", 5},
		{"space after the slashes", "// goalie:ignore dup Kept apart.\n", 4},
		{"longer directive name", "//goalie:ignored dup Kept apart.\n", 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{
				"a.go": "package a\n\n" + test.comment + sumFunc("One"),
				"b.go": "package a\n\n" + sumFunc("Two"),
			})

			code, stdout, stderr := runDup(t, dir)

			if code != 1 {
				t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr)
			}
			want := fmt.Sprintf("a.go:%d: fail dup: duplicates b.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n", test.line)
			if stdout != want {
				t.Errorf("stdout = %q, want %q", stdout, want)
			}
		})
	}
}

func TestMissingReasonNamesTheFileOnDiskNotALineDirective(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n//line template.y:100\n//goalie:ignore dup\n" + sumFunc("One"),
	})

	code, _, stderr := runDup(t, dir)

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.HasPrefix(stderr, "goalie: dup: a.go:4: ") {
		t.Errorf("stderr = %q, want the location a.go:4", stderr)
	}
}

func TestIgnoreCommentWithoutAReasonIsAnError(t *testing.T) {
	tests := []struct {
		name   string
		source string
		args   []string
		want   string
	}{
		{"no reason", "//goalie:ignore dup\n" + sumFunc("One"), nil, "a.go:3:"},
		{"only spaces", "//goalie:ignore dup   \n" + sumFunc("One"), nil, "a.go:3:"},
		{"under --warn-only", "//goalie:ignore dup\n" + sumFunc("One"), []string{"--warn-only"}, "a.go:3:"},
		{"inside a doc comment", "// One adds.\n//goalie:ignore dup\n" + sumFunc("One"), nil, "a.go:4:"},
		{"after a valid directive", ignoreLine + "//goalie:ignore dup\n" + sumFunc("One"), nil, "a.go:4:"},
		{"function below the limits", "//goalie:ignore dup\nfunc Short() {}\n", nil, "a.go:3:"},
		{"function with no body", "//goalie:ignore dup\nfunc Asm() int\n", nil, "a.go:3:"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{
				"a.go": "package a\n\n" + test.source,
				"b.go": "package a\n\n" + sumFunc("Two"),
				"c.go": "package a\n\n" + sumFunc("Three"),
			})

			code, stdout, stderr := runDup(t, dir, test.args...)

			if code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want nothing", stdout)
			}
			if !strings.Contains(stderr, test.want) {
				t.Errorf("stderr = %q, want the location %s", stderr, test.want)
			}
		})
	}
}
