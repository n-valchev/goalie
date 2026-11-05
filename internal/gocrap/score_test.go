package gocrap_test

import "testing"

func TestAnUncoveredComplexFunctionFails(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a.go":   decideFile,
		"p.out":  "mode: set\nexample.com/m/a.go:4.2,4.10 1 0\n",
	})
	stdout, _ := run(t, dir, 1, "--cover-profiles", "p.out")
	wantStdout(t, stdout, "a.go:3: Decide scores 42.0 (complexity 6, coverage 0.0%, limit 30)\n")
}

func TestCoverageLowersTheScore(t *testing.T) {
	// 6² × (1 - 0.5)³ + 6 = 10.5.
	dir := writeTree(t, map[string]string{"go.mod": goMod, "a.go": decideFile, "p.out": halfCovered})
	stdout, _ := run(t, dir, 1, "--cover-profiles", "p.out", "--limit", "10")
	wantStdout(t, stdout, "a.go:3: Decide scores 10.5 (complexity 6, coverage 50.0%, limit 10)\n")
}

func TestAScoreAtTheLimitPasses(t *testing.T) {
	dir := writeTree(t, map[string]string{"go.mod": goMod, "a.go": decideFile, "p.out": halfCovered})
	stdout, _ := run(t, dir, 0, "--cover-profiles", "p.out", "--limit", "10.5")
	wantStdout(t, stdout, "No functions over the limit.\n")
}

func TestAFunctionInNoProfileCountsAsUntested(t *testing.T) {
	dir := writeTree(t, map[string]string{"go.mod": goMod, "a.go": decideFile, "p.out": "mode: set\n"})
	stdout, _ := run(t, dir, 1, "--cover-profiles", "p.out")
	wantStdout(t, stdout, "a.go:3: Decide scores 42.0 (complexity 6, coverage 0.0%, limit 30)\n")
}

func TestABlockCoveredInAnyProfileCounts(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a.go":   decideFile,
		// The modes differ, because suites can use different -covermode values.
		"unit.out": "mode: set\nexample.com/m/a.go:4.2,4.10 1 1\nexample.com/m/a.go:7.2,7.10 1 0\n",
		"e2e.out":  "mode: count\nexample.com/m/a.go:4.2,4.10 1 0\nexample.com/m/a.go:7.2,7.10 1 3\n",
	})
	stdout, _ := run(t, dir, 0, "--cover-profiles", "unit.out,e2e.out", "--limit", "6")
	wantStdout(t, stdout, "No functions over the limit.\n")
}

func TestStatementsWeightTheCoverage(t *testing.T) {
	// 3 of 4 statements: 6² × 0.25³ + 6 = 6.5625, which rounds to 6.6.
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a.go":   decideFile,
		"p.out":  "mode: set\nexample.com/m/a.go:4.2,4.10 3 1\nexample.com/m/a.go:7.2,7.10 1 0\n",
	})
	stdout, _ := run(t, dir, 1, "--cover-profiles", "p.out", "--limit", "6")
	wantStdout(t, stdout, "a.go:3: Decide scores 6.6 (complexity 6, coverage 75.0%, limit 6)\n")
}

func TestFindingsAreSortedWorstFirst(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a.go":   "package m\n\nfunc Small(a int) int {\n\tif a > 0 {\n\t\treturn 1\n\t}\n\treturn 0\n}\n",
		"b.go":   decideFile,
		"p.out":  "mode: set\n",
	})
	stdout, _ := run(t, dir, 1, "--cover-profiles", "p.out", "--limit", "0")
	wantStdout(t, stdout, "b.go:3: Decide scores 42.0 (complexity 6, coverage 0.0%, limit 0)\n"+
		"a.go:3: Small scores 6.0 (complexity 2, coverage 0.0%, limit 0)\n")
}

func TestAMethodNamesItsReceiver(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a.go":   "package m\n\ntype T struct{}\n\nfunc (t *T) Run() {}\n",
		"p.out":  "mode: set\n",
	})
	stdout, _ := run(t, dir, 1, "--cover-profiles", "p.out", "--limit", "0")
	wantStdout(t, stdout, "a.go:5: T.Run scores 2.0 (complexity 1, coverage 0.0%, limit 0)\n")
}

func TestComplexityCountsEachBranchKind(t *testing.T) {
	// 1 + for + range + case + comm clause + || + the if inside the closure = 7. Defaults add nothing.
	source := `package m

func Branches(c chan int, values []int) {
	for i := 0; i < 1; i++ {
	}
	for range values {
	}
	switch {
	case len(values) > 0 || c == nil:
	default:
	}
	select {
	case <-c:
	default:
	}
	_ = func() {
		if c != nil {
		}
	}
}
`
	dir := writeTree(t, map[string]string{"go.mod": goMod, "a.go": source, "p.out": "mode: set\n"})
	stdout, _ := run(t, dir, 1, "--cover-profiles", "p.out", "--limit", "0")
	wantStdout(t, stdout, "a.go:3: Branches scores 56.0 (complexity 7, coverage 0.0%, limit 0)\n")
}

func TestTestFilesAreNotScored(t *testing.T) {
	dir := writeTree(t, map[string]string{"go.mod": goMod, "a_test.go": decideFile, "p.out": "mode: set\n"})
	stdout, _ := run(t, dir, 0, "--cover-profiles", "p.out")
	wantStdout(t, stdout, "No Go files found.\n")
}

func TestASubfolderRunMapsProfilePathsFromTheModuleRoot(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"pkg/a.go":  decideFile,
		"pkg/p.out": "mode: set\nexample.com/m/pkg/a.go:4.2,4.10 1 1\n",
	})
	stdout, _ := run(t, dir+"/pkg", 0, "--cover-profiles", "p.out")
	wantStdout(t, stdout, "No functions over the limit.\n")
}

func TestProfileErrors(t *testing.T) {
	tests := []struct{ name, profile string }{
		{"no mode line", "example.com/m/a.go:4.2,4.10 1 1\n"},
		{"empty", ""},
		{"bad block", "mode: set\nexample.com/m/a.go:oops 1 1\n"},
		{"bad count", "mode: set\nexample.com/m/a.go:4.2,4.10 1 x\n"},
		{"file outside the module", "mode: set\nexample.com/other/a.go:4.2,4.10 1 1\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{"go.mod": goMod, "a.go": decideFile, "p.out": test.profile})
			stdout, _ := run(t, dir, 2, "--cover-profiles", "p.out", "--warn-only")
			wantStdout(t, stdout, "")
		})
	}
}

func TestAMissingProfileIsAnError(t *testing.T) {
	dir := writeTree(t, map[string]string{"go.mod": goMod, "a.go": decideFile})
	run(t, dir, 2, "--cover-profiles", "missing.out")
}

func TestAMissingGoModIsAnError(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.go": decideFile, "p.out": "mode: set\n"})
	run(t, dir, 2, "--cover-profiles", "p.out")
}
