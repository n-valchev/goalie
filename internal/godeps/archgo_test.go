package godeps_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type violation struct {
	Rule    string   `json:"rule"`
	Details []string `json:"details"`
}

type markedEdge struct {
	From       string      `json:"from"`
	To         string      `json:"to"`
	Violations []violation `json:"violations"`
}

type archGoEntry struct {
	Package string   `json:"package"`
	Pass    bool     `json:"pass"`
	Details []string `json:"details"`
}

type archGoRule struct {
	Rule           string        `json:"rule"`
	PackageDetails []archGoEntry `json:"packageDetails"`
}

const (
	domainRule = "Packages matching pattern 'example.com/shop/domain' should ['not depend on internal packages that matches [[example.com/shop/infra]]']"
	appRule    = "Packages matching pattern 'example.com/shop/app' should ['only depend on internal packages that matches [[example.com/shop/infra]]']"
)

// shopTree is the module that the report in testdata was made from.
func shopTree(t *testing.T) string {
	t.Helper()
	return writeTree(t, map[string]string{
		"go.mod":          goMod,
		"app/app.go":      "package app\n\nimport _ \"example.com/shop/domain\"\n",
		"domain/order.go": "package domain\n\nimport _ \"example.com/shop/infra\"\n",
		"infra/db.go":     "package infra\n",
	})
}

func foreignTree(t *testing.T) string {
	t.Helper()
	return writeTree(t, map[string]string{
		"go.mod":    goMod,
		".goalierc": `{"godeps": {"foreign": ["example.com/lib"]}}`,
		"a/a.go":    "package a\n\nimport (\n\t_ \"example.com/lib/one\"\n\t_ \"example.com/lib/two\"\n)\n",
	})
}

func archGoReport(t *testing.T, rules ...archGoRule) string {
	t.Helper()
	var report struct {
		Compliance struct {
			Details struct {
				DependenciesRules struct {
					Details []archGoRule `json:"details"`
				} `json:"dependenciesRules"`
			} `json:"details"`
		} `json:"compliance"`
	}
	report.Compliance.Details.DependenciesRules.Details = rules
	content, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func breakLine(imported string) string {
	return "ShouldNotDependsOn.Internal rule contains imported package '" + imported + "'"
}

func readEdges(t *testing.T, stdout string) []markedEdge {
	t.Helper()
	var result struct {
		Edges []markedEdge `json:"edges"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not a model: %v\n%s", err, stdout)
	}
	return result.Edges
}

func wantNoViolations(t *testing.T, stdout string) {
	t.Helper()
	if strings.Contains(stdout, "violations") {
		t.Errorf("stdout = %s, want no violations", stdout)
	}
}

func wantStderr(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

func TestARealArchGoReportMarksTheEdgesThatBreakARule(t *testing.T) {
	dir := shopTree(t)
	report, err := os.ReadFile(filepath.Join("testdata", "arch-go", "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, ".arch-go/report.json", string(report))

	stdout, stderr := run(t, dir, 0, "--format", "json", "--arch-go-report", ".arch-go/report.json")

	want := []markedEdge{
		{"example.com/shop/app", "example.com/shop/domain", []violation{{
			appRule,
			[]string{"ShouldOnlyDependsOn.Internal rule doesn't contains imported package 'example.com/shop/domain'"},
		}}},
		{"example.com/shop/domain", "example.com/shop/infra", []violation{{
			domainRule,
			[]string{"ShouldNotDependsOn.Internal rule contains imported package 'example.com/shop/infra'"},
		}}},
	}
	if got := readEdges(t, stdout); !reflect.DeepEqual(got, want) {
		t.Errorf("edges = %+v, want %+v", got, want)
	}
	wantStderr(t, stderr, "godeps: arch-go lines with no imported package: 1\n")
}

func TestWithNoArchGoReportNoEdgeHasViolations(t *testing.T) {
	stdout, stderr := run(t, shopTree(t), 0, "--format", "json")
	wantNoViolations(t, stdout)
	wantStderr(t, stderr, "")
}

func TestAPassingEntryMarksNothing(t *testing.T) {
	dir := shopTree(t)
	write(t, dir, "report.json", archGoReport(t, archGoRule{"rule", []archGoEntry{
		{"example.com/shop/app", true, []string{breakLine("example.com/shop/domain")}},
		{"example.com/shop/domain", true, []string{"no path here"}},
	}}))
	stdout, stderr := run(t, dir, 0, "--format", "json", "--arch-go-report", "report.json")
	wantNoViolations(t, stdout)
	wantStderr(t, stderr, "")
}

func TestTwoRulesOnOneImportGiveTwoViolations(t *testing.T) {
	dir := shopTree(t)
	line := breakLine("example.com/shop/infra")
	write(t, dir, "report.json", archGoReport(t,
		archGoRule{"first rule", []archGoEntry{{"example.com/shop/domain", false, []string{line}}}},
		archGoRule{"second rule", []archGoEntry{{"example.com/shop/domain", false, []string{line}}}},
	))
	stdout, _ := run(t, dir, 0, "--format", "json", "--arch-go-report", "report.json")
	want := []violation{{"first rule", []string{line}}, {"second rule", []string{line}}}
	if got := readEdges(t, stdout)[1].Violations; !reflect.DeepEqual(got, want) {
		t.Errorf("violations = %+v, want %+v", got, want)
	}
}

func TestABreakMarksTheDependencyEdgeAndNotTheCompositionEdge(t *testing.T) {
	dir := writeTree(t, map[string]string{"go.mod": goMod, "a/a.go": embedsB, "b/b.go": baseFile})
	line := breakLine("example.com/shop/b")
	write(t, dir, "report.json", archGoReport(t,
		archGoRule{"rule", []archGoEntry{{"example.com/shop/a", false, []string{line}}}},
	))
	stdout, _ := run(t, dir, 0, "--format", "json", "--arch-go-report", "report.json")
	want := []markedEdge{
		{"example.com/shop/a", "example.com/shop/b", nil},
		{"example.com/shop/a", "example.com/shop/b", []violation{{"rule", []string{line}}}},
	}
	if got := readEdges(t, stdout); !reflect.DeepEqual(got, want) {
		t.Errorf("edges = %+v, want %+v", got, want)
	}
}

func TestABreakBelowAForeignModuleMarksTheEdgeToThatModule(t *testing.T) {
	dir := foreignTree(t)
	one, two := breakLine("example.com/lib/one"), breakLine("example.com/lib/two")
	write(t, dir, "report.json", archGoReport(t,
		archGoRule{"rule", []archGoEntry{{"example.com/shop/a", false, []string{one, two}}}},
	))
	stdout, stderr := run(t, dir, 0, "--format", "json", "--arch-go-report", "report.json")
	want := []markedEdge{{"example.com/shop/a", "example.com/lib", []violation{{"rule", []string{one, two}}}}}
	if got := readEdges(t, stdout); !reflect.DeepEqual(got, want) {
		t.Errorf("edges = %+v, want %+v", got, want)
	}
	wantStderr(t, stderr, "")
}

func TestBreaksWithNoEdgeWarnOnceWithTheirCount(t *testing.T) {
	dir := shopTree(t)
	write(t, dir, ".goalierc", `{"godeps": {"omit": ["infra"]}}`)
	write(t, dir, "report.json", archGoReport(t, archGoRule{"rule", []archGoEntry{
		{"example.com/shop/app", false, []string{breakLine("fmt"), breakLine("example.com/outside")}},
		{"example.com/shop/domain", false, []string{breakLine("example.com/shop/infra")}},
	}}))
	stdout, stderr := run(t, dir, 0, "--format", "json", "--arch-go-report", "report.json")
	wantNoViolations(t, stdout)
	wantStderr(t, stderr, "godeps: arch-go breaks with no edge: 3\n")
}

func TestTheArchGoReportPathComesFromTheFlagThenTheSetting(t *testing.T) {
	line := breakLine("example.com/shop/infra")
	report := func(rule string) string {
		return archGoReport(t, archGoRule{rule, []archGoEntry{{"example.com/shop/domain", false, []string{line}}}})
	}
	dir := shopTree(t)
	write(t, dir, ".goalierc", `{"godeps": {"archGoReport": "setting.json"}}`)
	write(t, dir, "setting.json", report("from setting"))
	write(t, dir, "flag.json", report("from flag"))

	tests := []struct {
		name, want string
		args       []string
	}{
		{"setting", "from setting", nil},
		{"flag", "from flag", []string{"--arch-go-report", "flag.json"}},
		{"absolute", "from flag", []string{"--arch-go-report", filepath.Join(dir, "flag.json")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, _ := run(t, dir, 0, append([]string{"--format", "json"}, test.args...)...)
			if got := readEdges(t, stdout)[1].Violations[0].Rule; got != test.want {
				t.Errorf("rule = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAnArchGoReportThatCanNotBeReadFailsTheRun(t *testing.T) {
	tests := []struct {
		name, config, content string
		args                  []string
	}{
		{"missing, flag", "{}", "", []string{"--arch-go-report", "none.json"}},
		{"missing, setting", `{"godeps": {"archGoReport": "none.json"}}`, "", nil},
		{"not JSON", "{}", "not json", []string{"--arch-go-report", "report.json"}},
		{"not JSON, setting", `{"godeps": {"archGoReport": "report.json"}}`, "not json", nil},
		{"no compliance", "{}", "{}", []string{"--arch-go-report", "report.json"}},
		{"null compliance", "{}", `{"compliance": null}`, []string{"--arch-go-report", "report.json"}},
		{"a gocrap report", "{}", `{"findings": [{"file": "a.go", "line": 3, "score": 2}]}`, []string{"--arch-go-report", "report.json"}},
		{"a godry report", "{}", `{"findings": []}`, []string{"--arch-go-report", "report.json"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := shopTree(t)
			write(t, dir, ".goalierc", test.config)
			write(t, dir, "report.json", test.content)
			stdout, stderr := run(t, dir, 2, append([]string{"--format", "json"}, test.args...)...)
			if stdout != "" {
				t.Errorf("stdout = %q, want nothing", stdout)
			}
			if !strings.Contains(stderr, ".json") {
				t.Errorf("stderr = %q, want the report's name", stderr)
			}
		})
	}
}

func TestAnArchGoReportWithNoRulesHasNoBreaks(t *testing.T) {
	dir := shopTree(t)
	write(t, dir, "report.json", `{"compliance": {}}`)
	stdout, stderr := run(t, dir, 0, "--format", "json", "--arch-go-report", "report.json")
	wantNoViolations(t, stdout)
	if stderr != "" {
		t.Errorf("stderr = %q, want nothing", stderr)
	}
}
