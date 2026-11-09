package goalie_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

type riskAnswer struct {
	File             string        `json:"file"`
	Line             int           `json:"line"`
	Function         string        `json:"function"`
	Complexity       int           `json:"complexity"`
	Coverage         *float64      `json:"coverage"`
	CrapRange        [2]float64    `json:"crapRange"`
	RequiredCoverage *float64      `json:"requiredCoverage"`
	CanPass          bool          `json:"canPass"`
	Limit            float64       `json:"limit"`
	Duplicates       similarAnswer `json:"duplicates"`
	Rules            []struct {
		Rule  string  `json:"rule"`
		Value float64 `json:"value"`
		Limit float64 `json:"limit"`
	} `json:"rules"`
	AlsoAt []struct {
		File string `json:"file"`
		Line int    `json:"line"`
	} `json:"alsoAt"`
}

func (c *mcpClient) risk(args map[string]any) riskAnswer {
	c.t.Helper()
	result := c.callTool("risk", args)
	if result.IsError {
		c.t.Fatalf("risk gave an error: %s", result.Content[0].Text)
	}
	var answer riskAnswer
	if err := json.Unmarshal([]byte(result.Content[0].Text), &answer); err != nil {
		c.t.Fatalf("answer %q: %v", result.Content[0].Text, err)
	}
	return answer
}

func TestServeRiskOfASimpleFunction(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n\n" + sumFunc("Total")})
	client := startServe(t, dir)
	client.initialize()

	// Total has a for and two ifs, so its complexity is 4, and 4² + 4 = 20 is under the limit of 30.
	answer := client.risk(map[string]any{"dir": dir, "file": "a.go", "line": 5})
	if answer.Function != "Total" || answer.Complexity != 4 || answer.CrapRange != [2]float64{4, 20} {
		t.Errorf("risk = %+v, want Total with complexity 4 and crapRange [4 20]", answer)
	}
	if answer.Coverage != nil || answer.RequiredCoverage == nil || *answer.RequiredCoverage != 0 ||
		!answer.CanPass || answer.Limit != 30 {
		t.Errorf("risk = %+v, want no coverage, requiredCoverage 0, canPass, and limit 30", answer)
	}
	if len(answer.Duplicates.Matches) != 0 || len(answer.Rules) != 0 {
		t.Errorf("risk = %+v, want no duplicates and no rules", answer)
	}
}

func TestServeRiskListsTheCopiesOfTheFunction(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"total/total.go": "package total\n\n" + sumFunc("Total"),
		"sum/sum.go":     "package sum\n\n" + sumFunc("Sum"),
	})
	client := startServe(t, dir)
	client.initialize()

	answer := client.risk(map[string]any{"dir": dir, "file": "sum/sum.go", "line": 3})
	if got, want := answer.Duplicates.String(), "total/total.go:3 Total 1.00"; got != want || answer.Duplicates.More != 0 {
		t.Errorf("duplicates =\n%s\nmore %d, want\n%s\nmore 0", got, answer.Duplicates.More, want)
	}
	if fmt.Sprint(answer.Rules) != "[{dup 1 0.82}]" {
		t.Errorf("rules = %v, want dup with value 1 and limit 0.82", answer.Rules)
	}
}

func TestServeRiskListsTheRulesThatTheFunctionIsOver(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n\n" + longFunction("Long", 90)})
	client := startServe(t, dir)
	client.initialize()

	answer := client.risk(map[string]any{"dir": dir, "file": "a.go", "line": 3})
	if fmt.Sprint(answer.Rules) != "[{long-function 90 80}]" {
		t.Errorf("rules = %v, want long-function with value 90 and limit 80", answer.Rules)
	}
}

func TestServeRiskFindsTheFunctionOfASymbol(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"svc/total/total.go": "package total\n\ntype Counter struct{ n int }\n\nfunc (c *Counter) Add(n int) {\n\tc.n += n\n}\n\n" +
			sumFunc("Total"),
		"svc/total/open_windows.go": "package total\n\nfunc Open() int { return 2 }\n",
		"svc/total/open_linux.go":   "package total\n\nfunc Open() int { return 1 }\n",
	})
	client := startServe(t, dir)
	client.initialize()

	for symbol, want := range map[string]string{
		"svc/total.Total":       "svc/total/total.go:9 Total",
		"svc/total.Counter.Add": "svc/total/total.go:5 Counter.Add",
		"svc/total.Open":        "svc/total/open_linux.go:3 Open",
	} {
		answer := client.risk(map[string]any{"dir": dir, "symbol": symbol})
		if got := fmt.Sprintf("%s:%d %s", answer.File, answer.Line, answer.Function); got != want {
			t.Errorf("risk of %s = %s, want %s", symbol, got, want)
		}
	}
	// One function with a variant for each platform: the first file, and the others in alsoAt.
	answer := client.risk(map[string]any{"dir": dir, "symbol": "svc/total.Open"})
	if fmt.Sprint(answer.AlsoAt) != "[{svc/total/open_windows.go 3}]" {
		t.Errorf("alsoAt = %v, want open_windows.go:3", answer.AlsoAt)
	}
}

func TestServeRiskGivesAnErrorForNoFunction(t *testing.T) {
	dir := gitRepo(t, map[string]string{"svc/a.go": "package svc\n\nvar limit = 3\n\n" + sumFunc("Total")})
	client := startServe(t, dir)
	client.initialize()

	for _, tc := range []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"dir": dir, "file": "svc/a.go", "line": 3}, "svc/a.go:3 is in no function"},
		{map[string]any{"dir": dir, "symbol": "svc.Sum"}, "symbol svc.Sum names no function"},
		{map[string]any{"dir": dir, "symbol": "other.Total"}, "symbol other.Total names no function"},
		{map[string]any{"dir": dir, "symbol": "Total"}, "symbol Total names no function"},
	} {
		result := client.callTool("risk", tc.args)
		if !result.IsError || !strings.Contains(result.Content[0].Text, tc.want) {
			t.Errorf("risk %v = %+v, want an error that says %q", tc.args, result, tc.want)
		}
	}
}

func TestServeRiskReadsNoSymbolOutsideTheFolderOfTheGoalierc(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"svc/.goalierc":  "{}",
		"svc/a.go":       "package svc\n",
		"other/other.go": "package other\n\n" + sumFunc("Total"),
	})
	client := startServe(t, dir)
	client.initialize()

	result := client.callTool("risk", map[string]any{"dir": filepath.Join(dir, "svc"), "symbol": "../other.Total"})
	if !result.IsError || !strings.Contains(result.Content[0].Text, "names no function") {
		t.Errorf("risk of ../other.Total = %+v, want an error", result)
	}
}

// branchyFunc has complexity ifs + 1.
func branchyFunc(name string, ifs int) string {
	return fmt.Sprintf("func %s(n int) int {\n%s\treturn n\n}\n", name, strings.Repeat("\tif n > 0 {\n\t\tn--\n\t}\n", ifs))
}

func TestServeRiskOfAComplexFunctionGivesTheCoverageItNeeds(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n\n" + branchyFunc("Some", 7) + "\n" + branchyFunc("Many", 30)})
	client := startServe(t, dir)
	client.initialize()

	// 8² × (1 - coverage)³ + 8 = 30 at a coverage of 29.95%, rounded up.
	some := client.risk(map[string]any{"dir": dir, "file": "a.go", "line": 3})
	if some.Function != "Some" || some.CrapRange != [2]float64{8, 72} || !some.CanPass ||
		some.RequiredCoverage == nil || *some.RequiredCoverage != 0.3 {
		t.Errorf("risk of Some = %+v, want crapRange [8 72] and requiredCoverage 0.3", some)
	}
	// A score never falls below the complexity, and 31 is over the limit.
	many := client.risk(map[string]any{"dir": dir, "file": "a.go", "line": 30})
	if many.Function != "Many" || many.Complexity != 31 || many.CanPass || many.RequiredCoverage != nil {
		t.Errorf("risk of Many = %+v, want complexity 31, canPass false, and no requiredCoverage", many)
	}
}
