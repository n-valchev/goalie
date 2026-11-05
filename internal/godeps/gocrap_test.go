package godeps_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type scoredOp struct {
	Name       string   `json:"name"`
	Crap       *float64 `json:"crap"`
	Complexity *int     `json:"complexity"`
	Coverage   *float64 `json:"coverage"`
	Grade      string   `json:"grade"`
}

type gradedClass struct {
	Path  string     `json:"path"`
	Grade string     `json:"grade"`
	Ops   []scoredOp `json:"ops"`
}

type gradedBox struct {
	ID    string `json:"id"`
	Grade string `json:"grade"`
}

type gradedModel struct {
	Packages []gradedBox   `json:"packages"`
	Classes  []gradedClass `json:"classes"`
}

func (m gradedModel) op(t *testing.T, name string) scoredOp {
	t.Helper()
	for _, class := range m.Classes {
		for _, op := range class.Ops {
			if op.Name == name {
				return op
			}
		}
	}
	t.Fatalf("no op %s in %+v", name, m.Classes)
	return scoredOp{}
}

func crapRow(file string, line int, function string, score float64) string {
	return fmt.Sprintf(`{"file": %q, "line": %d, "function": %q, "score": %g, "complexity": 2, "coverage": 0.5, "limit": 0}`,
		file, line, function, score)
}

func crapReport(rows ...string) string {
	return `{"findings": [` + strings.Join(rows, ",") + `]}`
}

func TestABadCrapReportIsAnError(t *testing.T) {
	tests := []struct{ name, report string }{
		{"empty file", ""},
		{"text output of gocrap", "a/a.go:3: F scores 2.0 (complexity 1, coverage 0.0%, limit 0)\n"},
		{"cut short", `{"findings": [`},
		{"no findings", `{}`},
		{"null findings", `{"findings": null}`},
		{"not an object", `[]`},
		{"wrong type", `{"findings": [{"file": "a/a.go", "line": "3"}]}`},
		{"a godry report", `{"findings": [{"score": 0.9, "left": {"file": "a/a.go", "startLine": 3}}]}`},
		{"a row with no line", `{"findings": [{"file": "a/a.go", "score": 1}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{"go.mod": goMod, "a/a.go": "package a\n", "crap.json": test.report})
			wantError(t, dir, "crap.json", "--format", "json", "--crap-report", "crap.json")
		})
	}
}

func TestAMissingCrapReportIsAnError(t *testing.T) {
	tests := []struct {
		name, config string
		args         []string
	}{
		{"flag", `{}`, []string{"--crap-report", "gone.json"}},
		{"setting", `{"godeps": {"crapReport": "gone.json"}}`, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wantError(t, smallTree(t, test.config), "gone.json", append([]string{"--format", "json"}, test.args...)...)
		})
	}
}

func TestTheCrapReportSettingApplies(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a/a.go":    "package a\n\nfunc F() {}\n",
		".goalierc": `{"godeps": {"crapReport": "crap.json"}}`,
		"crap.json": crapReport(crapRow("a/a.go", 3, "F", 1)),
	})
	if got := readModel[gradedModel](t, dir).op(t, "F").Grade; got != "green" {
		t.Errorf("grade = %q, want green", got)
	}
}

func TestAnAbsoluteCrapReportPathIsReadAsItIs(t *testing.T) {
	reports := writeTree(t, map[string]string{"crap.json": crapReport(crapRow("a/a.go", 3, "F", 1))})
	dir := writeTree(t, map[string]string{"go.mod": goMod, "a/a.go": "package a\n\nfunc F() {}\n"})
	if got := readModel[gradedModel](t, dir, "--crap-report", reports+"/crap.json").op(t, "F").Grade; got != "green" {
		t.Errorf("grade = %q, want green", got)
	}
}

func TestRowsThatMatchNoOpGiveOneWarningWithTheCount(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a/a.go": "package a\n\nfunc F() {}\n",
		"crap.json": crapReport(
			crapRow("a/a.go", 3, "F", 1), crapRow("a/a.go", 9, "Moved", 1), crapRow("gone/gone.go", 3, "Gone", 1),
		),
	})
	_, stderr := run(t, dir, 0, "--format", "json", "--crap-report", "crap.json")
	if want := "godeps: gocrap rows with no function: 2\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}

func TestEveryRowMatchingAnOpGivesNoWarning(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod, "a/a.go": "package a\n\nfunc F() {}\n", "crap.json": crapReport(crapRow("a/a.go", 3, "F", 1)),
	})
	if _, stderr := run(t, dir, 0, "--format", "json", "--crap-report", "crap.json"); stderr != "" {
		t.Errorf("stderr = %q, want nothing", stderr)
	}
}

func TestARowOfAnOmittedClassCountsAsMatchingNothing(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"gen/g.go":  "package gen\n\nfunc G() {}\n",
		".goalierc": `{"godeps": {"omit": ["gen"]}}`,
		"crap.json": crapReport(crapRow("gen/g.go", 3, "G", 1)),
	})
	_, stderr := run(t, dir, 0, "--format", "json", "--crap-report", "crap.json")
	if !strings.Contains(stderr, "gocrap rows with no function: 1") {
		t.Errorf("stderr = %q; want the row counted", stderr)
	}
}

func TestAPositionMatchesEvenWhenTheNameDiffers(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod, "a/a.go": "package a\n\nfunc F() {}\n", "crap.json": crapReport(crapRow("a/a.go", 3, "Old", 40)),
	})
	if got := readModel[gradedModel](t, dir, "--crap-report", "crap.json").op(t, "F").Crap; got == nil || *got != 40 {
		t.Errorf("crap = %v, want 40", got)
	}
}

func TestTwoFunctionsOnOneLineEachTakeTheirOwnRow(t *testing.T) {
	calm, hot := crapRow("a/a.go", 3, "F", 1), crapRow("a/a.go", 3, "G", 40)
	for _, report := range []string{crapReport(calm, hot), crapReport(hot, calm)} {
		dir := writeTree(t, map[string]string{
			"go.mod": goMod, "a/a.go": "package a\n\nfunc F() {}; func G() {}\n", "crap.json": report,
		})
		stdout, stderr := run(t, dir, 0, "--format", "json", "--crap-report", "crap.json")
		var got gradedModel
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatal(err)
		}
		if grade := got.op(t, "F").Grade; grade != "green" {
			t.Errorf("F = %q, want green", grade)
		}
		if grade := got.op(t, "G").Grade; grade != "red" {
			t.Errorf("G = %q, want red", grade)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want nothing", stderr)
		}
	}
}

func TestTheScoresPrintInTheDocumentedPlaces(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a/a.go":    "package a\n\nfunc F() {}\n",
		"crap.json": crapReport(crapRow("a/a.go", 3, "F", 6)),
	})
	stdout, _ := run(t, dir, 0, "--format", "json", "--crap-report", "crap.json", "--editor-url", "edit")
	want := `{
  "version": 1,
  "title": "example.com/shop",
  "module": "example.com/shop",
  "packages": [
    {
      "id": "a",
      "name": "a",
      "packageCount": 1,
      "grade": "green"
    }
  ],
  "classes": [
    {
      "id": "example.com/shop/a",
      "name": "a",
      "path": "a",
      "package": "a",
      "grade": "green",
      "types": [],
      "ops": [
        {
          "name": "F",
          "file": "a/a.go",
          "line": 3,
          "endLine": 3,
          "exported": true,
          "url": "edit",
          "crap": 6,
          "complexity": 2,
          "coverage": 0.5,
          "grade": "green"
        }
      ]
    }
  ],
  "edges": []
}
`
	if stdout != want {
		t.Errorf("stdout = %s\nwant %s", stdout, want)
	}
}

func TestAnOpTakesItsScoresFromTheRowAtItsFileAndLine(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a/a.go":    "package a\n\nfunc F() {}\n",
		"crap.json": crapReport(crapRow("a/a.go", 3, "F", 2.25)),
	})
	op := readModel[gradedModel](t, dir, "--crap-report", "crap.json").op(t, "F")
	if op.Crap == nil || *op.Crap != 2.25 {
		t.Errorf("crap = %v, want 2.25", op.Crap)
	}
	if op.Complexity == nil || *op.Complexity != 2 {
		t.Errorf("complexity = %v, want 2", op.Complexity)
	}
	if op.Coverage == nil || *op.Coverage != 0.5 {
		t.Errorf("coverage = %v, want 0.5", op.Coverage)
	}
	if op.Grade != "green" {
		t.Errorf("grade = %q, want green", op.Grade)
	}
}
