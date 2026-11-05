package gocrap_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeHalf is a coverCommand that writes one profile with 50% coverage of Decide.
const writeHalf = `printf 'mode: set\nexample.com/m/a.go:4.2,4.10 1 1\nexample.com/m/a.go:7.2,7.10 1 0\n' > \"$GOCRAP_COVER_DIR/unit.out\"`

func commandTree(t *testing.T, command string) string {
	t.Helper()
	return writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a.go":      decideFile,
		".goalierc": `{"gocrap": {"limit": 10, "coverCommand": "` + command + `"}}`,
	})
}

func TestTheCoverCommandSuppliesCoverage(t *testing.T) {
	stdout, _ := run(t, commandTree(t, writeHalf), 1)
	wantStdout(t, stdout, "a.go:3: Decide scores 10.5 (complexity 6, coverage 50.0%, limit 10)\n")
}

func TestTheCoverCommandCanWriteSeveralProfiles(t *testing.T) {
	second := `printf 'mode: set\nexample.com/m/a.go:7.2,7.10 1 1\n' > \"$GOCRAP_COVER_DIR/e2e.out\"`
	stdout, _ := run(t, commandTree(t, writeHalf+"; "+second), 0)
	wantStdout(t, stdout, "No functions over the limit.\n")
}

func TestTheCoverCommandRunsInTheRunDirectory(t *testing.T) {
	dir := commandTree(t, `pwd > ran-here; `+writeHalf)
	run(t, dir, 1)
	if _, err := os.Stat(filepath.Join(dir, "ran-here")); err != nil {
		t.Error(err)
	}
}

func TestTheCoverCommandOutputStaysOffStdout(t *testing.T) {
	stdout, stderr := run(t, commandTree(t, `echo running tests; `+writeHalf), 1)
	if strings.Contains(stdout, "running tests") || !strings.Contains(stderr, "running tests") {
		t.Errorf("stdout = %q, stderr = %q; want the command's output on stderr", stdout, stderr)
	}
}

func TestAFailedCoverCommandGivesNoScores(t *testing.T) {
	stdout, stderr := run(t, commandTree(t, writeHalf+`; exit 3`), 2, "--warn-only")
	wantStdout(t, stdout, "")
	if !strings.Contains(stderr, "coverage command failed") {
		t.Errorf("stderr = %q; want the failure named", stderr)
	}
}

func TestACoverCommandThatWritesNoProfileIsAnError(t *testing.T) {
	_, stderr := run(t, commandTree(t, `true`), 2)
	if !strings.Contains(stderr, "GOCRAP_COVER_DIR") {
		t.Errorf("stderr = %q; want the folder named", stderr)
	}
}

func TestACoverCommandFolderHoldsOnlyProfilesAtItsTopLevel(t *testing.T) {
	stdout, _ := run(t, commandTree(t, `mkdir \"$GOCRAP_COVER_DIR/raw\"; touch \"$GOCRAP_COVER_DIR/raw/covmeta\"; `+writeHalf), 1)
	wantStdout(t, stdout, "a.go:3: Decide scores 10.5 (complexity 6, coverage 50.0%, limit 10)\n")
}

func TestTheCoverProfilesFlagSkipsTheCommand(t *testing.T) {
	dir := commandTree(t, `exit 3`)
	write(t, dir, "p.out", halfCovered)
	run(t, dir, 1, "--cover-profiles", "p.out")
}

func TestConfigCoverProfilesAreRead(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a.go":      decideFile,
		"p.out":     halfCovered,
		".goalierc": `{"gocrap": {"limit": 10, "coverProfiles": ["p.out"]}}`,
	})
	run(t, dir, 1)
}

func TestSettingBothCoverSourcesIsAnError(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a.go":      decideFile,
		"p.out":     halfCovered,
		".goalierc": `{"gocrap": {"coverCommand": "true", "coverProfiles": ["p.out"]}}`,
	})
	_, stderr := run(t, dir, 2, "--cover-profiles", "p.out")
	if !strings.Contains(stderr, ".goalierc") {
		t.Errorf("stderr = %q; want the file named", stderr)
	}
}

func TestAnEmptyCoverCommandIsAnError(t *testing.T) {
	run(t, commandTree(t, ``), 2)
}

// The real toolchain, so that the default command and the import-path mapping are tested together.
func TestTheDefaultCommandMeasuresARealModule(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":     goMod,
		"a.go":       smallFile,
		"other/b.go": strings.Replace(smallFile, "Small", "Untested", 1),
		"a_test.go": `package m

import "testing"

func TestSmall(t *testing.T) {
	Small(1)
	Small(0)
}
`,
	})
	t.Setenv("GOFLAGS", "")
	stdout, _ := run(t, dir, 1, "--limit", "1")
	wantStdout(t, stdout, "other/b.go:3: Untested scores 6.0 (complexity 2, coverage 0.0%, limit 1)\n"+
		"a.go:3: Small scores 2.0 (complexity 2, coverage 100.0%, limit 1)\n")
}
