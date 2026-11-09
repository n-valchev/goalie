package goalie_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

type similarAnswer struct {
	Matches []struct {
		Score    float64 `json:"score"`
		File     string  `json:"file"`
		Line     int     `json:"line"`
		Function string  `json:"function"`
	} `json:"matches"`
	More int `json:"more"`
}

func (c *mcpClient) similar(args map[string]any) similarAnswer {
	c.t.Helper()
	result := c.callTool("find_similar", args)
	if result.IsError {
		c.t.Fatalf("find_similar gave an error: %s", result.Content[0].Text)
	}
	var answer similarAnswer
	if err := json.Unmarshal([]byte(result.Content[0].Text), &answer); err != nil {
		c.t.Fatalf("answer %q: %v", result.Content[0].Text, err)
	}
	return answer
}

// String gives each match as "file:line function score", one to a line.
func (a similarAnswer) String() string {
	var lines []string
	for _, match := range a.Matches {
		lines = append(lines, fmt.Sprintf("%s:%d %s %.2f", match.File, match.Line, match.Function, match.Score))
	}
	return strings.Join(lines, "\n")
}

// nearFunc has one line more than sumFunc, so that the two score 0.85: over the default threshold,
// and under 0.9.
func nearFunc() string {
	return strings.Replace(sumFunc("Near"), "\treturn total\n}", "\ttotal++\n\treturn total\n}", 1)
}

func TestServeFindSimilarGivesTheCopyOfCode(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"total/total.go": "package total\n\n" + sumFunc("Total"),
		"join/join.go":   "package join\n\nimport \"strings\"\n\n" + joinFunc("Join"),
	})
	client := startServe(t, dir)
	client.initialize()

	answer := client.similar(map[string]any{"dir": dir, "code": sumFunc("Sum")})
	if got, want := answer.String(), "total/total.go:3 Total 1.00"; got != want || answer.More != 0 {
		t.Errorf("matches =\n%s\nmore %d, want\n%s\nmore 0", got, answer.More, want)
	}
}

func TestServeFindSimilarGivesTheCopyOfTheFunctionAtFileAndLine(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"svc/total.go": "package svc\n\n" + sumFunc("Total"),
		"svc/sum.go":   "package svc\n\n" + sumFunc("Sum"),
	})
	client := startServe(t, dir)
	client.initialize()

	// Line 6 is inside Sum, which starts on line 3. file is relative to dir.
	answer := client.similar(map[string]any{"dir": filepath.Join(dir, "svc"), "file": "sum.go", "line": 6})
	if got, want := answer.String(), "svc/total.go:3 Total 1.00"; got != want {
		t.Errorf("matches =\n%s\nwant\n%s", got, want)
	}
}

func TestServeFindSimilarGivesNoMatchForAFunctionWithNoCopy(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n\n" + sumFunc("Total")})
	client := startServe(t, dir)
	client.initialize()

	// The second is a copy under the size limits of dup, which dup never reports.
	for _, code := range []string{joinFunc("Join"), "func Small() int {\n\treturn 1\n}\n"} {
		result := client.callTool("find_similar", map[string]any{"dir": dir, "code": code})
		if want := `{"matches":[],"more":0,"unparsed":[]}`; result.IsError || result.Content[0].Text != want {
			t.Errorf("find_similar = %+v, want %s", result, want)
		}
	}
}

// A function with the name of one in the same package is a variant for another platform, which dup
// doesn't report. So the package of code decides.
func TestServeFindSimilarReadsCodeInThePackageOfDir(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"total/total.go": "package total\n\n" + sumFunc("Total"),
		// A generator that go build leaves out, named to sort before total.go.
		"total/gen.go":   "//go:build ignore\n\npackage main\n",
		"other/other.go": "package other\n",
	})
	client := startServe(t, dir)
	client.initialize()

	for _, tc := range []struct{ folder, want string }{
		{"total", ""},
		{"other", "total/total.go:3 Total 1.00"},
	} {
		answer := client.similar(map[string]any{"dir": filepath.Join(dir, tc.folder), "code": sumFunc("Total")})
		if got := answer.String(); got != tc.want {
			t.Errorf("in %s: matches = %q, want %q", tc.folder, got, tc.want)
		}
	}
}

func TestServeFindSimilarGivesThe10BestMatchesAndTheCountOfTheRest(t *testing.T) {
	files := map[string]string{}
	for i := range 12 {
		files[fmt.Sprintf("c%02d.go", i)] = "package a\n\n" + sumFunc(fmt.Sprintf("Copy%02d", i))
	}
	// a.go sorts first, so Near comes after the copies only because of its score.
	files["a.go"] = "package a\n\n" + nearFunc()
	dir := gitRepo(t, files)
	client := startServe(t, dir)
	client.initialize()

	answer := client.similar(map[string]any{"dir": dir, "code": sumFunc("Sum")})
	var want []string
	for i := range 10 {
		want = append(want, fmt.Sprintf("c%02d.go:3 Copy%02d 1.00", i, i))
	}
	if got := answer.String(); got != strings.Join(want, "\n") || answer.More != 3 {
		t.Errorf("matches =\n%s\nmore %d, want\n%s\nmore 3", got, answer.More, strings.Join(want, "\n"))
	}
}

func TestServeFindSimilarGivesAnErrorForInputWithNoFunction(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n\n" + sumFunc("Total")})
	client := startServe(t, dir)
	client.initialize()

	for _, tc := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"code that doesn't parse", map[string]any{"code": "func broken( {\n}\n"}, "code doesn't parse as a Go function: code:1:"},
		{"code with no function", map[string]any{"code": "var x = 1\n"}, "code must be one Go function with a body, got 0"},
		{"code with two functions", map[string]any{"code": sumFunc("One") + sumFunc("Two")}, "code must be one Go function with a body, got 2"},
		{"a line outside any function", map[string]any{"file": "a.go", "line": 1}, "a.go:1 is in no function with a body"},
		{"neither code nor file", map[string]any{}, "needs code, or file and line"},
		{"file with no line", map[string]any{"file": "a.go"}, "needs code, or file and line"},
		{"both code and file", map[string]any{"code": sumFunc("Sum"), "file": "a.go", "line": 3}, "give code, or file and line, not both"},
	} {
		tc.args["dir"] = dir
		result := client.callTool("find_similar", tc.args)
		if !result.IsError || !strings.HasPrefix(result.Content[0].Text, tc.want) {
			t.Errorf("%s: find_similar = %+v, want an error that starts with %q", tc.name, result, tc.want)
		}
	}
}

func TestServeFindSimilarSkipsAFileThatDoesNotParse(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n\n" + sumFunc("Total")})
	write(t, dir, "broken.go", "package a\n\nfunc Broken( {\n")
	client := startServe(t, dir)
	client.initialize()

	result := client.callTool("find_similar", map[string]any{"dir": dir, "code": sumFunc("Sum")})
	want := `{"matches":[{"score":1,"file":"a.go","line":3,"function":"Total"}],"more":0,"unparsed":["broken.go"]}`
	if result.IsError || result.Content[0].Text != want {
		t.Errorf("find_similar = %+v, want %s", result, want)
	}
}

func TestServeFindSimilarLeavesOutWhatDupLeavesOut(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"a_test.go": "package a\n\n" + sumFunc("TotalInATest"),
		"b.go":      "package a\n\n// Kept for the old API.\n//goalie:ignore dup the old API keeps its own copy\n" + sumFunc("Excused"),
	})
	client := startServe(t, dir)
	client.initialize()

	if got := client.similar(map[string]any{"dir": dir, "code": sumFunc("Sum")}).String(); got != "" {
		t.Errorf("matches = %q, want none", got)
	}
	write(t, dir, ".goalierc", `{"rules": {"dup": {"includeTests": true}}}`)
	if got, want := client.similar(map[string]any{"dir": dir, "code": sumFunc("Sum")}).String(), "a_test.go:3 TotalInATest 1.00"; got != want {
		t.Errorf("with includeTests: matches = %q, want %q", got, want)
	}
}

// The agent still needs to reuse code when the repo doesn't fail on copies.
func TestServeFindSimilarFindsMatchesWhenDupIsOff(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		".goalierc": `{"rules": {"dup": {"mode": "off", "threshold": 0.9}}}`,
		"a.go":      "package a\n\n" + nearFunc(),
		"b.go":      "package a\n\n" + sumFunc("Total"),
	})
	client := startServe(t, dir)
	client.initialize()

	if got, want := client.similar(map[string]any{"dir": dir, "code": sumFunc("Sum")}).String(), "b.go:3 Total 1.00"; got != want {
		t.Errorf("matches = %q, want %q", got, want)
	}
}

func TestServeFindSimilarLeavesOutAMatchThatDupDropsAsWeak(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n\n" + outcome})
	client := startServe(t, dir)
	client.initialize()

	if got := client.similar(map[string]any{"dir": dir, "code": operationName}).String(); got != "a.go:3 outcome 1.00" {
		t.Fatalf("matches = %q, want the weak pair while weakAnchors is 0", got)
	}
	write(t, dir, ".goalierc", `{"rules": {"dup": {"weakAnchors": 1}}}`)
	if got := client.similar(map[string]any{"dir": dir, "code": operationName}).String(); got != "" {
		t.Errorf("matches = %q, want none", got)
	}
}

func TestServeFindSimilarFindsThePairThatGoalieCheckDupFinds(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"a.go": "package a\n\n" + sumFunc("Total"),
		"b.go": "package a\n\n" + nearFunc(),
		"c.go": "package a\n\nimport \"strings\"\n\n" + joinFunc("Join"),
	})
	_, stdout, _ := runDup(t, dir, "--format", "json")
	results := parseResults(t, stdout)
	if len(results) != 1 {
		t.Fatalf("goalie check dup = %s, want one pair", stdout)
	}
	left, right := results[0]["left"].(map[string]any), results[0]["right"].(map[string]any)
	want := fmt.Sprintf("%s:%v %s %.2f", right["file"], right["startLine"], right["function"], results[0]["score"])
	client := startServe(t, dir)
	client.initialize()

	answer := client.similar(map[string]any{"dir": dir, "file": left["file"], "line": left["startLine"]})
	if got := answer.String(); got != want || answer.Matches[0].Score != results[0]["score"] {
		t.Errorf("matches = %q (score %v), want %q (score %v)", got, answer.Matches, want, results[0]["score"])
	}
}
