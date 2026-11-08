package goalie_test

import (
	"fmt"
	"strings"
	"testing"
)

// onlyParameterRules turns off the other rules, so that each test sees only the parameter count.
const onlyParameterRules = `{"rules": {"dup": {"mode": "off"}, "crap": {"mode": "off"}, "long-function": {"mode": "off"}}}`

// The type S comes after fn, so that fn stays at line 3.
func withParams(fn, params string) string {
	return fmt.Sprintf("func %s(%s) {}\n\ntype S struct{}\n", fn, params)
}

func ints(count int) string {
	names := make([]string, count)
	for i := range names {
		names[i] = fmt.Sprintf("p%d", i)
	}
	return strings.Join(names, ", ") + " int"
}

func checkParameters(t *testing.T, fn string) string {
	t.Helper()
	dir := repoWithNewLongFunction(t, onlyParameterRules, fn)
	code, stdout, stderr := runGoalie(t, dir, "check", "--base", "HEAD")
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	return stdout
}

func TestManyArgumentsReportsAFunctionOverFourParameters(t *testing.T) {
	cases := []struct {
		params     int
		wantStdout string
	}{
		{4, ""},
		{5, "a.go:3: warn many-arguments: Parse has 5 parameters (limit 4)\n"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.params), func(t *testing.T) {
			if stdout := checkParameters(t, withParams("Parse", ints(tc.params))); stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

func TestAConstructorUsesManyDependenciesAndNotManyArguments(t *testing.T) {
	cases := []struct {
		params     int
		wantStdout string
	}{
		{5, ""},
		{6, "a.go:3: warn many-dependencies: NewServer has 6 parameters (limit 5)\n"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.params), func(t *testing.T) {
			if stdout := checkParameters(t, withParams("NewServer", ints(tc.params))); stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

// Each function has 6 parameters, which is over both limits, so the rule name shows which one applies.
func TestOnlyAFunctionNamedNewOrNewAndACapitalIsAConstructor(t *testing.T) {
	cases := []struct {
		fn        string
		wantRule  string
		wantName  string
		wantLimit int
	}{
		{"New", "many-dependencies", "New", 5},
		{"NewServer", "many-dependencies", "NewServer", 5},
		{"Newline", "many-arguments", "Newline", 4},
		{"(s S) NewServer", "many-arguments", "S.NewServer", 4},
	}
	for _, tc := range cases {
		t.Run(tc.fn, func(t *testing.T) {
			stdout := checkParameters(t, withParams(tc.fn, ints(6)))
			want := fmt.Sprintf("a.go:3: warn %s: %s has 6 parameters (limit %d)\n", tc.wantRule, tc.wantName, tc.wantLimit)
			if stdout != want {
				t.Errorf("stdout = %q, want %q", stdout, want)
			}
		})
	}
}

func TestParametersCountEachNameAndNoReceiver(t *testing.T) {
	cases := []struct {
		name       string
		fn         string
		wantStdout string
	}{
		{"a, b int counts 2", withParams("Parse", "a, b int, c, d, e string"),
			"a.go:3: warn many-arguments: Parse has 5 parameters (limit 4)\n"},
		{"an unnamed parameter counts 1", withParams("Parse", "int, string, int, string, int"),
			"a.go:3: warn many-arguments: Parse has 5 parameters (limit 4)\n"},
		{"a variadic parameter counts 1", withParams("Parse", "a, b, c, d int, rest ...int"),
			"a.go:3: warn many-arguments: Parse has 5 parameters (limit 4)\n"},
		{"a receiver counts 0", withParams("(s *S) Parse", ints(4)), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if stdout := checkParameters(t, tc.fn); stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

func TestGoalieIgnoreManyArgumentsDoesNotHideManyDependencies(t *testing.T) {
	fn := "//goalie:ignore many-arguments The options are all required.\n" + withParams("NewServer", ints(6))
	want := "a.go:4: warn many-dependencies: NewServer has 6 parameters (limit 5)\n"
	if stdout := checkParameters(t, fn); stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}
