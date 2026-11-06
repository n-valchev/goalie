package goalie_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// git hides the machine's own Git config, so that a developer's settings can't change what a test
// sets up. Run itself still sees the real config.
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Dev", "GIT_AUTHOR_EMAIL=dev@example.com",
		"GIT_COMMITTER_NAME=Dev", "GIT_COMMITTER_EMAIL=dev@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func gitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		write(t, dir, name, content)
	}
	git(t, dir, "init", "--initial-branch=main")
	git(t, dir, "add", "--all")
	git(t, dir, "commit", "--message", "first")
	return dir
}

// sumFunc clears the default limits of the dup rule.
func sumFunc(name string) string {
	return fmt.Sprintf(`func %s(values []int) int {
	total := 0
	for _, value := range values {
		if value > 0 {
			total += value
		}
	}
	if total > 100 {
		return 100
	}
	return total
}
`, name)
}
