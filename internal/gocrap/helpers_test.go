package gocrap_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/n-valchev/goalie/internal/gocrap"
)

const goMod = "module example.com/m\n\ngo 1.25\n"

// decide is on lines 3 to 19 of a file that starts with a package clause and a blank line. Its
// complexity is 6: the function, three ifs, one &&, and one non-default case.
const decide = `func Decide(a, b int) int {
	if a > 0 {
		return 1
	}
	if b > 0 && a < 0 {
		return 2
	}
	if a == b {
		return 3
	}
	switch a {
	case 7:
		return 4
	default:
		return 5
	}
}
`

const decideFile = "package m\n\n" + decide

// halfCovered lists statements of Decide: 1 of 2 covered gives 50% coverage.
const halfCovered = "mode: set\nexample.com/m/a.go:4.2,4.10 1 1\nexample.com/m/a.go:7.2,7.10 1 0\n"

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		write(t, dir, name, content)
	}
	return dir
}

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

func run(t *testing.T, dir string, want int, args ...string) (stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := gocrap.Run(args, dir, &out, &errOut)
	if code != want {
		t.Errorf("exit code = %d, want %d; stdout = %q, stderr = %q", code, want, out.String(), errOut.String())
	}
	return out.String(), errOut.String()
}

func wantStdout(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}
