package cli_test

import (
	"strings"
	"testing"
)

func TestSkipFilesSkipsMatchingFiles(t *testing.T) {
	tests := []struct {
		name, path, patterns string
	}{
		{"file name in the root", "packages.go", "packages.go"},
		{"file name in any folder", "internal/users/packages.go", "packages.go"},
		{"glob on the file name", "internal/users/wire_users.go", "wire_*.go"},
		{"path from the run directory", "internal/users/packages.go", "internal/users/packages.go"},
		{"glob inside a path", "internal/users/packages.go", "internal/*/packages.go"},
		{"path written with a leading dot", "internal/users/packages.go", "./internal/users/packages.go"},
		{"second pattern in a list", "packages.go", "other.go,packages.go"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertSkipped(t, copyTree(t, test.path, ""), "--skip-files", test.patterns)
		})
	}
}

func TestSkipFilesLeavesOtherFilesScanned(t *testing.T) {
	tests := []struct {
		name, path, patterns string
	}{
		{"longer file name", "mypackages.go", "packages.go"},
		{"path from another folder", "other/packages.go", "internal/packages.go"},
		{"star does not cross folders", "internal/a/b/packages.go", "internal/*/packages.go"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertScanned(t, copyTree(t, test.path, ""), test.path, "--skip-files", test.patterns)
		})
	}
}

func TestSkipFilesCanBeRepeated(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"one.go":  "package a\n\n" + sumFunc("One"),
		"two.go":  "package a\n\n" + sumFunc("Two"),
		"kept.go": "package a\n\n" + sumFunc("Kept"),
	})
	assertSkipped(t, dir, "--skip-files", "one.go", "--skip-files", "two.go")
}

func TestSkipFilesAppliesToAPathYouName(t *testing.T) {
	dir := copyTree(t, "packages.go", "")
	assertSkipped(t, dir, "--skip-files", "packages.go", "kept.go", "packages.go")
}

func TestASkippedFileIsNotParsed(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"kept.go":     "package a\n\n" + sumFunc("One"),
		"packages.go": "this is not Go",
	})
	assertSkipped(t, dir, "--skip-files", "packages.go")
}

func TestABadSkipFilesPatternIsAnError(t *testing.T) {
	dir := copyTree(t, "packages.go", "")
	code, stdout, stderr := runGodry(t, dir, "--warn-only", "--skip-files", "[")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "[") {
		t.Errorf("exit code = %d, stdout = %q, stderr = %q; want exit 2 naming the pattern", code, stdout, stderr)
	}
}

func TestSkipFilesAppliesToNewCodeMode(t *testing.T) {
	dir := gitRepo(t, map[string]string{"kept.go": "package a\n\n" + sumFunc("One")})
	write(t, dir, "packages.go", "package a\n\n"+sumFunc("Two"))
	write(t, dir, "added.go", "package a\n\n"+sumFunc("Three"))

	code, stdout, stderr := runGodry(t, dir, "--base", "HEAD", "--skip-files", "packages.go")
	if code != 1 || strings.Contains(stdout, "packages.go") || !strings.Contains(stdout, "added.go:") {
		t.Errorf("exit code = %d, stdout = %q, stderr = %q; want only added.go reported", code, stdout, stderr)
	}
}

func TestHelpListsSkipFiles(t *testing.T) {
	_, stdout, _ := runGodry(t, t.TempDir(), "--help")
	if !strings.Contains(stdout, "--skip-files") {
		t.Errorf("stdout = %q; want --skip-files listed", stdout)
	}
}
