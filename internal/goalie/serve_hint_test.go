package goalie_test

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/n-valchev/goalie/internal/config"
)

// everyRuleCode gives each rule a result at a limit of 0, and low-cohesion at its default of 1.
const everyRuleCode = `package a

type Store struct{ a, b int }

func NewStore(a int) *Store { return &Store{a: a} }

func (s *Store) Up() int { s.a++; return s.a }

func (s *Store) Down() int { s.b--; return s.b }

func Pass(id string) bool { return Both(id, true) }

func Both(id string, b bool) bool {
	if len(id) > 0 && b {
		return true
	}
	return false
}

`

// fixList is the list of fixes in the README section "Steer coding agents".
func fixList(t *testing.T) []string {
	t.Helper()
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	_, list, _ := strings.Cut(string(readme), "Fix every result before you finish:\n\n")
	list, _, _ = strings.Cut(list, "\n\n")
	return strings.Split(list, "\n")
}

// fixFor lowers the first letter of hint, because the README writes each fix after a colon.
func fixFor(fixes []string, rule, hint string) bool {
	first, size := utf8.DecodeRuneInString(hint)
	for _, fix := range fixes {
		if strings.Contains(fix, "`"+rule) && strings.HasSuffix(fix, ": "+string(unicode.ToLower(first))+hint[size:]) {
			return true
		}
	}
	return false
}

func TestServeGivesCrapTheHintOfItsCase(t *testing.T) {
	// At a limit of 2, complexity 2 needs coverage, and complexity 3 can't pass.
	dir := gitRepo(t, map[string]string{
		".goalierc": `{"rules": {` + rulesOff("crap") + `, "crap": {"limit": 2}}}`,
		"a.go": "package a\n\nfunc Two(a bool) int {\n\tif a {\n\t\treturn 1\n\t}\n\treturn 0\n}\n\n" +
			"func Three(a, b bool) int {\n\tif a {\n\t\treturn 1\n\t}\n\tif b {\n\t\treturn 2\n\t}\n\treturn 0\n}\n",
	})
	client := startServe(t, dir)
	client.initialize()

	answer := client.check(map[string]any{"dir": dir, "scope": "all"})
	want := map[string]string{
		"Three": "Split the function. No test can bring it under the limit.",
		"Two":   "Write tests that cover at least that much of the function.",
	}
	if len(answer.Results) != len(want) {
		t.Fatalf("got %d results, want %d: %v", len(answer.Results), len(want), answer.Results)
	}
	fixes := fixList(t)
	for _, result := range answer.Results {
		function := fmt.Sprint(result["function"])
		if result["hint"] != want[function] {
			t.Errorf("%s: hint = %v, want %q", function, result["hint"], want[function])
		}
		if !fixFor(fixes, "crap", want[function]) {
			t.Errorf("%s: the README fix list has no crap line that ends with %q", function, want[function])
		}
	}
}

func TestServeGivesEachResultOfEveryRuleTheHintOfTheReadme(t *testing.T) {
	fixes := fixList(t)
	files := map[string]string{}
	var rules []string
	fields := reflect.TypeFor[config.Rules]()
	for i := range fields.NumField() {
		rule := fields.Field(i).Tag.Get("json")
		rules = append(rules, rule)
		limit := `, "` + rule + `": {"limit": 0}`
		if rule == "dup" || rule == "low-cohesion" {
			limit = ""
		}
		files[rule+"/.goalierc"] = `{"rules": {` + rulesOff(rule) + limit + `}}`
		files[rule+"/a.go"] = everyRuleCode + sumFunc("Total") + "\n" + sumFunc("Copied")
	}
	dir := gitRepo(t, files)
	client := startServe(t, dir)
	client.initialize()

	for _, rule := range rules {
		answer := client.check(map[string]any{"dir": dir + "/" + rule, "scope": "all"})
		if len(answer.Results) == 0 {
			t.Errorf("%s: no results", rule)
		}
		for _, result := range answer.Results {
			hint := fmt.Sprint(result["hint"])
			if result["rule"] != rule || !fixFor(fixes, rule, hint) {
				t.Errorf("%s: %v gives hint %q, which isn't the README fix for %s", rule, result["rule"], hint, rule)
			}
		}
	}
}
