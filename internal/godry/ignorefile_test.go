package godry_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ignoreTree(t *testing.T, path, ignore string) string {
	t.Helper()
	dir := copyTree(t, path, "")
	write(t, dir, ".godryignore", ignore)
	return dir
}

func TestIgnoreFileSkipsMatchingFiles(t *testing.T) {
	tests := []struct {
		name, path, ignore string
	}{
		{"file name in any folder", "internal/users/packages.go", "packages.go\n"},
		{"glob on the file name", "internal/users/wire_users.go", "wire_*.go\n"},
		{"path from the run directory", "internal/users/packages.go", "internal/users/packages.go\n"},
		{"after comments and blank lines", "packages.go", "# Wiring repeats on purpose.\n\n  # Indented.\npackages.go\n"},
		{"no final newline", "packages.go", "packages.go"},
		{"Windows line endings", "packages.go", "# Wiring.\r\npackages.go\r\n"},
		{"spaces around a pattern", "packages.go", "  packages.go  \n"},
		{"everything below a folder", "internal/generated/a/copy.go", "internal/generated/**\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertSkipped(t, ignoreTree(t, test.path, test.ignore))
		})
	}
}

func TestIgnoreFileLeavesOtherFilesScanned(t *testing.T) {
	assertScanned(t, ignoreTree(t, "copy.go", "packages.go\n# copy.go\n"), "copy.go")
}

func TestAMissingOrEmptyIgnoreFileIsNotAnError(t *testing.T) {
	assertScanned(t, copyTree(t, "copy.go", ""), "copy.go")
	assertScanned(t, ignoreTree(t, "copy.go", ""), "copy.go")
}

func TestIgnoreFileAndSkipFilesBothApply(t *testing.T) {
	dir := writeTree(t, map[string]string{
		".godryignore": "one.go\n",
		"one.go":       "package a\n\n" + sumFunc("One"),
		"two.go":       "package a\n\n" + sumFunc("Two"),
		"kept.go":      "package a\n\n" + sumFunc("Kept"),
	})
	assertSkipped(t, dir, "--skip-files", "two.go")
}

func TestIgnoreFileIsReadOnlyFromTheRunDirectory(t *testing.T) {
	dir := writeTree(t, map[string]string{
		".godryignore": "copy.go\n",
		"pkg/kept.go":  "package a\n\n" + sumFunc("One"),
		"pkg/copy.go":  "package a\n\n" + sumFunc("Two"),
	})
	assertScanned(t, filepath.Join(dir, "pkg"), "copy.go")

	nested := copyTree(t, "pkg/copy.go", "")
	write(t, nested, "pkg/.godryignore", "copy.go\n")
	assertScanned(t, nested, "pkg/copy.go")
}

func TestIgnoreFileAppliesToNewCodeMode(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		".godryignore": "packages.go\n",
		"kept.go":      "package a\n\n" + sumFunc("One"),
	})
	write(t, dir, "packages.go", "package a\n\n"+sumFunc("Two"))
	wantReport(t, dir, 0, "No new functions found.\n", "--base", "HEAD")
}

func TestABadIgnoreFilePatternNamesItsLine(t *testing.T) {
	dir := ignoreTree(t, "copy.go", "# Comment.\npackages.go\n[\n")
	code, stdout, stderr := runGodry(t, dir, "--warn-only")
	if code != 2 || stdout != "" || !strings.Contains(stderr, ".godryignore:3:") {
		t.Errorf("exit code = %d, stdout = %q, stderr = %q; want exit 2 at .godryignore:3", code, stdout, stderr)
	}
}

func TestUnsupportedPatternsAreErrors(t *testing.T) {
	for _, pattern := range []string{"!keep.go", "internal/generated/", "internal/a**b/packages.go", "**.go", "[a/b].go"} {
		t.Run(pattern, func(t *testing.T) {
			dir := ignoreTree(t, "copy.go", pattern+"\n")
			if code, _, stderr := runGodry(t, dir); code != 2 || !strings.Contains(stderr, ".godryignore:1:") {
				t.Errorf("exit code = %d, stderr = %q; want exit 2 at .godryignore:1", code, stderr)
			}
			flagDir := copyTree(t, "copy.go", "")
			if code, _, stderr := runGodry(t, flagDir, "--skip-files", pattern); code != 2 || !strings.Contains(stderr, pattern) {
				t.Errorf("exit code = %d, stderr = %q; want exit 2 naming the pattern", code, stderr)
			}
		})
	}
}

func TestAnUnreadableIgnoreFileIsAnError(t *testing.T) {
	dir := copyTree(t, "copy.go", "")
	if err := os.Mkdir(filepath.Join(dir, ".godryignore"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runGodry(t, dir); code != 2 || !strings.Contains(stderr, ".godryignore") {
		t.Errorf("exit code = %d, stderr = %q; want exit 2 naming the file", code, stderr)
	}
}

func TestHelpListsTheIgnoreFile(t *testing.T) {
	_, stdout, _ := runGodry(t, t.TempDir(), "--help")
	if !strings.Contains(stdout, ".godryignore") {
		t.Errorf("stdout = %q; want .godryignore listed", stdout)
	}
}

func TestATrailingSlashErrorSuggestsTheFolderPattern(t *testing.T) {
	dir := ignoreTree(t, "copy.go", "internal/generated/\n")
	if code, _, stderr := runGodry(t, dir); code != 2 || !strings.Contains(stderr, "internal/generated/**") {
		t.Errorf("exit code = %d, stderr = %q; want exit 2 suggesting internal/generated/**", code, stderr)
	}
}
