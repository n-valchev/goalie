package goalie_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func pairTree(t *testing.T, left, right string) string {
	t.Helper()
	return writeTree(t, map[string]string{
		left:  "package a\n\n" + sumFunc("One"),
		right: "package b\n\n" + sumFunc("Two"),
	})
}

// sharedPairTree also names shared code, as the body of a JSON list: `"pkg/**", "lib/**"`.
func sharedPairTree(t *testing.T, left, right, shared string) string {
	t.Helper()
	dir := pairTree(t, left, right)
	write(t, dir, ".goalierc", `{"shared": [`+shared+`]}`)
	return dir
}

func TestSharedNamesTheCaseOfEachFinding(t *testing.T) {
	tests := []struct{ name, left, right, shared, want string }{
		{"a product copies a shared package", "pkg/a.go", "billing/b.go", `"pkg/**"`, "copies-shared"},
		{"the side does not matter", "billing/b.go", "zlib/a.go", `"zlib/**"`, "copies-shared"},
		{"a shared folder inside a product", "billing/shared/a.go", "search/b.go", `"**/shared/**"`, "copies-shared"},
		{"two products", "billing/b.go", "search/c.go", `"pkg/**"`, "across-products"},
		{"one product", "billing/api/b.go", "billing/db/c.go", `"pkg/**"`, "same-product"},
		{"both shared", "pkg/a/a.go", "lib/b.go", `"pkg/**", "lib/**"`, "within-shared"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, _ := wantCode(t, sharedPairTree(t, test.left, test.right, test.shared), 1)
			if !strings.Contains(stdout, "anchors 0, case "+test.want+")") {
				t.Errorf("stdout = %q; want case %s", stdout, test.want)
			}
		})
	}
}

func TestAFileInNoProductHasNoCase(t *testing.T) {
	tests := []struct{ name, left, right string }{
		{"a root file and a product", "main.go", "billing/b.go"},
		{"a root file and a shared package", "main.go", "pkg/a.go"},
		{"two root files", "main.go", "other.go"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, _ := wantCode(t, sharedPairTree(t, test.left, test.right, `"pkg/**"`), 1)
			if strings.Contains(stdout, "case") {
				t.Errorf("stdout = %q; want no case", stdout)
			}
		})
	}
}

func TestAFileOutsideTheRunDirectoryIsInNoProduct(t *testing.T) {
	dir := pairTree(t, "x/a.go", "y/b.go")
	write(t, dir, "run/.goalierc", `{"shared": ["pkg/**"], "paths": ["../x", "../y"]}`)
	stdout, _ := wantCode(t, filepath.Join(dir, "run"), 1)
	if strings.Contains(stdout, "case") {
		t.Errorf("stdout = %q; want no case", stdout)
	}
}

func TestWithoutSharedTheOutputHasNoCase(t *testing.T) {
	dir := pairTree(t, "pkg/a.go", "billing/b.go")
	stdout, _ := wantCode(t, dir, 1)
	if want := "billing/b.go:3: fail dup: duplicates pkg/a.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	stdout, _ = wantCode(t, dir, 1, "--format", "json")
	if strings.Contains(stdout, `"case"`) {
		t.Errorf("stdout = %q; want no case field", stdout)
	}
}

func TestSharedAddsTheCaseToJSON(t *testing.T) {
	stdout, _ := wantCode(t, sharedPairTree(t, "pkg/a.go", "billing/b.go", `"pkg/**"`), 1, "--format", "json")
	var report struct {
		Findings []struct {
			Case string `json:"case"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Findings) != 1 || report.Findings[0].Case != "copies-shared" {
		t.Errorf("stdout = %q; want one copies-shared finding", stdout)
	}
}

func TestSharedDoesNotChangeWhichPairsAreReported(t *testing.T) {
	wantCode(t, sharedPairTree(t, "pkg/a.go", "billing/b.go", `"pkg/**"`), 1)
	dir := writeTree(t, map[string]string{
		"pkg/a.go":  "package a\n\n" + sumFunc("One"),
		".goalierc": `{"shared": ["pkg/**"]}`,
	})
	wantReport(t, dir, 0, "")
}

func TestSharedAppliesToNewCodeMode(t *testing.T) {
	dir := gitRepo(t, map[string]string{"pkg/a.go": "package a\n\n" + sumFunc("One")})
	write(t, dir, "billing/b.go", "package b\n\n"+sumFunc("Two"))
	write(t, dir, ".goalierc", `{"shared": ["pkg/**"]}`)
	stdout, _ := wantCode(t, dir, 1, "--base", "HEAD")
	if !strings.Contains(stdout, "billing/b.go:3: fail dup: duplicates pkg/a.go:3") || !strings.Contains(stdout, "case copies-shared") {
		t.Errorf("stdout = %q; want the new function first, with its case", stdout)
	}
}

func TestABadSharedPatternIsAnError(t *testing.T) {
	for _, pattern := range []string{"!pkg", "pkg/"} {
		t.Run(pattern, func(t *testing.T) {
			_, stderr := wantCode(t, sharedPairTree(t, "pkg/a.go", "billing/b.go", `"`+pattern+`"`), 2, "--warn-only")
			if !strings.Contains(stderr, ".goalierc") || !strings.Contains(stderr, pattern) {
				t.Errorf("stderr = %q; want the file and the pattern named", stderr)
			}
		})
	}
}

func TestASharedPatternThatMatchesNoFileIsReported(t *testing.T) {
	for _, pattern := range []string{"pkg", "pgk/**"} {
		t.Run(pattern, func(t *testing.T) {
			_, stderr := wantCode(t, sharedPairTree(t, "pkg/a.go", "billing/b.go", `"`+pattern+`"`), 1)
			if !strings.Contains(stderr, `shared pattern "`+pattern+`" matches no file`) {
				t.Errorf("stderr = %q; want a warning that names the pattern", stderr)
			}
		})
	}
}

func TestAMatchingSharedPatternPrintsNoWarning(t *testing.T) {
	_, stderr := wantCode(t, sharedPairTree(t, "pkg/a.go", "billing/b.go", `"pkg/**"`), 1)
	if stderr != "" {
		t.Errorf("stderr = %q; want nothing", stderr)
	}
}

func TestHelpListsShared(t *testing.T) {
	stdout, _ := wantCode(t, t.TempDir(), 0, "--help")
	for _, want := range []string{"shared", "copies-shared", "across-products", "same-product", "within-shared"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q; want %s listed", stdout, want)
		}
	}
}
