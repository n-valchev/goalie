package goalie_test

import (
	"fmt"
	"maps"
	"math"
	"math/rand"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A plain all-pairs comparison of this set takes minutes, so this budget fails without the
// speedups and still leaves slow build machines room.
const scanBudget = 30 * time.Second

var statementShapes = []string{
	"total += %s(total)",
	"label = %s(label)",
	"defer %s(label)",
	"values = append(values, %s(total))",
	"if total > 10 {\n\t\ttotal = %s(total)\n\t}",
	"if label == \"\" {\n\t\treturn %s(total)\n\t}",
	"for _, value := range values {\n\t\ttotal -= %s(value)\n\t}",
	"switch total {\n\tcase 0:\n\t\ttotal = %s(1)\n\tdefault:\n\t\ttotal *= 2\n\t}",
}

func randomStatement(rng *rand.Rand, callees int) string {
	shape := statementShapes[rng.Intn(len(statementShapes))]
	return fmt.Sprintf(shape, fmt.Sprintf("step%d", rng.Intn(callees)))
}

// generatedFunctions returns families of near-duplicates. Each member edits a few statements of
// its family's base, and the base length varies, so scores spread around the default threshold
// and fingerprint set sizes differ inside and between families.
func generatedFunctions(seed int64, families, perFamily, callees int) []string {
	rng := rand.New(rand.NewSource(seed))
	var functions []string
	for range families {
		base := make([]string, 8+rng.Intn(24))
		for i := range base {
			base[i] = randomStatement(rng, callees)
		}
		for range perFamily {
			body := slices.Clone(base)
			for range rng.Intn(4) {
				body[rng.Intn(len(body))] = randomStatement(rng, callees)
			}
			body = body[:len(body)-rng.Intn(3)]
			functions = append(functions, fmt.Sprintf(
				"func F%05d(values []int, label string) int {\n\ttotal := 0\n\t%s\n\treturn total\n}\n",
				len(functions), strings.Join(body, "\n\t")))
		}
	}
	return functions
}

// spreadOverFiles puts the functions into packages of many files, as a real repo would have them.
func spreadOverFiles(functions []string) map[string]string {
	const perFile = 50
	files := map[string]string{}
	for start := 0; start < len(functions); start += perFile {
		number := start / perFile
		name := fmt.Sprintf("pkg%02d/file%03d.go", number%20, number)
		end := min(start+perFile, len(functions))
		files[name] = fmt.Sprintf("package pkg%02d\n\n%s", number%20, strings.Join(functions[start:end], "\n"))
	}
	return files
}

func fileName(index int) string {
	return fmt.Sprintf("f%03d.go", index)
}

func oneFunctionFile(function string) string {
	return "package gen\n\n" + function
}

// allPairsFindings runs the dup rule on every pair alone. A threshold of 0 reports each pair, so the
// result is the finding and score of every pair, with no help from anything a scan of the whole
// set can use.
func allPairsFindings(t *testing.T, functions []string) []map[string]any {
	t.Helper()
	var findings []map[string]any
	for i := range functions {
		for j := i + 1; j < len(functions); j++ {
			dir := writeTree(t, map[string]string{
				fileName(i): oneFunctionFile(functions[i]),
				fileName(j): oneFunctionFile(functions[j]),
			})
			_, stdout, stderr := runDupWith(t, dir, `"minLines": 1, "minNodes": 1`, "--format", "json", "--limit", "0")
			report := decodeReport(t, stdout)
			if len(report.Findings) != 1 {
				t.Fatalf("pair %d, %d: %d findings, want 1; stderr: %s", i, j, len(report.Findings), stderr)
			}
			findings = append(findings, pairFields(report.Findings[0]))
		}
	}
	return findings
}

// pairFields leaves out the fields that every result has, because the limit in them changes with
// the threshold of each run.
func pairFields(finding map[string]any) map[string]any {
	result := maps.Clone(finding)
	for _, shared := range []string{"rule", "mode", "value", "limit"} {
		delete(result, shared)
	}
	return result
}

func scoreOf(finding map[string]any) float64 {
	return finding["score"].(float64)
}

func fileOf(finding map[string]any, side string) string {
	return finding[side].(map[string]any)["file"].(string)
}

// reportedAt filters the all-pairs findings to what a scan at threshold must print, in report order.
func reportedAt(findings []map[string]any, threshold float64) []map[string]any {
	expected := []map[string]any{}
	for _, finding := range findings {
		if scoreOf(finding) >= threshold {
			expected = append(expected, finding)
		}
	}
	sort.Slice(expected, func(i, j int) bool {
		a, b := expected[i], expected[j]
		if scoreOf(a) != scoreOf(b) {
			return scoreOf(a) > scoreOf(b)
		}
		if fileOf(a, "left") != fileOf(b, "left") {
			return fileOf(a, "left") < fileOf(b, "left")
		}
		return fileOf(a, "right") < fileOf(b, "right")
	})
	return expected
}

// thresholdsAround returns each score from 0.5 up and the nearest numbers on both sides of it,
// because a speedup that rounds differently from the score fails only at those exact values.
func thresholdsAround(findings []map[string]any) []float64 {
	thresholds := []float64{0, 1e-12, 0.25, 0.5, 0.82, 0.9, 1}
	seen := map[float64]bool{}
	for _, finding := range findings {
		score := scoreOf(finding)
		if score >= 0.5 && !seen[score] {
			seen[score] = true
			thresholds = append(thresholds, score, math.Nextafter(score, 0), math.Nextafter(score, 1))
		}
	}
	return thresholds
}

func TestScanEqualsAllPairsAtAndNearEveryScore(t *testing.T) {
	functions := generatedFunctions(1, 10, 6, 40)
	findings := allPairsFindings(t, functions)

	nearDefault := 0
	for _, finding := range findings {
		if score := scoreOf(finding); score >= 0.72 && score <= 0.92 {
			nearDefault++
		}
	}
	if nearDefault < 20 {
		t.Fatalf("%d pairs score near the default threshold, want at least 20", nearDefault)
	}

	files := map[string]string{}
	for i, function := range functions {
		files[fileName(i)] = oneFunctionFile(function)
	}
	wantEachThreshold(t, writeTree(t, files), findings)
}

func wantEachThreshold(t *testing.T, dir string, findings []map[string]any, args ...string) {
	t.Helper()
	for _, threshold := range thresholdsAround(findings) {
		text := strconv.FormatFloat(threshold, 'g', -1, 64)
		all := append([]string{"--format", "json", "--limit", text}, args...)
		_, stdout, stderr := runDupWith(t, dir, `"minLines": 1, "minNodes": 1`, all...)
		var got []map[string]any
		for _, finding := range decodeReport(t, stdout).Findings {
			got = append(got, pairFields(finding))
		}
		want := reportedAt(findings, threshold)
		if len(got) == 0 && len(want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("threshold %s: %d findings differ from %d all-pairs findings; stderr: %s", text, len(got), len(want), stderr)
		}
	}
}

func withSide(side any, function string, isNew bool) map[string]any {
	result := maps.Clone(side.(map[string]any))
	result["function"] = function
	result["new"] = isNew
	return result
}

// The all-pairs findings come from functions with distinct names, because the same-name rule would
// hide a pair in them. The repo then gets the shared names, so this also proves the rule holds in
// new-code mode.
func TestNewCodeEqualsAllPairsWithSameNamePairsLeftOut(t *testing.T) {
	functions := generatedFunctions(6, 8, 5, 40)
	findings := allPairsFindings(t, functions)

	committed := func(i int) bool { return i%3 == 0 }
	untracked := func(i int) bool { return i%3 == 1 }
	isNew := func(i int) bool { return committed(i) || untracked(i) }
	// Neighbors come from one family, so most of these same-name pairs would otherwise be reported.
	nameOf := func(i int) string { return fmt.Sprintf("F%05d", i-i%2) }
	source := func(i int) string {
		return oneFunctionFile(strings.Replace(functions[i], fmt.Sprintf("F%05d", i), nameOf(i), 1))
	}

	indexOf := map[string]int{}
	old := map[string]string{}
	for i := range functions {
		indexOf[fileName(i)] = i
		if !isNew(i) {
			old[fileName(i)] = source(i)
		}
	}
	dir := gitRepo(t, old)
	git(t, dir, "checkout", "-b", "feature")
	for i := range functions {
		if committed(i) {
			write(t, dir, fileName(i), source(i))
		}
	}
	commitAll(t, dir, "new functions")
	for i := range functions {
		if untracked(i) {
			write(t, dir, fileName(i), source(i))
		}
	}

	expected := []map[string]any{}
	hidden := 0
	for _, finding := range findings {
		a, b := indexOf[fileOf(finding, "left")], indexOf[fileOf(finding, "right")]
		if !isNew(a) && !isNew(b) {
			continue
		}
		if nameOf(a) == nameOf(b) {
			if scoreOf(finding) >= 0.82 {
				hidden++
			}
			continue
		}
		left := withSide(finding["left"], nameOf(a), isNew(a))
		right := withSide(finding["right"], nameOf(b), isNew(b))
		if isNew(b) && !isNew(a) {
			left, right = right, left
		}
		expected = append(expected, map[string]any{
			"score": finding["score"], "literals": finding["literals"], "anchors": finding["anchors"],
			"left": left, "right": right,
		})
	}
	if hidden < 5 {
		t.Fatalf("%d same-name pairs score above the default threshold, want at least 5", hidden)
	}

	wantEachThreshold(t, dir, expected, "--base", "main")
}

func TestTenThousandFunctionsScanWithinBudget(t *testing.T) {
	functions := generatedFunctions(2, 2000, 5, 2000)
	dir := writeTree(t, spreadOverFiles(functions))

	start := time.Now()
	code, stdout, stderr := runDup(t, dir, "--format", "json")
	elapsed := time.Since(start)

	t.Logf("%d functions, %d findings, %v", len(functions), len(decodeReport(t, stdout).Findings), elapsed)
	if code != 1 {
		t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr)
	}
	if elapsed > scanBudget {
		t.Errorf("scan took %v, want at most %v", elapsed, scanBudget)
	}
}

func TestRepeatedScansPrintTheSameBytes(t *testing.T) {
	dir := writeTree(t, spreadOverFiles(generatedFunctions(3, 100, 5, 200)))

	_, first, _ := runDup(t, dir)
	for range 5 {
		if _, again, _ := runDup(t, dir); again != first {
			t.Fatal("two scans of the same files printed different output")
		}
	}
}

func TestParseErrorNamesTheFirstBadFileEveryTime(t *testing.T) {
	files := spreadOverFiles(generatedFunctions(4, 100, 5, 200))
	files["pkg00/file000.go"] = "package pkg00\n\nfunc broken( {\n"
	files["pkg19/file099.go"] = "package pkg19\n\nfunc broken( {\n"
	dir := writeTree(t, files)

	for range 20 {
		code, _, stderr := runDup(t, dir)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		if !strings.Contains(stderr, "pkg00/file000.go") || strings.Contains(stderr, "pkg19/file099.go") {
			t.Fatalf("stderr = %q, want only the first bad file", stderr)
		}
	}
}

func TestOldPairsInALargeRepoAreLeftOut(t *testing.T) {
	functions := generatedFunctions(7, 2000, 5, 2000)
	dir := gitRepo(t, spreadOverFiles(functions))
	copied := strings.Replace(functions[42], "F00042", "Copied", 1)
	write(t, dir, "a_new.go", "package copied\n\n"+copied)

	_, stdout, _ := runDup(t, dir, "--format", "json")
	var want []string
	oldPairs := 0
	for _, finding := range decodeReport(t, stdout).Findings {
		if fileOf(finding, "left") == "a_new.go" {
			want = append(want, fileOf(finding, "right"))
		} else {
			oldPairs++
		}
	}
	if len(want) == 0 || oldPairs < 100 {
		t.Fatalf("%d pairs with the new function and %d old pairs, want at least 1 and 100", len(want), oldPairs)
	}

	code, stdout, stderr := runDup(t, dir, "--base", "HEAD", "--format", "json")

	if code != 1 {
		t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr)
	}
	var got []string
	for _, finding := range decodeReport(t, stdout).Findings {
		if fileOf(finding, "left") != "a_new.go" {
			t.Errorf("finding %v doesn't lead with the new function", finding)
		}
		got = append(got, fileOf(finding, "right"))
	}
	if !slices.Equal(got, want) {
		t.Errorf("--base reported %v, want %v", got, want)
	}
}

func TestOneNewFunctionInTenThousandChecksWithinBudget(t *testing.T) {
	functions := generatedFunctions(5, 10000, 1, 2000)
	dir := gitRepo(t, spreadOverFiles(functions))
	copied := strings.Replace(functions[42], "F00042", "Copied", 1)
	write(t, dir, "a_new.go", "package fresh\n\n"+copied)

	start := time.Now()
	code, stdout, stderr := runDup(t, dir, "--base", "HEAD", "--format", "json")
	elapsed := time.Since(start)

	t.Logf("%v", elapsed)
	if code != 1 {
		t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr)
	}
	findings := decodeReport(t, stdout).Findings
	if len(findings) != 1 || fileOf(findings[0], "left") != "a_new.go" {
		t.Errorf("findings = %v, want one pair led by a_new.go", findings)
	}
	if elapsed > scanBudget {
		t.Errorf("check took %v, want at most %v", elapsed, scanBudget)
	}
}
