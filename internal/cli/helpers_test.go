package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/n-valchev/godry/internal/cli"
)

// writeTree creates a temporary directory holding files, keyed by slash-separated relative path.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runGodry(t *testing.T, dir string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = cli.Run(args, dir, &out, &errOut)
	return code, out.String(), errOut.String()
}

// sumFunc is a 12-line function that clears the default limits.
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
