package goalie_test

import (
	"reflect"
	"strings"
	"testing"
)

var lowCohesionAll = `{"rules": {` + rulesOff("low-cohesion") + `, "low-cohesion": {"scope": "all"}}}`

const storeType = "package a\n\ntype Store struct {\n\titems map[string]int\n\tconn  Conn\n}\n\n"

func checkLowCohesion(t *testing.T, files map[string]string, wantStdout string) {
	t.Helper()
	files[".goalierc"] = lowCohesionAll
	wantOutput(t, gitRepo(t, files), []string{"check"}, wantStdout)
}

func TestLowCohesionReportsATypeWithTwoGroups(t *testing.T) {
	cases := []struct {
		name       string
		methods    string
		wantStdout string
	}{
		{"1 group", "func (s *Store) Get(k string) int { return s.items[k] }\n" +
			"func (s *Store) Put(k string, v int) { s.items[k] = v }\n", ""},
		{"2 groups", "func (s *Store) Get(k string) int { return s.items[k] }\n" +
			"func (s *Store) Put(k string, v int) { s.items[k] = v }\n" +
			"func (s *Store) Ping() error { return s.conn.Ping() }\n" +
			"func (s *Store) Close() error { return s.conn.Close() }\n",
			"a.go:3: warn low-cohesion: Store has 2 groups of methods: Close, Ping; Get, Put (limit 1)\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkLowCohesion(t, map[string]string{"a.go": storeType + tc.methods}, tc.wantStdout)
		})
	}
}

const getAndPing = "func (s *Store) Get(k string) int { return s.items[k] }\n" +
	"func (s *Store) Ping() error { return s.conn.Ping() }\n"

func storeWarn(groups string) string {
	return "a.go:3: warn low-cohesion: Store has 2 groups of methods: " + groups + " (limit 1)\n"
}

func TestWhatJoinsTwoMethodsInOneGroup(t *testing.T) {
	cases := []struct {
		name       string
		methods    string
		wantStdout string
	}{
		{"two methods that share nothing", getAndPing, storeWarn("Get; Ping")},
		{"a call on the receiver", getAndPing + "func (s *Store) Check() int { s.Ping(); return s.Get(\"\") }\n", ""},
		{"a third method that joins two others", getAndPing +
			"func (s *Store) Both() bool { return s.items != nil && s.conn != nil }\n", ""},
		{"a method with no field use is left out", getAndPing + "func (s *Store) Name() string { return \"store\" }\n",
			storeWarn("Get; Ping")},
		{"a method that only another method calls stays in", getAndPing +
			"func (s *Store) log() {}\nfunc (s *Store) Put(k string, v int) { s.log(); s.items[k] = v }\n",
			storeWarn("Get, Put, log; Ping")},
		{"the receiver passed to a call joins every group", getAndPing + "func (s *Store) Save() { save(s) }\n", ""},
		{"a copy of the receiver passed to a call joins every group", getAndPing + "func (s *Store) Save() { save(*s) }\n", ""},
		{"a field use inside a closure", "func (s *Store) Get(k string) int { f := func() int { return s.items[k] }; return f() }\n" +
			"func (s *Store) Put(k string, v int) { s.items[k] = v }\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkLowCohesion(t, map[string]string{"a.go": storeType + tc.methods}, tc.wantStdout)
		})
	}
}

func TestWhichMethodsAndTypesCount(t *testing.T) {
	cases := []struct {
		name       string
		files      map[string]string
		wantStdout string
	}{
		{"methods in two files of the package", map[string]string{
			"a.go": storeType + "func (s *Store) Get(k string) int { return s.items[k] }\n",
			"b.go": "package a\n\nfunc (s Store) Ping() error { return s.conn.Ping() }\n",
		}, storeWarn("Get; Ping")},
		{"a method in a test file doesn't count", map[string]string{
			"a.go":      storeType + "func (s *Store) Get(k string) int { return s.items[k] }\n",
			"a_test.go": "package a\n\nfunc (s *Store) Ping() error { return s.conn.Ping() }\n",
		}, ""},
		{"a method with an accessor variant and another variant is not an accessor", map[string]string{
			"a.go": storeType + getAndPing + "func (s *Store) Conn() Conn { return s.conn }\n",
			"b.go": "package a\n\nfunc (s *Store) Conn() Conn { c := s.conn; return c }\n",
		}, storeWarn("Conn, Ping; Get")},
		{"a method whose variants return different fields is not an accessor", map[string]string{
			"a.go": storeType + getAndPing + "func (s *Store) Conn() Conn { return s.conn }\n",
			"b.go": "package a\n\nfunc (s *Store) Conn() Conn { return s.items }\n",
		}, ""},
		{"a type with fewer than 2 methods left", map[string]string{
			"a.go": storeType + "func (s *Store) Get(k string) int { return s.items[k] }\n" +
				"func (s *Store) Name() string { return \"store\" }\n",
		}, ""},
		{"a defined type that isn't a struct", map[string]string{
			"a.go": "package a\n\ntype Kind int\n\nfunc (k Kind) A() int { return int(k) }\nfunc (k Kind) B() int { return int(k) }\n",
		}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkLowCohesion(t, tc.files, tc.wantStdout)
		})
	}
}

var onlyLowCohesion = `{"rules": {` + rulesOff("low-cohesion") + `}}`

func TestLowCohesionInScopeChangesReportsATypeThatTheChangeTouches(t *testing.T) {
	const other = "\nfunc Other() int { return 1 }\n"
	const ping = "package a\n\nfunc (s *Store) Ping() error { return s.conn.Ping() }\n"
	base := map[string]string{"a.go": storeType + "func (s *Store) Get(k string) int { return s.items[k] }\n" + other, "b.go": ping}
	cases := []struct {
		name, file, now, wantStdout string
	}{
		{"a change to one method", "a.go",
			storeType + "func (s *Store) Get(k string) int { return s.items[k] + 0 }\n" + other, storeWarn("Get; Ping")},
		{"a change to a method in another file of the package", "b.go",
			"package a\n\nfunc (s *Store) Ping() error { return s.conn.Close() }\n", storeWarn("Get; Ping")},
		{"a change elsewhere in the file", "a.go", base["a.go"] + "\nfunc More() int { return 2 }\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{".goalierc": onlyLowCohesion}
			for name, src := range base {
				files[name] = src
			}
			dir := gitRepo(t, files)
			write(t, dir, tc.file, tc.now)
			wantOutput(t, dir, []string{"check", "--base", "HEAD"}, tc.wantStdout)
		})
	}
}

func wantOutput(t *testing.T, dir string, args []string, wantStdout string) {
	t.Helper()
	code, stdout, stderr := runGoalie(t, dir, args...)
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	if stdout != wantStdout {
		t.Errorf("stdout = %q, want %q", stdout, wantStdout)
	}
}

func TestGoalieIgnoreAboveATypeExcusesIt(t *testing.T) {
	const fields = "struct {\n\titems map[string]int\n\tconn  Conn\n}\n"
	const directive = "//goalie:ignore low-cohesion The store wraps both on purpose.\n"
	cases := []struct {
		name, declaration string
	}{
		{"a type on its own", directive + "type Store " + fields},
		{"a type in a group", "type (\n\t" + directive + "\tStore " + fields + ")\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkLowCohesion(t, map[string]string{"a.go": "package a\n\n" + tc.declaration + "\n" + getAndPing}, "")
		})
	}
}

func TestAMistakeInGoalieIgnoreAboveATypeExitsWithTwo(t *testing.T) {
	cases := []struct {
		directive, wantStderr string
	}{
		{"//goalie:ignore low-cohesion", "a.go:3: //goalie:ignore needs a reason"},
		{"//goalie:ignore low-cohesin The store wraps both.", `a.go:3: //goalie:ignore names unknown rule "low-cohesin"`},
	}
	for _, tc := range cases {
		t.Run(tc.directive, func(t *testing.T) {
			src := "package a\n\n" + tc.directive + "\ntype Store struct {\n\titems map[string]int\n}\n"
			dir := gitRepo(t, map[string]string{"a.go": src, ".goalierc": lowCohesionAll})
			code, _, stderr := runGoalie(t, dir, "check")
			if code != 2 || !strings.Contains(stderr, tc.wantStderr) {
				t.Errorf("got exit code %d and stderr %q, want 2 and %q", code, stderr, tc.wantStderr)
			}
		})
	}
}

// A call to health must stay a method call. As a selector that names no method, it would join the
// embedded Mutex, which Get uses.
func TestADirectiveAboveAMethodDoesNotChangeTheGroups(t *testing.T) {
	src := "package a\n\nimport \"sync\"\n\ntype Store struct {\n\tsync.Mutex\n\titems map[string]int\n\tconn  Conn\n}\n\n" +
		"func (s *Store) Get(k string) int { s.Lock(); return s.items[k] }\n" +
		"func (s *Store) Ping() error { return s.health() }\n" +
		"//goalie:ignore low-cohesion Only Ping calls it.\n" +
		"func (s *Store) health() error { return s.conn.Ping() }\n"
	checkLowCohesion(t, map[string]string{"a.go": src},
		"a.go:5: warn low-cohesion: Store has 2 groups of methods: Get; Ping, health (limit 1)\n")
}

func TestALowCohesionResultHasAJSONForm(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": storeType + getAndPing, ".goalierc": lowCohesionAll})
	code, stdout, stderr := runGoalie(t, dir, "check", "--format", "json")
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	results := parseResults(t, stdout)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1: %s", len(results), stdout)
	}
	wantFields(t, results[0], map[string]any{
		"rule": "low-cohesion", "mode": "warn", "value": 2.0, "limit": 1.0,
		"file": "a.go", "line": 3.0, "type": "Store",
	})
	if want := []any{[]any{"Get"}, []any{"Ping"}}; !reflect.DeepEqual(results[0]["groups"], want) {
		t.Errorf("groups = %v, want %v", results[0]["groups"], want)
	}
}

func TestLowCohesionLeavesOutAnAccessor(t *testing.T) {
	cases := []struct {
		name       string
		src        string
		wantStdout string
	}{
		{"a getter for each field", storeType + "func (s *Store) Items() map[string]int { return s.items }\n" +
			"func (s *Store) Conn() Conn { return s.conn }\n", ""},
		{"a field of a field", storeType + "func (s *Store) Items() map[string]int { return s.items }\n" +
			"func (s *Store) Addr() string { return s.conn.addr }\n", ""},
		{"the address of a field", storeType + "func (s *Store) Items() *map[string]int { return &s.items }\n" +
			"func (s *Store) Conn(n int) *Conn { return &s.conn }\n", ""},
		{"a call to a getter joins its field", storeType + getAndPing + "func (s *Store) Conn() Conn { return s.conn }\n" +
			"func (s *Store) Ready() bool { return s.items != nil && s.Conn() != nil }\n", ""},
		{"a method that reads a map entry still counts", storeType + getAndPing +
			"func (s *Store) Conn() Conn { return s.conn }\n", storeWarn("Get; Ping")},
		{"a method value is not an accessor", storeType + getAndPing + "func (s *Store) Pinger() func() error { return s.Ping }\n",
			storeWarn("Get; Ping, Pinger")},
		{"an error type with Error and Unwrap", "package a\n\nimport \"fmt\"\n\n" +
			"type ViolationError struct {\n\tMsg string\n\tErr error\n}\n\n" +
			"func (e *ViolationError) Error() string { return fmt.Sprintf(\"%s\", e.Msg) }\n" +
			"func (e *ViolationError) Unwrap() error { return e.Err }\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkLowCohesion(t, map[string]string{"a.go": tc.src}, tc.wantStdout)
		})
	}
}

func TestASelectorOnAnEmbeddedFieldIsOneSharedField(t *testing.T) {
	src := "package a\n\nimport \"sync\"\n\ntype Store struct {\n\tsync.Mutex\n\titems map[string]int\n\tconn  Conn\n}\n\n" +
		"func (s *Store) Get(k string) int { s.Lock(); return s.items[k] }\n" +
		"func (s *Store) Ping() error { s.Lock(); return s.conn.Ping() }\n"
	checkLowCohesion(t, map[string]string{"a.go": src}, "")
}
