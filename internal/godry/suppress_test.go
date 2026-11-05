package godry_test

import (
	"fmt"
	"strings"
	"testing"
)

const ignoreLine = "//godry:ignore Kept apart on purpose.\n"

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

			code, stdout, stderr := runGodry(t, dir)

			if code != 0 {
				t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
			}
			if stdout != "No duplicates found.\n" {
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

	code, stdout, _ := runGodry(t, dir)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if want := "a.go:3: duplicates c.go:3 (score 1.00, literals 1.00, anchors 0)\n"; stdout != want {
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
		{"space after the slashes", "// godry:ignore Kept apart.\n", 4},
		{"longer directive name", "//godry:ignored Kept apart.\n", 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{
				"a.go": "package a\n\n" + test.comment + sumFunc("One"),
				"b.go": "package a\n\n" + sumFunc("Two"),
			})

			code, stdout, stderr := runGodry(t, dir)

			if code != 1 {
				t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr)
			}
			want := fmt.Sprintf("a.go:%d: duplicates b.go:3 (score 1.00, literals 1.00, anchors 0)\n", test.line)
			if stdout != want {
				t.Errorf("stdout = %q, want %q", stdout, want)
			}
		})
	}
}

func TestMissingReasonNamesTheFileOnDiskNotALineDirective(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n//line template.y:100\n//godry:ignore\n" + sumFunc("One"),
	})

	code, _, stderr := runGodry(t, dir)

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.HasPrefix(stderr, "godry: a.go:4: ") {
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
		{"no reason", "//godry:ignore\n" + sumFunc("One"), nil, "a.go:3:"},
		{"only spaces", "//godry:ignore   \n" + sumFunc("One"), nil, "a.go:3:"},
		{"under --warn-only", "//godry:ignore\n" + sumFunc("One"), []string{"--warn-only"}, "a.go:3:"},
		{"inside a doc comment", "// One adds.\n//godry:ignore\n" + sumFunc("One"), nil, "a.go:4:"},
		{"after a valid directive", ignoreLine + "//godry:ignore\n" + sumFunc("One"), nil, "a.go:4:"},
		{"function below the limits", "//godry:ignore\nfunc Short() {}\n", nil, "a.go:3:"},
		{"function with no body", "//godry:ignore\nfunc Asm() int\n", nil, "a.go:3:"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{
				"a.go": "package a\n\n" + test.source,
				"b.go": "package a\n\n" + sumFunc("Two"),
				"c.go": "package a\n\n" + sumFunc("Three"),
			})

			code, stdout, stderr := runGodry(t, dir, test.args...)

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
