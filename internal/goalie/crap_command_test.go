package goalie_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// writeHalf is a coverCommand that writes one profile with 50% coverage of Decide.
const writeHalf = `printf 'mode: set\nexample.com/m/a.go:4.2,4.10 1 1\nexample.com/m/a.go:7.2,7.10 1 0\n' > \"$GOALIE_COVER_DIR/unit.out\"`

func commandTree(t *testing.T, command string) string {
	t.Helper()
	return writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a.go":      decideFile,
		".goalierc": `{"rules": {"crap": {"limit": 10, "coverCommand": "` + command + `"}}}`,
	})
}

func TestTheCoverCommandCanWriteSeveralProfiles(t *testing.T) {
	second := `printf 'mode: set\nexample.com/m/a.go:7.2,7.10 1 1\n' > \"$GOALIE_COVER_DIR/e2e.out\"`
	stdout, _ := runCrap(t, commandTree(t, writeHalf+"; "+second), 0)
	wantStdout(t, stdout, "")
}

func TestTheCoverCommandRunsInTheRunDirectory(t *testing.T) {
	dir := commandTree(t, `pwd > ran-here; `+writeHalf)
	runCrap(t, dir, 1)
	if _, err := os.Stat(filepath.Join(dir, "ran-here")); err != nil {
		t.Error(err)
	}
}

func TestTheCoverCommandOutputStaysOffStdout(t *testing.T) {
	stdout, stderr := runCrap(t, commandTree(t, `echo running tests; `+writeHalf), 1)
	if strings.Contains(stdout, "running tests") || !strings.Contains(stderr, "running tests") {
		t.Errorf("stdout = %q, stderr = %q; want the command's output on stderr", stdout, stderr)
	}
}

func TestAFailedCoverCommandGivesNoScores(t *testing.T) {
	stdout, stderr := runCrap(t, commandTree(t, writeHalf+`; exit 3`), 2, "--warn-only")
	wantStdout(t, stdout, "")
	if !strings.Contains(stderr, "coverage command failed") {
		t.Errorf("stderr = %q; want the failure named", stderr)
	}
}

func TestACoverCommandThatWritesNoProfileIsAnError(t *testing.T) {
	_, stderr := runCrap(t, commandTree(t, `true`), 2)
	if !strings.Contains(stderr, "GOALIE_COVER_DIR") {
		t.Errorf("stderr = %q; want the folder named", stderr)
	}
}

func TestACoverCommandFolderHoldsOnlyProfilesAtItsTopLevel(t *testing.T) {
	stdout, _ := runCrap(t, commandTree(t, `mkdir \"$GOALIE_COVER_DIR/raw\"; touch \"$GOALIE_COVER_DIR/raw/covmeta\"; `+writeHalf), 1)
	wantStdout(t, stdout, "a.go:3: fail crap: Decide scores 10.5 (complexity 6, coverage 50.0%, limit 10)\n")
}

func TestConfigCoverProfilesAreRead(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a.go":      decideFile,
		"p.out":     halfCovered,
		".goalierc": `{"rules": {"crap": {"limit": 10, "coverProfiles": ["p.out"]}}}`,
	})
	runCrap(t, dir, 1)
}

func TestSettingBothCoverSourcesIsAnError(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a.go":      decideFile,
		"p.out":     halfCovered,
		".goalierc": `{"rules": {"crap": {"coverCommand": "true", "coverProfiles": ["p.out"]}}}`,
	})
	_, stderr := runCrap(t, dir, 2)
	if !strings.Contains(stderr, ".goalierc") {
		t.Errorf("stderr = %q; want the file named", stderr)
	}
}

func TestAnEmptyCoverCommandIsAnError(t *testing.T) {
	runCrap(t, commandTree(t, ``), 2)
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
	stdout, _ := runCrap(t, dir, 1, "--limit", "1")
	wantStdout(t, stdout, "other/b.go:3: fail crap: Untested scores 6.0 (complexity 2, coverage 0.0%, limit 1)\n"+
		"a.go:3: fail crap: Small scores 2.0 (complexity 2, coverage 100.0%, limit 1)\n")
}

func globTree(t *testing.T) string {
	return writeTree(t, map[string]string{
		"go.mod":         goMod,
		"a.go":           decideFile,
		"cov/unit.out":   "mode: set\nexample.com/m/a.go:4.2,4.10 1 1\nexample.com/m/a.go:7.2,7.10 1 0\n",
		"cov/e2e.out":    "mode: set\nexample.com/m/a.go:7.2,7.10 1 1\n",
		"cov/raw/x.out":  "not a profile",
		"other/more.out": halfCovered,
	})
}

func TestACoverProfilesGlobReadsEveryMatchAndSkipsFolders(t *testing.T) {
	stdout, _ := runCrapWith(t, globTree(t), 0, `"coverProfiles": ["cov/*"]`, "--limit", "6")
	wantStdout(t, stdout, "")
}

func TestAnAbsoluteCoverProfilesGlobWorks(t *testing.T) {
	dir := globTree(t)
	runCrapWith(t, dir, 0, `"coverProfiles": [`+strconv.Quote(filepath.Join(dir, "cov", "*.out"))+`]`, "--limit", "6")
}

func TestAFileMatchedTwiceMergesTheSameAsOnce(t *testing.T) {
	dir := globTree(t)
	stdout, _ := runCrapWith(t, dir, 1, `"coverProfiles": ["other/more.out", "other/*.out"]`, "--limit", "0")
	once, _ := runCrapWith(t, dir, 1, `"coverProfiles": ["other/more.out"]`, "--limit", "0")
	if stdout != once {
		t.Errorf("stdout = %q; want %q", stdout, once)
	}
}

func TestAGlobWithNoMatchIsAnErrorThatNamesThePattern(t *testing.T) {
	_, stderr := runCrapWith(t, globTree(t), 2, `"coverProfiles": ["cov/*.xyz"]`)
	if !strings.Contains(stderr, "cov/*.xyz") {
		t.Errorf("stderr = %q; want the pattern named", stderr)
	}
}

func TestAMalformedCoverProfilesGlobIsAnError(t *testing.T) {
	runCrapWith(t, globTree(t), 2, `"coverProfiles": ["cov/["]`)
}

func TestGlobCharactersInTheRunDirectoryAreNotAPattern(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a[b")
	for name, content := range map[string]string{"go.mod": goMod, "a.go": decideFile, "cov/p.out": halfCovered} {
		write(t, dir, name, content)
	}
	for _, entry := range []string{"cov/p.out", "cov/*.out"} {
		stdout, _ := runCrapWith(t, dir, 1, `"coverProfiles": ["`+entry+`"]`, "--limit", "10")
		wantStdout(t, stdout, "a.go:3: fail crap: Decide scores 10.5 (complexity 6, coverage 50.0%, limit 10)\n")
	}
}
