package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/n-valchev/godry/internal/cli"
)

type jsonReport struct {
	Findings []map[string]any `json:"findings"`
}

func decodeReport(t *testing.T, stdout string) jsonReport {
	t.Helper()
	var report jsonReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	return report
}

func TestCleanJSONScanHasAnEmptyFindingsArray(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.go": "package a\n\n" + sumFunc("Sum")})

	code, stdout, stderr := runGodry(t, dir, "--format", "json")

	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	assertEmptyFindings(t, stdout)
}

// A null findings value would decode to the same empty Go slice, so this compares raw JSON.
func assertEmptyFindings(t *testing.T, stdout string) {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if got := string(raw["findings"]); got != "[]" {
		t.Errorf("findings = %s, want []", got)
	}
}

func TestNoGoFilesInJSONKeepsStdoutParseable(t *testing.T) {
	code, stdout, stderr := runGodry(t, t.TempDir(), "--format", "json")

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	assertEmptyFindings(t, stdout)
	if stderr != "No Go files found.\n" {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestNoGoFilesInTextGoesToStdout(t *testing.T) {
	code, stdout, stderr := runGodry(t, t.TempDir())

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if stdout != "No Go files found.\n" {
		t.Errorf("stdout = %q", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want nothing", stderr)
	}
}

func TestJSONReportsEveryFieldOfAFinding(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"pkg/a.go": "package a\n\n" + sumFunc("One"),
		"pkg/b.go": "package a\n\nvar limit = 1\n\n" + sumFunc("Two"),
	})

	code, stdout, stderr := runGodry(t, dir, "--format", "json")

	if code != 1 {
		t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr)
	}
	findings := decodeReport(t, stdout).Findings
	if len(findings) != 1 {
		t.Fatalf("findings = %v, want one", findings)
	}
	// The node count belongs to normalization, so only its presence is pinned here.
	nodes := findings[0]["left"].(map[string]any)["nodes"]
	if count, ok := nodes.(float64); !ok || count <= 0 {
		t.Errorf("left nodes = %v, want a positive number", nodes)
	}
	want := map[string]any{
		"score": 1.0,
		"left": map[string]any{
			"file": "pkg/a.go", "function": "One", "startLine": 3.0, "endLine": 14.0,
			"nodes": nodes, "new": false,
		},
		"right": map[string]any{
			"file": "pkg/b.go", "function": "Two", "startLine": 5.0, "endLine": 16.0,
			"nodes": nodes, "new": false,
		},
	}
	if !reflect.DeepEqual(findings[0], want) {
		t.Errorf("finding = %v, want %v", findings[0], want)
	}
}

func TestTextLineHasTwoDecimalScore(t *testing.T) {
	edited := strings.Replace(sumFunc("Two"), "if total > 100 {", "if total > 100 && total != 200 {", 1)
	dir := writeTree(t, map[string]string{
		"internal/a.go": "package a\n\n" + sumFunc("One"),
		"internal/b.go": "package a\n\n" + edited,
	})

	_, stdout, _ := runGodry(t, dir, "--threshold", "0.5")

	// The exact score belongs to normalization. This pins the line's format.
	line := regexp.MustCompile(`^internal/a\.go:3: duplicates internal/b\.go:3 \(score 0\.\d\d\)\n$`)
	if !line.MatchString(stdout) {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestPathsAreRelativeToTheRunDirectory(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"pkg/sub/a.go": "package a\n\n" + sumFunc("One"),
		"pkg/sub/b.go": "package a\n\n" + sumFunc("Two"),
	})
	want := "pkg/sub/a.go:3: duplicates pkg/sub/b.go:3 (score 1.00)\n"

	for _, path := range []string{"pkg", "./pkg/sub/", filepath.Join(dir, "pkg")} {
		t.Run(path, func(t *testing.T) {
			_, stdout, stderr := runGodry(t, dir, path)
			if stdout != want {
				t.Errorf("stdout = %q, want %q; stderr: %s", stdout, want, stderr)
			}
		})
	}
}

func TestHigherScoreComesFirstWhateverThePaths(t *testing.T) {
	edited := strings.Replace(sumFunc("Near"), "if total > 100 {", "if total > 100 && total != 200 {", 1)
	dir := writeTree(t, map[string]string{
		"a1.go": "package a\n\n" + sumFunc("One"),
		"a2.go": "package a\n\n" + edited,
		"z1.go": "package a\n\n" + joinFunc("Two"),
		"z2.go": "package a\n\n" + joinFunc("Three"),
	})

	_, stdout, _ := runGodry(t, dir, "--threshold", "0.6")

	order := regexp.MustCompile(`^z1\.go:3: duplicates z2\.go:3 \(score 1\.00\)\na1\.go:3: duplicates a2\.go:3 \(score 0\.\d\d\)\n$`)
	if !order.MatchString(stdout) {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestEqualScoresSortByFirstThenSecondLocation(t *testing.T) {
	dir := writeTree(t, map[string]string{
		// Line 16 sorts before line 3 as text, so these lines prove a numeric order.
		"a.go": "package a\n\n" + sumFunc("One") + "\n" + sumFunc("Two"),
		"b.go": "package a\n\n" + sumFunc("Three") + "\n" + sumFunc("Four"),
	})

	_, stdout, _ := runGodry(t, dir)

	want := "a.go:3: duplicates a.go:16 (score 1.00)\n" +
		"a.go:3: duplicates b.go:3 (score 1.00)\n" +
		"a.go:3: duplicates b.go:16 (score 1.00)\n" +
		"a.go:16: duplicates b.go:3 (score 1.00)\n" +
		"a.go:16: duplicates b.go:16 (score 1.00)\n" +
		"b.go:3: duplicates b.go:16 (score 1.00)\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestRepeatedRunsGiveIdenticalOutput(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + sumFunc("One") + "\n" + sumFunc("Two"),
		"b.go": "package a\n\n" + sumFunc("Three") + "\n" + joinFunc("Four"),
		"c.go": "package a\n\n" + joinFunc("Five"),
	})

	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			_, first, _ := runGodry(t, dir, "--format", format)
			for range 10 {
				if _, again, _ := runGodry(t, dir, "--format", format); again != first {
					t.Fatalf("output changed:\n%s\nthen:\n%s", first, again)
				}
			}
		})
	}
}

func TestJSONFindingsUseTheSameOrderAsText(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + sumFunc("One") + "\n" + sumFunc("Two"),
		"b.go": "package a\n\n" + sumFunc("Three"),
	})

	_, stdout, _ := runGodry(t, dir, "--format", "json")

	var got []string
	for _, finding := range decodeReport(t, stdout).Findings {
		got = append(got, finding["left"].(map[string]any)["function"].(string)+"-"+
			finding["right"].(map[string]any)["function"].(string))
	}
	if want := []string{"One-Two", "One-Three", "Two-Three"}; !reflect.DeepEqual(got, want) {
		t.Errorf("pairs = %v, want %v", got, want)
	}
}

func TestWarnOnlyStillReportsFindingsInJSON(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + sumFunc("One"),
		"b.go": "package a\n\n" + sumFunc("Two"),
	})

	code, stdout, _ := runGodry(t, dir, "--format", "json", "--warn-only")

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if got := len(decodeReport(t, stdout).Findings); got != 1 {
		t.Errorf("findings = %d, want 1", got)
	}
}

func TestErrorUnderJSONLeavesStdoutEmpty(t *testing.T) {
	dir := writeTree(t, map[string]string{"bad.go": "package a\n\nfunc broken( {\n"})

	code, stdout, stderr := runGodry(t, dir, "--format", "json")

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	if stderr == "" {
		t.Error("stderr is empty")
	}
}

func TestInvalidFormatExitsTwoEvenWithFindings(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + sumFunc("One"),
		"b.go": "package a\n\n" + sumFunc("Two"),
	})

	for _, format := range []string{"xml", "JSON", ""} {
		t.Run(format, func(t *testing.T) {
			code, stdout, stderr := runGodry(t, dir, "--format", format)
			if code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want nothing", stdout)
			}
			if !strings.Contains(stderr, "--format") {
				t.Errorf("stderr = %q", stderr)
			}
		})
	}
}

func TestHelpListsModesFlagsSkipRulesAndExitCodes(t *testing.T) {
	code, stdout, stderr := runGodry(t, t.TempDir(), "--help")

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want nothing", stderr)
	}
	sections := map[string][]string{
		"modes": {"godry [paths]", "godry --base <rev> [paths]"},
		"flags": {
			"--base <rev>", "--threshold N", "default 0.82", "--min-lines N", "default 9",
			"--min-nodes N", "default 20", "--format F", "text or json", "--warn-only",
			"--include-tests", "--skip-tags a,b", "--help",
		},
		"skip rules": {
			"vendor", "testdata", "node_modules", "start with . or _", "*_test.go",
			"Code generated ... DO NOT EDIT.", "ignore build constraint", "same function name",
			"//godry:ignore <reason>",
		},
		"exit codes": {
			"0  No duplicates", "1  Duplicates exist", "2  Any error",
		},
	}
	for section, phrases := range sections {
		for _, phrase := range phrases {
			if !strings.Contains(stdout, phrase) {
				t.Errorf("%s: help does not contain %q", section, phrase)
			}
		}
	}
}

func TestHelpDoesNotScan(t *testing.T) {
	dir := writeTree(t, map[string]string{"bad.go": "package a\n\nfunc broken( {\n"})

	code, stdout, _ := runGodry(t, dir, "--help")

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("stdout = %q", stdout)
	}
}

// failingWriter stands in for a full disk or a closed pipe.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("disk full")
}

func TestFailedWriteOfTheReportExitsTwo(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + sumFunc("One"),
		"b.go": "package a\n\n" + sumFunc("Two"),
	})

	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			var stderr bytes.Buffer
			code := cli.Run([]string{"--format", format, "--warn-only"}, dir, failingWriter{}, &stderr)
			if code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			if !strings.Contains(stderr.String(), "disk full") {
				t.Errorf("stderr = %q", stderr.String())
			}
		})
	}
}
