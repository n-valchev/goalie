package godeps_test

import (
	"encoding/json"
	"maps"
	"strings"
	"testing"
)

func wantClassGrades(t *testing.T, got gradedModel, want map[string]string) {
	t.Helper()
	grades := map[string]string{}
	for _, class := range got.Classes {
		grades[class.Path] = class.Grade
	}
	if !maps.Equal(grades, want) {
		t.Errorf("class grades = %v, want %v", grades, want)
	}
}

func wantBoxGrades(t *testing.T, got gradedModel, want map[string]string) {
	t.Helper()
	grades := map[string]string{}
	for _, box := range got.Packages {
		grades[box.ID] = box.Grade
	}
	if !maps.Equal(grades, want) {
		t.Errorf("box grades = %v, want %v", grades, want)
	}
}

// The fixture is real output. To rebuild it, run this from testdata/gocrap:
//
//	gocrap --limit 0 --shared-limit 0 --warn-only --format json > crap.json
func TestARealGocrapReportGradesEveryOp(t *testing.T) {
	const dir = "testdata/gocrap"
	stdout, stderr := run(t, dir, 0, "--format", "json", "--crap-report", "crap.json")
	if stderr != "" {
		t.Errorf("stderr = %q, want nothing", stderr)
	}
	var got gradedModel
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"Invoice.Paid": "green",  // Score 1 against a limit of 4.
		"Sign":         "green",  // Score 2: exactly half the limit.
		"Band":         "yellow", // Score 3.
		"Size":         "yellow", // Score 4: exactly the limit.
		"Untested":     "red",    // Score 6.
		"Ignored":      "red",    // gocrap:ignore keeps it out of the report.
		"Plain":        "green",  // Score 1: exactly half the shared limit of 2.
		"OrDash":       "yellow", // Score 2: exactly the shared limit.
		"Bounded":      "red",    // Score 3, which `limit` alone would make yellow.
	} {
		if grade := got.op(t, name).Grade; grade != want {
			t.Errorf("%s = %q, want %q", name, grade, want)
		}
	}
	untested := got.op(t, "Untested")
	if untested.Crap == nil || *untested.Crap != 6 || *untested.Complexity != 2 || *untested.Coverage != 0 {
		t.Errorf("Untested = %+v, want crap 6, complexity 2, coverage 0", untested)
	}
	wantClassGrades(t, got, map[string]string{"billing": "red", "lib/text": "red"})
	wantBoxGrades(t, got, map[string]string{"billing": "red", ":shared": "red", ":shared/lib": "red", ":shared/lib/text": "red"})
}

func TestTheGradeComesFromTheScoreAndTheLimit(t *testing.T) {
	const shared = `{"shared": ["a/**"], "gocrap": {"limit": 100, "sharedLimit": 8}}`
	tests := []struct {
		name, config string
		score        float64
		want         string
	}{
		{"default limit, exactly half", `{}`, 15, "green"},
		{"default limit, just over half", `{}`, 15.01, "yellow"},
		{"default limit, exactly the limit", `{}`, 30, "yellow"},
		{"default limit, just over", `{}`, 30.01, "red"},
		{"set limit, exactly half", `{"gocrap": {"limit": 8}}`, 4, "green"},
		{"set limit, just over half", `{"gocrap": {"limit": 8}}`, 4.01, "yellow"},
		{"set limit, exactly the limit", `{"gocrap": {"limit": 8}}`, 8, "yellow"},
		{"set limit, just over", `{"gocrap": {"limit": 8}}`, 8.01, "red"},
		{"limit of 0, score of 0", `{"gocrap": {"limit": 0}}`, 0, "green"},
		{"limit of 0, any score", `{"gocrap": {"limit": 0}}`, 1, "red"},
		{"sharedLimit, exactly half", shared, 4, "green"},
		{"sharedLimit, just over half", shared, 4.01, "yellow"},
		{"sharedLimit, exactly the limit", shared, 8, "yellow"},
		{"sharedLimit, just over", shared, 8.01, "red"},
		{"sharedLimit of 0", `{"shared": ["a/**"], "gocrap": {"sharedLimit": 0}}`, 1, "red"},
		{"sharedLimit left out", `{"shared": ["a/**"], "gocrap": {"limit": 8}}`, 8.01, "red"},
		{"sharedLimit on other code", `{"shared": ["b/**"], "gocrap": {"limit": 100, "sharedLimit": 8}}`, 8.01, "green"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{
				"go.mod":    goMod,
				"a/a.go":    "package a\n\nfunc F() {}\n",
				"b/b.go":    "package b\n",
				".goalierc": test.config,
				"crap.json": crapReport(crapRow("a/a.go", 3, "F", test.score)),
			})
			if got := readGrades(t, dir, "--crap-report", "crap.json").op(t, "F").Grade; got != test.want {
				t.Errorf("grade = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSharedLimitAppliesPerFileNotPerClass(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a/one.go":  "package a\n\nfunc One() {}\n",
		"a/two.go":  "package a\n\nfunc Two() {}\n",
		".goalierc": `{"shared": ["a/one.go"], "gocrap": {"limit": 100, "sharedLimit": 8}}`,
		"crap.json": crapReport(crapRow("a/one.go", 3, "One", 9), crapRow("a/two.go", 3, "Two", 9)),
	})
	got := readGrades(t, dir, "--crap-report", "crap.json")
	if grade := got.op(t, "One").Grade; grade != "red" {
		t.Errorf("One = %q, want red", grade)
	}
	if grade := got.op(t, "Two").Grade; grade != "green" {
		t.Errorf("Two = %q, want green", grade)
	}
}

func TestTheSharedFlagChoosesWhichCodeTakesSharedLimit(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a/a.go":    "package a\n\nfunc F() {}\n",
		".goalierc": `{"gocrap": {"limit": 100, "sharedLimit": 8}}`,
		"crap.json": crapReport(crapRow("a/a.go", 3, "F", 9)),
	})
	if got := readGrades(t, dir, "--crap-report", "crap.json", "--shared", "a/**").op(t, "F").Grade; got != "red" {
		t.Errorf("grade = %q, want red", got)
	}
}

func TestAnOpMissingFromTheReportIsRedAndHasNoScores(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod, "a/a.go": "package a\n\nfunc F() {}\n", "crap.json": crapReport(),
	})
	op := readGrades(t, dir, "--crap-report", "crap.json").op(t, "F")
	if op.Grade != "red" {
		t.Errorf("grade = %q, want red", op.Grade)
	}
	if op.Crap != nil || op.Complexity != nil || op.Coverage != nil {
		t.Errorf("op = %+v, want no scores", op)
	}
}

func TestAnIgnoredFunctionIsMissingFromTheReportSoItIsRed(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a/a.go":    "package a\n\n//gocrap:ignore generated\nfunc F() {}\n",
		"crap.json": crapReport(),
	})
	if got := readGrades(t, dir, "--crap-report", "crap.json").op(t, "F").Grade; got != "red" {
		t.Errorf("grade = %q, want red", got)
	}
}

func TestACoverageOfZeroIsPrinted(t *testing.T) {
	row := `{"file": "a/a.go", "line": 3, "function": "F", "score": 2, "complexity": 1, "coverage": 0, "limit": 0}`
	dir := writeTree(t, map[string]string{
		"go.mod": goMod, "a/a.go": "package a\n\nfunc F() {}\n", "crap.json": crapReport(row),
	})
	if got := readGrades(t, dir, "--crap-report", "crap.json").op(t, "F").Coverage; got == nil || *got != 0 {
		t.Errorf("coverage = %v, want 0", got)
	}
}

func TestAFunctionWithNoBodyHasNoGradeAndDoesNotCountAsMissing(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a/a.go":    "package a\n\nfunc Stub()\n\nfunc F() {}\n",
		"b/b.go":    "package b\n\nfunc Stub2()\n",
		"crap.json": crapReport(crapRow("a/a.go", 5, "F", 1)),
	})
	got := readGrades(t, dir, "--crap-report", "crap.json")
	if grade := got.op(t, "Stub").Grade; grade != "" {
		t.Errorf("Stub = %q, want no grade", grade)
	}
	wantClassGrades(t, got, map[string]string{"a": "green", "b": ""})
	wantBoxGrades(t, got, map[string]string{"a": "green", "b": ""})
}

func TestAFunctionWithNoBodyTakesNoRow(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod, "a/a.go": "package a\n\nfunc Stub()\n", "crap.json": crapReport(crapRow("a/a.go", 3, "Stub", 1)),
	})
	stdout, stderr := run(t, dir, 0, "--format", "json", "--crap-report", "crap.json")
	if strings.Contains(stdout, `"crap"`) {
		t.Errorf("stdout = %s; want no scores", stdout)
	}
	if !strings.Contains(stderr, "gocrap rows with no function: 1") {
		t.Errorf("stderr = %q; want the row counted", stderr)
	}
}

func TestAClassAndEveryBoxAboveItTakeTheWorstGradeBelowThem(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":              goMod,
		"root.go":             "package shop\n\nfunc Root() {}\n",
		"billing/billing.go":  "package billing\n\nfunc Calm() {}\n",
		"billing/tax/tax.go":  "package tax\n\nfunc Mild() {}\n\nfunc Hot() {}\n",
		"billing/card/pay.go": "package card\n\nfunc Pay() {}\n",
		"web/web.go":          "package web\n\nfunc Serve() {}\n",
		"empty/empty.go":      "package empty\n",
		"crap.json": crapReport(
			crapRow("root.go", 3, "Root", 40),
			crapRow("billing/billing.go", 3, "Calm", 1),
			crapRow("billing/tax/tax.go", 3, "Mild", 1),
			crapRow("billing/tax/tax.go", 5, "Hot", 20),
			crapRow("billing/card/pay.go", 3, "Pay", 1),
			crapRow("web/web.go", 3, "Serve", 1),
		),
	})
	got := readGrades(t, dir, "--crap-report", "crap.json")
	wantClassGrades(t, got, map[string]string{
		".": "red", "billing": "green", "billing/tax": "yellow", "billing/card": "green", "web": "green", "empty": "",
	})
	wantBoxGrades(t, got, map[string]string{
		"billing": "yellow", "billing/tax": "yellow", "billing/card": "green", "web": "green", "empty": "",
	})
}

func TestTheSharedBoxesTakeTheGradeOfSharedCode(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":     goMod,
		"lib/log.go": "package lib\n\nfunc Log() {}\n",
		".goalierc":  `{"shared": ["lib/**"], "gocrap": {"sharedLimit": 8}}`,
		"crap.json":  crapReport(crapRow("lib/log.go", 3, "Log", 9)),
	})
	wantBoxGrades(t, readGrades(t, dir, "--crap-report", "crap.json"), map[string]string{":shared": "red", ":shared/lib": "red"})
}

func TestAForeignClassHasNoGrade(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a/a.go":    "package a\n\nimport _ \"example.com/lib\"\n\nfunc F() {}\n",
		".goalierc": `{"godeps": {"foreign": ["example.com/lib"]}}`,
		"crap.json": crapReport(crapRow("a/a.go", 5, "F", 1)),
	})
	stdout, _ := run(t, dir, 0, "--format", "json", "--crap-report", "crap.json")
	if got := strings.Count(stdout, `"grade": "green"`); got != 3 {
		t.Errorf("green grades = %d, want 3 for the op, the class, and the box\n%s", got, stdout)
	}
}

func TestWithNoReportNothingHasAGrade(t *testing.T) {
	tests := []struct {
		name, config string
		args         []string
	}{
		{"nothing set", `{}`, nil},
		{"empty setting", `{"godeps": {"crapReport": ""}}`, nil},
		{"empty flag replaces the setting", `{"godeps": {"crapReport": "gone.json"}}`, []string{"--crap-report", ""}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{
				"go.mod": goMod, "a/a.go": "package a\n\nfunc F() {}\n", ".goalierc": test.config,
			})
			stdout, _ := run(t, dir, 0, append([]string{"--format", "json"}, test.args...)...)
			for _, field := range []string{`"grade"`, `"crap"`, `"complexity"`, `"coverage"`} {
				if strings.Contains(stdout, field) {
					t.Errorf("stdout = %s; want no %s", stdout, field)
				}
			}
		})
	}
}

func TestTheLimitInTheReportIsIgnored(t *testing.T) {
	row := `{"file": "a/a.go", "line": 3, "function": "F", "score": 5, "complexity": 2, "coverage": 0.5, "limit": 1}`
	dir := writeTree(t, map[string]string{
		"go.mod": goMod, "a/a.go": "package a\n\nfunc F() {}\n", "crap.json": crapReport(row),
	})
	if got := readGrades(t, dir, "--crap-report", "crap.json").op(t, "F").Grade; got != "green" {
		t.Errorf("grade = %q, want green", got)
	}
}
