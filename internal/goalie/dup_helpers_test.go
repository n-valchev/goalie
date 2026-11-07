package goalie_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// runRule checks the whole repo unless the test names a revision, so that a test needs no Git
// history unless it is about changes.
func runRule(t *testing.T, dir, rule string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	command := []string{"check", rule}
	if !slices.ContainsFunc(args, func(arg string) bool { return strings.HasPrefix(arg, "--base") }) {
		command = append(command, "--all")
	}
	return runGoalie(t, dir, append(command, args...)...)
}

func runDup(t *testing.T, dir string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	return runRule(t, dir, "dup", args...)
}

// runDupWith first writes settings of the dup rule, such as `"minLines": 1`, to .goalierc.
func runDupWith(t *testing.T, dir, rule string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	write(t, dir, ".goalierc", `{"rules": {"dup": {`+rule+`}}}`)
	return runDup(t, dir, args...)
}

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		write(t, dir, name, content)
	}
	return dir
}

// joinFunc clears the default limits but shares little structure with sumFunc.
func joinFunc(name string) string {
	return fmt.Sprintf(`func %s(parts []string) string {
	var builder strings.Builder
	for index, part := range parts {
		if index != 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(strings.ToUpper(part))
	}
	return builder.String()
}
`, name)
}

// linesFunc has exactly the given number of lines, and the same structure for a given count.
func linesFunc(name string, lines int) string {
	var body strings.Builder
	for i := 0; i < lines-3; i++ {
		fmt.Fprintf(&body, "\ttotal += compute(total, %d)\n", i)
	}
	return fmt.Sprintf("func %s(total int) int {\n%s\treturn total\n}\n", name, body.String())
}

func listTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		tree[path] = string(content)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}
