package goalie_test

import (
	"fmt"
	"strings"
	"testing"
)

var onlyCognitiveComplexity = `{"rules": {` + rulesOff("cognitive-complexity") + `}}`

// anyScore reports any score over 0, so that each test reads the exact score from the output.
var anyScore = `{"rules": {` + rulesOff("cognitive-complexity") + `, "cognitive-complexity": {"limit": 0}}}`

func complexityWarn(score int) string {
	return fmt.Sprintf("a.go:3: warn cognitive-complexity: Parse has cognitive complexity %d (limit 0)\n", score)
}

func checkComplexity(t *testing.T, settings, body string) string {
	t.Helper()
	dir := repoWithNewCode(t, settings, withBody(body))
	code, stdout, stderr := runGoalie(t, dir, "check", "--base", "HEAD")
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	return stdout
}

func TestCognitiveComplexityReportsAScoreOverFifteen(t *testing.T) {
	cases := []struct {
		ifs        int
		wantStdout string
	}{
		{15, ""},
		{16, "a.go:3: warn cognitive-complexity: Parse has cognitive complexity 16 (limit 15)\n"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.ifs), func(t *testing.T) {
			body := strings.Repeat("\tif a {\n\t}\n", tc.ifs)
			if stdout := checkComplexity(t, onlyCognitiveComplexity, body); stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

func nestedIfs(depth int) string {
	return strings.Repeat("\tif a {\n", depth) + strings.Repeat("\t}\n", depth)
}

func TestCognitiveComplexityChargesNestingAndEachBranch(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"three nested ifs", nestedIfs(3), 6},
		{"four nested ifs", nestedIfs(4), 10},
		{"ifs side by side", strings.Repeat("\tif a {\n\t}\n", 3), 3},
		{"a switch with many cases", "\tswitch {\n\tcase a:\n\tcase b:\n\tcase c:\n\tdefault:\n\t}\n", 1},
		{"a select with many cases", "\tch := make(chan bool)\n\tselect {\n\tcase <-ch:\n\tcase ch <- a:\n\tdefault:\n\t}\n", 1},
		{"else and else if", "\tif a {\n\t} else if b {\n\t} else {\n\t}\n", 3},
		{"a nested else if", "\tif a {\n\t\tif b {\n\t\t} else if c {\n\t\t}\n\t}\n", 4},
		{"nesting below else", "\tif a {\n\t} else {\n\t\tif b {\n\t\t}\n\t}\n", 4},
		{"for, range, and a type switch", "\tfor a {\n\t\tfor range 3 {\n\t\t\tswitch any(b).(type) {\n\t\t\t}\n\t\t}\n\t}\n", 6},
		{"a closure", "\t_ = func() {\n\t\tif a {\n\t\t}\n\t}\n", 2},
		{"operators", "\tif a && b || c && d {\n\t}\n", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if stdout := checkComplexity(t, anyScore, tc.body); stdout != complexityWarn(tc.want) {
				t.Errorf("stdout = %q, want %q", stdout, complexityWarn(tc.want))
			}
		})
	}
}

func TestCognitiveComplexityExcusesOnlyAGoGuard(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"err != nil", "\tvar err error\n\tif err != nil {\n\t\treturn\n\t}\n", 0},
		{"!ok", "\t_, ok := any(a).(int)\n\tif !ok {\n\t\treturn\n\t}\n", 0},
		{"if err := f(); err != nil", "\tif err := error(nil); err != nil {\n\t\treturn\n\t}\n", 0},
		{"a name ending in Err", "\tvar readErr error\n\tif readErr != nil {\n\t\treturn\n\t}\n", 0},
		{"panic", "\tvar err error\n\tif err != nil {\n\t\tpanic(err)\n\t}\n", 0},
		{"continue", "\tvar err error\n\tfor a {\n\t\tif err != nil {\n\t\t\tcontinue\n\t\t}\n\t}\n", 1},
		{"break", "\tvar err error\n\tfor a {\n\t\tif err != nil {\n\t\t\tbreak\n\t\t}\n\t}\n", 1},
		{"more logic inside", "\tvar err error\n\tif err != nil {\n\t\tif a {\n\t\t}\n\t\treturn\n\t}\n", 3},
		{"an else", "\tvar err error\n\tif err != nil {\n\t\treturn\n\t} else {\n\t}\n", 2},
		{"a closure in the init", "\tif err := func() error {\n\t\tif a {\n\t\t}\n\t\treturn nil\n\t}(); err != nil {\n\t\treturn\n\t}\n", 3},
		{"no return at the end", "\tvar err error\n\tif err != nil {\n\t\t_ = err\n\t}\n", 1},
		{"another name", "\tvar fail error\n\tif fail != nil {\n\t\treturn\n\t}\n", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := ""
			if tc.want > 0 {
				want = complexityWarn(tc.want)
			}
			if stdout := checkComplexity(t, anyScore, tc.body); stdout != want {
				t.Errorf("stdout = %q, want %q", stdout, want)
			}
		})
	}
}
