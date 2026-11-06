package goalie_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	dupResult  = "b.go:3: fail dup: duplicates a.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"
	crapResult = "c.go:3: fail crap: Decide scores 42.0 (complexity 6, coverage 0.0%, limit 30)\n"
	dupWarn    = "b.go:3: warn dup: duplicates a.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"
	crapWarn   = "c.go:3: warn crap: Decide scores 42.0 (complexity 6, coverage 0.0%, limit 30)\n"
)

// repoWithBothResults changes the code so that dup reports b.go at 1.00 / 0.82 = 1.22 of its limit,
// and crap reports c.go at 42 / 30 = 1.4 of its limit.
func repoWithBothResults(t *testing.T, settings string) string {
	t.Helper()
	dir := gitRepo(t, map[string]string{"go.mod": goMod, "a.go": "package m\n\n" + sumFunc("Total")})
	write(t, dir, "b.go", "package m\n\n"+sumFunc("Copied"))
	write(t, dir, "c.go", decideFile)
	write(t, dir, "p.out", emptyProfile)
	write(t, dir, ".goalierc", settings)
	return dir
}

func TestTheRuleNamesDecideWhichRulesRun(t *testing.T) {
	const profile = `"coverProfiles": ["p.out"]`
	cases := []struct {
		name       string
		settings   string
		args       []string
		wantCode   int
		wantStdout string
	}{
		{"no names runs every rule", `{"rules": {"crap": {` + profile + `}}}`, nil, 1, crapResult + dupResult},
		{"no names skips a rule that is off", `{"rules": {"dup": {"mode": "off"}, "crap": {` + profile + `}}}`, nil, 1, crapResult},
		{"one name runs that rule alone", `{"rules": {"crap": {` + profile + `}}}`, []string{"dup"}, 1, dupResult},
		{"two names run both", `{"rules": {"crap": {` + profile + `}}}`, []string{"dup", "crap"}, 1, crapResult + dupResult},
		{"a name given twice runs once", `{}`, []string{"dup", "dup"}, 1, dupResult},
		{"a named rule that is off reports as warn", `{"rules": {"dup": {"mode": "off"}, "crap": {` + profile + `}}}`,
			[]string{"dup", "crap"}, 1, crapResult + dupWarn},
		{"a named rule that is off can't fail the check", `{"rules": {"dup": {"mode": "off"}, "crap": {"mode": "warn", ` + profile + `}}}`,
			[]string{"dup", "crap"}, 0, crapWarn + dupWarn},
		{"every rule off runs nothing", `{"rules": {"dup": {"mode": "off"}, "crap": {"mode": "off"}}}`, nil, 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append(append([]string{"check"}, tc.args...), "--base", "HEAD")
			code, stdout, stderr := runGoalie(t, repoWithBothResults(t, tc.settings), args...)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d; stderr: %s", code, tc.wantCode, stderr)
			}
			if stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

func TestAMistypedRuleNameStopsTheWholeCheck(t *testing.T) {
	dir := repoWithBothResults(t, `{"rules": {"crap": {"coverCommand": "touch started"}}}`)
	code, stdout, stderr := runGoalie(t, dir, "check", "crap", "dupe", "--base", "HEAD")
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	if !strings.Contains(stderr, `"dupe"`) {
		t.Errorf("stderr = %q, want it to name the mistyped rule", stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "started")); err == nil {
		t.Error("crap ran")
	}
}

func TestAMissingRevisionStopsEveryRule(t *testing.T) {
	dir := repoWithBothResults(t, `{"rules": {"dup": {"scope": "all"}, "crap": {"coverProfiles": ["p.out"]}}}`)
	code, stdout, stderr := runGoalie(t, dir, "check")
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	if !strings.Contains(stderr, "revision") {
		t.Errorf("stderr = %q, want it to say that the check needs a revision", stderr)
	}
}

// repoWithOldResults has every problem in Git, so only a check of the whole repo can see them.
func repoWithOldResults(t *testing.T, settings string) string {
	t.Helper()
	return gitRepo(t, map[string]string{
		"go.mod":    goMod,
		"a.go":      "package m\n\n" + sumFunc("Total"),
		"b.go":      "package m\n\n" + sumFunc("Copied"),
		"c.go":      decideFile,
		"p.out":     emptyProfile,
		".goalierc": settings,
	})
}

func TestAllChecksTheWholeRepoWithNoRevision(t *testing.T) {
	const (
		oldDup     = "a.go:3: fail dup: duplicates b.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"
		oldDupWarn = "a.go:3: warn dup: duplicates b.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n"
		cantPass   = "c.go:3: fail crap: Decide can't pass at any coverage, so split it (complexity 6, limit 5)\n"

		totalNeeds  = "a.go:3: warn crap: Total needs at least 61% coverage (complexity 4, limit 5)\n"
		copiedNeeds = "b.go:3: warn crap: Copied needs at least 61% coverage (complexity 4, limit 5)\n"
	)
	cases := []struct {
		name       string
		settings   string
		args       []string
		wantCode   int
		wantStdout string
	}{
		{"scope changes becomes all", `{"rules": {"dup": {"scope": "changes"}, "crap": {"scope": "changes", "coverProfiles": ["p.out"]}}}`,
			nil, 1, crapResult + oldDup},
		{"it follows each mode", `{"rules": {"dup": {"mode": "warn"}, "crap": {"mode": "warn", "coverProfiles": ["p.out"]}}}`,
			nil, 0, crapWarn + oldDupWarn},
		{"a rule that is off stays off", `{"rules": {"dup": {"mode": "off"}, "crap": {"coverProfiles": ["p.out"]}}}`,
			nil, 1, crapResult},
		{"a revision changes nothing", `{"rules": {"crap": {"coverProfiles": ["p.out"]}}}`,
			[]string{"--base", "no-such-rev"}, 1, crapResult + oldDup},
		{"a named rule that is off runs as warn", `{"rules": {"dup": {"mode": "off"}}}`,
			[]string{"dup"}, 0, oldDupWarn},
		{"it works with a partial check", `{"rules": {"dup": {"mode": "off"}, "crap": {"limit": 5, "coverCommand": "exit 1"}}}`,
			[]string{"--partial"}, 1, cantPass + totalNeeds + copiedNeeds},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"check", "--all"}, tc.args...)
			code, stdout, stderr := runGoalie(t, repoWithOldResults(t, tc.settings), args...)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d; stderr: %s", code, tc.wantCode, stderr)
			}
			if stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
		})
	}
}

func TestFailResultsComeFirstThenTheHighestValueOverLimit(t *testing.T) {
	cases := []struct {
		name       string
		rules      string
		wantStdout string
	}{
		{"the higher ratio first", `"crap": {"coverProfiles": ["p.out"]}`, crapResult + dupResult},
		{"fail before a higher ratio", `"dup": {"mode": "fail"}, "crap": {"mode": "warn", "coverProfiles": ["p.out"]}`, dupResult + crapWarn},
		{"the ratio within warn", `"dup": {"mode": "warn"}, "crap": {"mode": "warn", "coverProfiles": ["p.out"]}`, crapWarn + dupWarn},
		{"a limit of 0 counts as 1", `"crap": {"limit": 0, "coverProfiles": ["p.out"]}`,
			"c.go:3: fail crap: Decide scores 42.0 (complexity 6, coverage 0.0%, limit 0)\n" +
				"b.go:3: fail crap: Copied scores 20.0 (complexity 4, coverage 0.0%, limit 0)\n" + dupResult},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, stdout, stderr := runGoalie(t, repoWithBothResults(t, `{"rules": {`+tc.rules+`}}`), "check", "--base", "HEAD")
			if stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q; stderr: %s", stdout, tc.wantStdout, stderr)
			}
		})
	}
}

func TestAPartialCheckMixesItsWarnResultsWithOtherRules(t *testing.T) {
	dir := repoWithBothResults(t, `{"rules": {"crap": {"limit": 20}}}`)
	code, stdout, stderr := runGoalie(t, dir, "check", "--partial", "--base", "HEAD")
	if code != 1 {
		t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr)
	}
	want := dupResult + "c.go:3: warn crap: Decide needs at least 28% coverage (complexity 6, limit 20)\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func parseResults(t *testing.T, stdout string) []map[string]any {
	t.Helper()
	var parsed struct {
		Findings []map[string]any `json:"findings"`
	}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("stdout isn't JSON: %v\n%s", err, stdout)
	}
	if parsed.Findings == nil {
		t.Fatalf("stdout has no findings list: %s", stdout)
	}
	return parsed.Findings
}

func wantFields(t *testing.T, got, want map[string]any) {
	t.Helper()
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s = %v, want %v; result: %v", key, got[key], value, got)
		}
	}
}

func TestEachJSONResultHoldsTheRuleTheValueAndTheLimit(t *testing.T) {
	dir := repoWithBothResults(t, `{"rules": {"dup": {"mode": "warn"}, "crap": {"coverProfiles": ["p.out"]}}}`)
	code, stdout, stderr := runGoalie(t, dir, "check", "--format", "json", "--base", "HEAD")
	if code != 1 {
		t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr)
	}
	results := parseResults(t, stdout)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2: %s", len(results), stdout)
	}
	wantFields(t, results[0], map[string]any{"rule": "crap", "mode": "fail", "value": 42.0, "limit": 30.0})
	wantFields(t, results[1], map[string]any{"rule": "dup", "mode": "warn", "value": 1.0, "limit": 0.82})
}

// godeps reads these fields, so they must not change.
func TestJSONResultsKeepTheFieldsOfEachRule(t *testing.T) {
	dir := repoWithBothResults(t, `{"rules": {"crap": {"coverProfiles": ["p.out"]}}}`)

	_, stdout, _ := runGoalie(t, dir, "check", "crap", "--format", "json", "--base", "HEAD")
	wantFields(t, parseResults(t, stdout)[0], map[string]any{
		"file": "c.go", "line": 3.0, "function": "Decide", "score": 42.0, "complexity": 6.0, "coverage": 0.0, "limit": 30.0,
	})

	_, stdout, _ = runGoalie(t, dir, "check", "dup", "--format", "json", "--base", "HEAD")
	result := parseResults(t, stdout)[0]
	wantFields(t, result, map[string]any{"score": 1.0, "literals": 1.0, "anchors": 0.0})
	left, ok := result["left"].(map[string]any)
	if !ok {
		t.Fatalf("left = %v, want an object", result["left"])
	}
	wantFields(t, left, map[string]any{"file": "b.go", "function": "Copied", "startLine": 3.0, "endLine": 14.0, "new": true})
	if _, ok := result["right"].(map[string]any); !ok {
		t.Errorf("right = %v, want an object", result["right"])
	}
}

func TestPartialResultsHaveAJSONForm(t *testing.T) {
	dir := gitRepo(t, map[string]string{"go.mod": goMod})
	write(t, dir, "a.go", "package m\n\n"+withComplexity("Near", 20))
	write(t, dir, "z.go", "package m\n\n"+withComplexity("Over", 31))
	code, stdout, stderr := runGoalie(t, dir, "check", "crap", "--partial", "--format", "json", "--base", "HEAD")
	if code != 1 {
		t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr)
	}
	results := parseResults(t, stdout)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2: %s", len(results), stdout)
	}
	wantFields(t, results[0], map[string]any{
		"rule": "crap", "mode": "fail", "value": 31.0, "limit": 30.0,
		"file": "z.go", "line": 3.0, "function": "Over", "complexity": 31.0, "canPass": false,
	})
	if needed, ok := results[0]["neededCoverage"]; ok {
		t.Errorf("neededCoverage = %v, want none, because no coverage can help", needed)
	}
	wantFields(t, results[1], map[string]any{
		"rule": "crap", "mode": "warn", "value": 20.0, "limit": 30.0,
		"file": "a.go", "line": 3.0, "function": "Near", "complexity": 20.0, "canPass": true, "neededCoverage": 0.71,
	})
}

func TestStdoutStaysValidJSONWithNoResults(t *testing.T) {
	cases := []struct {
		name     string
		settings string
	}{
		{"every rule off", `{"rules": {"dup": {"mode": "off"}, "crap": {"mode": "off"}}}`},
		{"no changes", `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runGoalie(t, repoWithOldResults(t, tc.settings), "check", "--format", "json", "--base", "HEAD")
			if code != 0 {
				t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
			}
			if results := parseResults(t, stdout); len(results) != 0 {
				t.Errorf("results = %v, want none", results)
			}
		})
	}
}

func TestAnUnknownFormatExitsWithTwo(t *testing.T) {
	code, stdout, stderr := runGoalie(t, repoWithBothResults(t, `{}`), "check", "--format", "xml", "--base", "HEAD")
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	if !strings.Contains(stderr, "xml") {
		t.Errorf("stderr = %q, want it to name the format", stderr)
	}
}

func TestFailedTestsLeaveTheOtherRulesVisible(t *testing.T) {
	const settings = `{"rules": {"crap": {"coverCommand": "echo test output >&2; exit 3"}}}`
	t.Run("text", func(t *testing.T) {
		code, stdout, stderr := runGoalie(t, repoWithBothResults(t, settings), "check", "--base", "HEAD")
		if code != 2 {
			t.Errorf("exit code = %d, want 2", code)
		}
		if stdout != dupResult {
			t.Errorf("stdout = %q, want %q", stdout, dupResult)
		}
		for _, want := range []string{"test output", "coverage command failed"} {
			if !strings.Contains(stderr, want) {
				t.Errorf("stderr = %q, want it to hold %q", stderr, want)
			}
		}
	})
	t.Run("json", func(t *testing.T) {
		code, stdout, _ := runGoalie(t, repoWithBothResults(t, settings), "check", "--format", "json", "--base", "HEAD")
		if code != 2 {
			t.Errorf("exit code = %d, want 2", code)
		}
		results := parseResults(t, stdout)
		if len(results) != 1 {
			t.Fatalf("got %d results, want 1: %s", len(results), stdout)
		}
		wantFields(t, results[0], map[string]any{"rule": "dup"})
	})
}

// A list with no results would read as clean code to anything that loads the file.
func TestAFailedRuleOnItsOwnWritesNoReport(t *testing.T) {
	dir := repoWithBothResults(t, `{"rules": {"crap": {"coverCommand": "exit 3"}}}`)
	code, stdout, stderr := runGoalie(t, dir, "check", "crap", "--format", "json", "--base", "HEAD")
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	if !strings.Contains(stderr, "coverage command failed") {
		t.Errorf("stderr = %q, want it to name the failure", stderr)
	}
}

func TestAWarnRuleStillExitsWithTwoWhenTheTestsFail(t *testing.T) {
	dir := repoWithBothResults(t, `{"rules": {"dup": {"mode": "warn"}, "crap": {"mode": "warn", "coverCommand": "exit 3"}}}`)
	code, stdout, stderr := runGoalie(t, dir, "check", "--base", "HEAD")
	if code != 2 {
		t.Errorf("exit code = %d, want 2; stderr: %s", code, stderr)
	}
	if stdout != dupWarn {
		t.Errorf("stdout = %q, want %q", stdout, dupWarn)
	}
}

func TestAMessageWithNoResultsGoesToStderrAndNamesTheRule(t *testing.T) {
	code, stdout, stderr := runGoalie(t, repoWithOldResults(t, `{}`), "check", "--base", "HEAD")
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	for _, want := range []string{"dup: No new functions found.", "crap: No new functions found."} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr = %q, want it to hold %q", stderr, want)
		}
	}
}
