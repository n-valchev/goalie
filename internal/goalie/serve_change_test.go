package goalie_test

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// marks lists each result as "<function, type, or file> <change>", in the order of the answer.
func marks(answer checkAnswer) string {
	var marked []string
	for _, result := range answer.Results {
		about := cmp.Or(result["function"], result["type"], result["file"])
		if left, ok := result["left"].(map[string]any); ok {
			pair := []string{fmt.Sprint(left["function"]), fmt.Sprint(result["right"].(map[string]any)["function"])}
			slices.Sort(pair)
			about = strings.Join(pair, "~")
		}
		marked = append(marked, fmt.Sprintf("%v %v", about, result["change"]))
	}
	return strings.Join(marked, ", ")
}

func TestServeMarksWhatTheChangeDidToEachFunction(t *testing.T) {
	dir := serveRepo(t, map[string]string{
		"a.go": "package a\n\n" + longFunction("Crosses", 5) + "\n" + longFunction("Grows", 7) + "\n" + longFunction("Kept", 7),
	})
	write(t, dir, "a.go", "package a\n\n"+longFunction("Crosses", 6)+"\n"+longFunction("Grows", 8)+"\n"+
		touched("Kept", 7)+"\n"+longFunction("Added", 7))
	write(t, dir, "b.go", "package a\n\n"+longFunction("InNewFile", 7))
	client := startServe(t, dir)
	client.initialize()

	got := marks(client.check(map[string]any{"dir": dir}))
	if want := "Grows worse, Added new, InNewFile new, Crosses new, Kept unchanged"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestServeMarksWhatTheChangeDidToAFileAndAType(t *testing.T) {
	settings := `{"rules": {` + rulesOff("long-file", "low-cohesion") + `, "long-file": {"limit": 5}}}`
	store := "package a\n\ntype Store struct {\n\titems map[string]int\n\tconn  Conn\n\tlog   Log\n}\n\n" +
		"func (s *Store) Get(k string) int { return s.items[k] }\n"
	dir := gitRepo(t, map[string]string{
		".goalierc": settings,
		"a.go":      store,
		"b.go":      "package a\n\nfunc (s *Store) Ping() error { return s.conn.Ping() }\n",
		"c.go":      "package a\n\nvar x = 1\n",
	})
	write(t, dir, "a.go", store+"func (s *Store) Note() { s.log.Print() }\n")
	write(t, dir, "c.go", "package a\n\nvar x = 1\n\nvar y = 2\n\nvar z = 3\n")
	client := startServe(t, dir)
	client.initialize()

	got := marks(client.check(map[string]any{"dir": dir}))
	if want := "Store worse, c.go new"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestServeMarksWhatTheChangeDidToADuplicatePair(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		".goalierc": `{"rules": {` + rulesOff("dup") + `}}`,
		"a.go":      "package a\n\n" + sumFunc("OldA"),
		"b/b.go":    "package b\n\n" + sumFunc("OldB"),
	})
	write(t, dir, "a.go", "package a\n\n"+strings.Replace(sumFunc("OldA"), "total := 0", "total := 0 // Start.", 1)+
		"\n"+sumFunc("Added"))
	client := startServe(t, dir)
	client.initialize()

	got := marks(client.check(map[string]any{"dir": dir}))
	if want := "Added~OldA new, Added~OldB new, OldA~OldB unchanged"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func touched(name string, lines int) string {
	return strings.Replace(longFunction(name, lines), "_ = 0", "_ = 1", 1)
}

func TestServePutsWhatTheChangeMadeFirstThenFailBeforeWarn(t *testing.T) {
	// many-arguments warns at a value 3 times its limit, and long-function fails at 1.4 times.
	settings := `{"rules": {` + rulesOff("long-function", "many-arguments") +
		`, "long-function": {"mode": "fail", "limit": 5}, "many-arguments": {"limit": 1}}}`
	warns := func(name, body string) string { return "func " + name + "(a, b, c int) int { return " + body + " }\n" }
	dir := gitRepo(t, map[string]string{
		".goalierc": settings,
		"a.go":      "package a\n\n" + longFunction("OldFail", 7) + "\n" + warns("OldWarn", "a"),
	})
	write(t, dir, "a.go", "package a\n\n"+touched("OldFail", 7)+"\n"+warns("OldWarn", "b")+"\n"+
		longFunction("AddedFail", 7)+"\n"+warns("AddedWarn", "c"))
	client := startServe(t, dir)
	client.initialize()

	got := marks(client.check(map[string]any{"dir": dir}))
	if want := "AddedFail new, AddedWarn new, OldFail unchanged, OldWarn unchanged"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestServeLeavesOutOldResultsBeforeANewOne(t *testing.T) {
	var base, now strings.Builder
	base.WriteString("package a\n")
	now.WriteString("package a\n")
	for i := range 12 {
		base.WriteString("\n" + longFunction(fmt.Sprintf("Old%02d", i), 9))
		now.WriteString("\n" + touched(fmt.Sprintf("Old%02d", i), 9))
	}
	now.WriteString("\n" + longFunction("New", 6))
	dir := serveRepo(t, map[string]string{"a.go": base.String()})
	write(t, dir, "a.go", now.String())
	client := startServe(t, dir)
	client.initialize()

	answer := client.check(map[string]any{"dir": dir})
	if len(answer.Results) != 10 || answer.More != 3 {
		t.Fatalf("got %d results and more %d, want 10 and 3", len(answer.Results), answer.More)
	}
	if first := answer.Results[0]; first["function"] != "New" || first["change"] != "new" {
		t.Errorf("first result = %v, want New, marked new", first)
	}
}

func TestServeMarksTheResultsOfAFileThatDidNotParseAtTheBaseNew(t *testing.T) {
	dir := serveRepo(t, map[string]string{"a.go": "package a\n\n" + longFunction("Long", 7) + "\nfunc Half() {\n\tif {\n"})
	write(t, dir, "a.go", "package a\n\n"+touched("Long", 7))
	client := startServe(t, dir)
	client.initialize()

	if got := marks(client.check(map[string]any{"dir": dir})); got != "Long new" {
		t.Errorf("got %s, want Long new", got)
	}
	if stderr := client.stderr.String(); strings.Contains(stderr, "goalie:") {
		t.Errorf("stderr = %q, want no error", stderr)
	}
}
