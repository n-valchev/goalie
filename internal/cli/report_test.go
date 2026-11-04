package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
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
		"score": 1.0, "literals": 1.0, "anchors": 0.0,
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
	line := regexp.MustCompile(`^internal/a\.go:3: duplicates internal/b\.go:3 \(score 0\.\d\d, literals 0\.50, anchors 0\)\n$`)
	if !line.MatchString(stdout) {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestPathsAreRelativeToTheRunDirectory(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"pkg/sub/a.go": "package a\n\n" + sumFunc("One"),
		"pkg/sub/b.go": "package a\n\n" + sumFunc("Two"),
	})
	want := "pkg/sub/a.go:3: duplicates pkg/sub/b.go:3 (score 1.00, literals 1.00, anchors 0)\n"

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

	order := regexp.MustCompile(`^z1\.go:3: duplicates z2\.go:3 \(score 1\.00, literals 1\.00, anchors 3\)\na1\.go:3: duplicates a2\.go:3 \(score 0\.\d\d, literals 0\.50, anchors 0\)\n$`)
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

	want := "a.go:3: duplicates a.go:16 (score 1.00, literals 1.00, anchors 0)\n" +
		"a.go:3: duplicates b.go:3 (score 1.00, literals 1.00, anchors 0)\n" +
		"a.go:3: duplicates b.go:16 (score 1.00, literals 1.00, anchors 0)\n" +
		"a.go:16: duplicates b.go:3 (score 1.00, literals 1.00, anchors 0)\n" +
		"a.go:16: duplicates b.go:16 (score 1.00, literals 1.00, anchors 0)\n" +
		"b.go:3: duplicates b.go:16 (score 1.00, literals 1.00, anchors 0)\n"
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

// evidenceLine runs godry on two functions, with limits low enough for short examples.
func evidenceLine(t *testing.T, left, right string) string {
	t.Helper()
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + left,
		"b.go": "package a\n\n" + right,
	})
	code, stdout, stderr := runGodry(t, dir, "--min-lines", "1", "--min-nodes", "1", "--threshold", "0.1")
	if code != 1 {
		t.Fatalf("exit code = %d, stdout = %q, stderr = %q; want a finding", code, stdout, stderr)
	}
	return stdout
}

func wantEvidence(t *testing.T, left, right, want string) {
	t.Helper()
	if line := evidenceLine(t, left, right); !strings.Contains(line, want) {
		t.Errorf("line = %q, want it to contain %q", line, want)
	}
}

const operationName = `func operationName(operation inference.InferenceOperation) string {
	switch operation {
	case inference.OperationEmbedding:
		return "embeddings"
	case inference.OperationChat:
		return "chat"
	default:
		return "unknown"
	}
}
`

const outcome = `func outcome(value inference.InferenceOutcome) string {
	switch value {
	case inference.OutcomeError:
		return "error"
	case inference.OutcomeSuccess:
		return "success"
	default:
		return "unknown"
	}
}
`

const loadUser = `func loadUser(ctx context.Context, id string) (*User, error) {
	row := db.QueryRowContext(ctx, "SELECT * FROM users WHERE id = $1", id)
	var user User
	if err := row.Scan(&user.ID, &user.Name); err != nil {
		return nil, fmt.Errorf("load user: %w", err)
	}
	return &user, nil
}
`

const loadOrder = `func loadOrder(ctx context.Context, id string) (*Order, error) {
	row := db.QueryRowContext(ctx, "SELECT * FROM orders WHERE id = $1", id)
	var order Order
	if err := row.Scan(&order.ID, &order.Total); err != nil {
		return nil, fmt.Errorf("load order: %w", err)
	}
	return &order, nil
}
`

func TestAMappingPairShowsLowLiteralsAndNoAnchors(t *testing.T) {
	want := "a.go:3: duplicates b.go:3 (score 1.00, literals 0.20, anchors 0)\n"
	if line := evidenceLine(t, operationName, outcome); line != want {
		t.Errorf("line = %q, want %q", line, want)
	}
}

func TestACopyWithNewDataStaysReportedWithItsAnchors(t *testing.T) {
	want := "a.go:3: duplicates b.go:3 (score 1.00, literals 0.00, anchors 3)\n"
	if line := evidenceLine(t, loadUser, loadOrder); line != want {
		t.Errorf("line = %q, want %q", line, want)
	}
}

func TestLiteralScoreCases(t *testing.T) {
	wrap := func(name, body string) string {
		return "func " + name + "() {\n\tuse(" + body + ")\n\tuse(1 + 2)\n}\n"
	}
	tests := []struct {
		name, left, right, want string
	}{
		{"same literals", `"a", 7`, `"a", 7`, "literals 1.00"},
		{"each different literal counts once", `"a", "a", "b"`, `"a", "c", "c"`, "literals 0.50"},
		{"a raw string equals a quoted string", "`a`", `"a"`, "literals 1.00"},
		{"a string never equals a number", `"8080"`, `8080`, "literals 0.33"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wantEvidence(t, wrap("one", test.left), wrap("two", test.right), test.want)
		})
	}
}

func TestCommonLiteralsAreNotCounted(t *testing.T) {
	left := "func one() {\n\tuse(0, 1, \"\", nil, true, false)\n}\n"
	right := "func two() {\n\tuse(1, 0, ``, false, nil, true)\n}\n"
	wantEvidence(t, left, right, "literals -,")
}

func TestOneSideWithoutLiteralsScoresZero(t *testing.T) {
	left := "func one() {\n\tuse(0, 1)\n}\n"
	right := "func two() {\n\tuse(5, 1)\n}\n"
	wantEvidence(t, left, right, "literals 0.00")
}

func TestAnchorCountsSharedCalledNamesOnly(t *testing.T) {
	tests := []struct {
		name, left, right, want string
	}{
		{"a name called twice counts once", "shared()\n\tshared()", "shared()\n\tshared()", "anchors 1)"},
		{"a name on one side only does not count", "shared()\n\tmine()", "shared()\n\tyours()", "anchors 1)"},
		{"a method counts, not its receiver", "user.Save()", "order.Save()", "anchors 1)"},
		{"a package call counts by its last name", "fmt.Errorf(x)", "errors.Errorf(x)", "anchors 1)"},
		{"a generic call counts", "convert[int](x)", "convert[string](x)", "anchors 1)"},
		{"a call inside a function literal counts", "go func() { work() }()", "go func() { work() }()", "anchors 1)"},
		{"operators do not count", "x = a != b", "x = a != b", "anchors 0)"},
		{"a field read does not count", "x = user.Name", "x = user.Name", "anchors 0)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wrap := func(name, body string) string {
				return "func " + name + "() {\n\t" + body + "\n}\n"
			}
			wantEvidence(t, wrap("one", test.left), wrap("two", test.right), test.want)
		})
	}
}

func TestJSONHasLiteralsAndAnchors(t *testing.T) {
	type finding struct {
		Literals *float64 `json:"literals"`
		Anchors  *int     `json:"anchors"`
	}
	run := func(left, right string) (finding, string) {
		dir := writeTree(t, map[string]string{"a.go": "package a\n\n" + left, "b.go": "package a\n\n" + right})
		_, stdout, _ := runGodry(t, dir, "--format", "json", "--min-lines", "1", "--min-nodes", "1")
		var parsed struct {
			Findings []finding `json:"findings"`
		}
		if err := json.Unmarshal([]byte(stdout), &parsed); err != nil || len(parsed.Findings) != 1 {
			t.Fatalf("stdout = %q, err = %v; want one finding", stdout, err)
		}
		return parsed.Findings[0], stdout
	}

	found, _ := run(loadUser, loadOrder)
	if found.Literals == nil || *found.Literals != 0 || found.Anchors == nil || *found.Anchors != 3 {
		t.Errorf("finding = %+v; want literals 0 and anchors 3", found)
	}
	unknown, stdout := run("func one() {\n\tuse(0)\n}\n", "func two() {\n\tuse(1)\n}\n")
	if unknown.Literals != nil || !strings.Contains(stdout, `"literals": null`) {
		t.Errorf("stdout = %q; want literals to be null", stdout)
	}
}

func mappingTree(t *testing.T) string {
	t.Helper()
	return writeTree(t, map[string]string{
		"a.go": "package a\n\n" + operationName,
		"b.go": "package a\n\n" + outcome,
	})
}

func TestWeakFindingsStayByDefault(t *testing.T) {
	code, stdout, stderr := runGodry(t, mappingTree(t))
	if code != 1 || !strings.Contains(stdout, "literals 0.20, anchors 0") || stderr != "" {
		t.Errorf("exit code = %d, stdout = %q, stderr = %q; want the finding", code, stdout, stderr)
	}
}

func TestAWeakFindingIsDroppedAndCounted(t *testing.T) {
	code, stdout, stderr := runGodry(t, mappingTree(t), "--weak-anchors", "1")
	if code != 0 || stdout != "No duplicates found.\n" {
		t.Errorf("exit code = %d, stdout = %q; want a clean result", code, stdout)
	}
	if want := "godry: 1 weak finding dropped\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}

func TestDroppingKeepsStrongFindingsAndCountsTheRest(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + operationName,
		"b.go": "package a\n\n" + outcome,
		"c.go": "package a\n\n" + strings.ReplaceAll(outcome, "outcome", "third"),
		"d.go": "package a\n\n" + joinFunc("One"),
		"e.go": "package a\n\n" + joinFunc("Two"),
	})
	code, stdout, stderr := runGodry(t, dir, "--weak-anchors", "1", "--weak-literals", "0.3")
	want := "d.go:3: duplicates e.go:3 (score 1.00, literals 1.00, anchors 3)\n" +
		"b.go:3: duplicates c.go:3 (score 1.00, literals 1.00, anchors 0)\n"
	if code != 1 || !sameLines(stdout, want) {
		t.Errorf("exit code = %d, stdout = %q, want these lines: %q", code, stdout, want)
	}
	if want := "godry: 2 weak findings dropped\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}

func sameLines(a, b string) bool {
	left, right := strings.Split(a, "\n"), strings.Split(b, "\n")
	sort.Strings(left)
	sort.Strings(right)
	return reflect.DeepEqual(left, right)
}

func TestTheDropRuleNeedsBothNumbersToBeLow(t *testing.T) {
	sameMapping := strings.ReplaceAll(operationName, "operationName", "copied")
	noLiterals := func(name string) string {
		return "func " + name + "(a, b int) int {\n\ta = a + b\n\tb = a * b\n\treturn a - b\n}\n"
	}
	calls := func(name, first, second, third string) string {
		return "func " + name + "() {\n\t" + first + "(\"x" + name + "\")\n\t" + second + "(\"y" + name + "\")\n\t" + third + "(\"z" + name + "\")\n}\n"
	}
	literals := func(name, first, second string) string {
		return "func " + name + "() int {\n\tif a > " + first + " {\n\t\treturn " + second + "\n\t}\n\treturn a\n}\n"
	}
	tests := []struct {
		name, left, right string
		args              []string
		dropped           bool
	}{
		{"shared calls keep different data", loadUser, loadOrder, []string{"--weak-anchors", "1"}, false},
		{"same literals keep a pair with no calls", operationName, sameMapping, []string{"--weak-anchors", "1"}, false},
		{"no literals to compare keeps the pair", noLiterals("one"), noLiterals("two"), []string{"--weak-anchors", "1"}, false},
		{"anchors equal to the limit are not weak", calls("one", "f", "g", "h"), calls("two", "f", "g", "h"), []string{"--weak-anchors", "3"}, false},
		{"anchors below the limit are weak", calls("one", "f", "g", "h"), calls("two", "f", "g", "h"), []string{"--weak-anchors", "4"}, true},
		{"literals equal to the limit are not weak", literals("one", "7", "8"), literals("two", "7", "9"), []string{"--weak-anchors", "1", "--weak-literals", "0.3333333333333333"}, false},
		{"literals below the limit are weak", literals("one", "7", "8"), literals("two", "7", "9"), []string{"--weak-anchors", "1", "--weak-literals", "0.34"}, true},
		{"a low limit keeps the pair", operationName, outcome, []string{"--weak-anchors", "1", "--weak-literals", "0.2"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{"a.go": "package a\n\n" + test.left, "b.go": "package a\n\n" + test.right})
			args := append([]string{"--min-lines", "1", "--min-nodes", "1"}, test.args...)
			code, stdout, stderr := runGodry(t, dir, args...)
			if test.dropped && (code != 0 || stderr != "godry: 1 weak finding dropped\n") {
				t.Errorf("exit code = %d, stdout = %q, stderr = %q; want the finding dropped", code, stdout, stderr)
			}
			if !test.dropped && (code != 1 || stderr != "") {
				t.Errorf("exit code = %d, stdout = %q, stderr = %q; want the finding kept", code, stdout, stderr)
			}
		})
	}
}

func TestDroppingWorksWithJSON(t *testing.T) {
	code, stdout, stderr := runGodry(t, mappingTree(t), "--format", "json", "--weak-anchors", "1")
	if findings := decodeReport(t, stdout).Findings; code != 0 || len(findings) != 0 {
		t.Errorf("exit code = %d, findings = %v; want none", code, findings)
	}
	if want := "godry: 1 weak finding dropped\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}

func TestDroppingWorksInNewCodeMode(t *testing.T) {
	dir := gitRepo(t, map[string]string{"old.go": "package a\n\n" + operationName})
	write(t, dir, "new.go", "package a\n\n"+outcome)

	wantReport(t, dir, 1, "new.go:3: duplicates old.go:3 (score 1.00, literals 0.20, anchors 0)\n", "--base", "main")
	wantReport(t, dir, 0, "No duplicates found.\n", "--base", "main", "--weak-anchors", "1")
}

func TestInvalidWeakLimitsAreErrors(t *testing.T) {
	tests := [][]string{
		{"--weak-anchors", "-1"},
		{"--weak-anchors", "many"},
		{"--weak-literals", "-0.1"},
		{"--weak-literals", "1.1"},
		{"--weak-literals", "NaN"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			code, stdout, stderr := runGodry(t, mappingTree(t), append(args, "--warn-only")...)
			if code != 2 || stdout != "" || !strings.Contains(stderr, strings.TrimPrefix(args[0], "--")) {
				t.Errorf("exit code = %d, stdout = %q, stderr = %q; want exit 2 naming the flag", code, stdout, stderr)
			}
		})
	}
}

func TestHelpListsTheWeakLimits(t *testing.T) {
	_, stdout, _ := runGodry(t, t.TempDir(), "--help")
	for _, flag := range []string{"--weak-anchors", "--weak-literals"} {
		if !strings.Contains(stdout, flag) {
			t.Errorf("stdout = %q; want %s listed", stdout, flag)
		}
	}
}
