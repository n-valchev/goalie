package goalie_test

import (
	"fmt"
	"strings"
	"testing"
)

var onlyComplexCondition = `{"rules": {` + rulesOff("complex-condition") + `}}`

// withBody puts body at line 4 of a.go, inside a function that has bool parameters a to e.
func withBody(body string) string {
	return "func Parse(a, b, c, d, e bool) {\n" + body + "}\n"
}

func operators(count int) string {
	return strings.Join(strings.Split("abcde"[:count+1], ""), " && ")
}

func checkConditions(t *testing.T, fn string) string {
	t.Helper()
	dir := repoWithNewCode(t, onlyComplexCondition, fn)
	code, stdout, stderr := runGoalie(t, dir, "check", "--base", "HEAD")
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	return stdout
}

const conditionWarn = "a.go:4: warn complex-condition: Parse has a condition with 4 operators (limit 3)\n"

func TestComplexConditionReportsAConditionOverThreeOperators(t *testing.T) {
	cases := []struct {
		operators  int
		wantStdout string
	}{
		{3, ""},
		{4, conditionWarn},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.operators), func(t *testing.T) {
			fn := withBody(fmt.Sprintf("\tif %s {\n\t}\n", operators(tc.operators)))
			if stdout := checkConditions(t, fn); stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

func TestComplexConditionCountsIfForAndCase(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantStdout string
	}{
		{"if", "\tif a && b && c && d && e {\n\t}\n", conditionWarn},
		{"for", "\tfor a && b && c && d && e {\n\t}\n", conditionWarn},
		{"case", "\tswitch {\n\tcase a && b && c && d && e:\n\t}\n",
			"a.go:5: warn complex-condition: Parse has a condition with 4 operators (limit 3)\n"},
		{"a case with several expressions adds them up", "\tswitch {\n\tcase a && b, c && d, e && a && b:\n\t}\n",
			"a.go:5: warn complex-condition: Parse has a condition with 4 operators (limit 3)\n"},
		{"a select case", "\tch := make(chan bool)\n\tselect {\n\tcase ch <- a && b && c && d && e:\n\tdefault:\n\t}\n", ""},
		{"a closure in the condition", "\tif func() bool { return a && b && c }() && d && e {\n\t}\n", conditionWarn},
		{"return", "\t_ = func() bool { return a && b && c && d && e }\n", ""},
		{"assignment", "\tready := a && b && c && d && e\n\t_ = ready\n", ""},
		{"& and |", "\tx := 0\n\tif x&1|x&2|x&4|x&8|x&16 != 0 {\n\t}\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if stdout := checkConditions(t, withBody(tc.body)); stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

// Both orders, so that neither the first nor the last condition passes as the worst.
func TestAFunctionReportsOnceWithItsWorstCondition(t *testing.T) {
	four, five := "\tif a && b && c && d && e {\n\t}\n", "\tif a && b && c && d && e && a {\n\t}\n"
	cases := []struct {
		name       string
		body       string
		wantStdout string
	}{
		{"worst last", four + five, "a.go:6: warn complex-condition: Parse has a condition with 5 operators (limit 3)\n"},
		{"worst first", five + four, "a.go:4: warn complex-condition: Parse has a condition with 5 operators (limit 3)\n"},
		{"a tie reports the first", four + four, conditionWarn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if stdout := checkConditions(t, withBody(tc.body)); stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}
