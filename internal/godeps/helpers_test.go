package godeps_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/n-valchev/goalie/internal/godeps"
)

const goMod = "module example.com/shop\n\ngo 1.25\n"

type class struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type model struct {
	Version int     `json:"version"`
	Title   string  `json:"title"`
	Module  string  `json:"module"`
	Classes []class `json:"classes"`
	Edges   []edge  `json:"edges"`
}

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
	code := godeps.Run(args, dir, &out, &errOut)
	if code != want {
		t.Errorf("exit code = %d, want %d; stdout = %q, stderr = %q", code, want, out.String(), errOut.String())
	}
	return out.String(), errOut.String()
}

func readModel(t *testing.T, dir string, args ...string) model {
	t.Helper()
	stdout, _ := run(t, dir, 0, append([]string{"--format", "json"}, args...)...)
	var result model
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not a model: %v\n%s", err, stdout)
	}
	return result
}
