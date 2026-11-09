package goalie_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// The defaults of the README tables "Every rule has a limit, a mode, and a scope" and the limit
// table of each rule.
const defaultRules = `dup 0.82 fail changes
crap 30 fail changes
long-function 80 warn changes
many-arguments 4 warn changes
many-dependencies 5 warn changes
complex-condition 3 warn changes
cognitive-complexity 15 warn changes
primitive-arguments 1 warn changes
long-file 1000 warn changes
high-mean-complexity 7 warn changes
low-cohesion 1 warn changes`

func (c *mcpClient) rules(dir string) string {
	c.t.Helper()
	result := c.callTool("rules", map[string]any{"dir": dir})
	if result.IsError {
		c.t.Fatalf("rules gave an error: %s", result.Content[0].Text)
	}
	var answer struct {
		Rules []map[string]any `json:"rules"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &answer); err != nil {
		c.t.Fatalf("answer %q: %v", result.Content[0].Text, err)
	}
	var lines []string
	for _, rule := range answer.Rules {
		if len(rule) != 4 {
			c.t.Errorf("%v has %d fields, want rule, limit, mode, and scope", rule, len(rule))
		}
		lines = append(lines, fmt.Sprintf("%v %v %v %v", rule["rule"], rule["limit"], rule["mode"], rule["scope"]))
	}
	return strings.Join(lines, "\n")
}

func TestServeRulesGivesTheDefaultsWithNoGoalierc(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n"})
	client := startServe(t, dir)
	client.initialize()

	if got := client.rules(dir); got != defaultRules {
		t.Errorf("rules =\n%s\nwant\n%s", got, defaultRules)
	}
}

// withRules gives the defaults with some lines replaced, such as "crap 30 fail changes" with
// "crap 30 off changes".
func withRules(replaced ...string) string {
	return strings.NewReplacer(replaced...).Replace(defaultRules)
}

func TestServeRulesGivesTheSettingsOfGoalierc(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		".goalierc": `{"rules": {"long-function": {"limit": 60, "scope": "all"}, "crap": {"mode": "off"}, ` +
			`"dup": {"threshold": 0.9, "minLines": 4}, "many-arguments": {"mode": "off", "limit": -1}}}`,
		"a.go": "package a\n",
	})
	client := startServe(t, dir)
	client.initialize()

	want := withRules(
		"long-function 80 warn changes", "long-function 60 warn all",
		"crap 30 fail changes", "crap 30 off changes",
		"dup 0.82 fail changes", "dup 0.9 fail changes",
		// goalie check doesn't build a rule that is off, so it doesn't reject its limit either.
		"many-arguments 4 warn changes", "many-arguments -1 off changes",
	)
	if got := client.rules(dir); got != want {
		t.Errorf("rules =\n%s\nwant\n%s", got, want)
	}
}

func TestServeRulesReadsTheGoaliercNearestToDirInEachWorktree(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		".goalierc":     `{"rules": {"long-function": {"limit": 5}}}`,
		"svc/.goalierc": `{"rules": {"long-function": {"limit": 7}}}`,
		"svc/deep/a.go": "package deep\n",
	})
	worktree := filepath.Join(t.TempDir(), "wt")
	git(t, dir, "worktree", "add", "-b", "feature", worktree)
	write(t, worktree, "svc/.goalierc", `{"rules": {"long-function": {"limit": 9}}}`)
	client := startServe(t, dir)
	client.initialize()

	for _, tc := range []struct{ dir, limit string }{
		{dir, "5"},
		{filepath.Join(dir, "svc", "deep"), "7"},
		{filepath.Join(worktree, "svc", "deep"), "9"},
	} {
		want := withRules("long-function 80", "long-function "+tc.limit)
		if got := client.rules(tc.dir); got != want {
			t.Errorf("%s: rules =\n%s\nwant\n%s", tc.dir, got, want)
		}
	}
}

func TestServeRulesRejectsADirThatCheckRejects(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n"})
	other := gitRepo(t, map[string]string{"a.go": "package a\n"})
	client := startServe(t, dir)
	client.initialize()

	for _, bad := range []string{"", "svc", other} {
		if result := client.callTool("rules", map[string]any{"dir": bad}); !result.IsError {
			t.Errorf("dir %q: rules = %s, want an error", bad, result.Content[0].Text)
		}
	}
}

func TestServeRulesGivesTheErrorOfGoalieCheckForABadGoalierc(t *testing.T) {
	for name, goalierc := range map[string]string{
		"not JSON":       `{"rules": `,
		"unknown rule":   `{"rules": {"long-functions": {"limit": 5}}}`,
		"negative limit": `{"rules": {"long-function": {"limit": -1}}}`,
		"bad threshold":  `{"rules": {"dup": {"threshold": 2}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := gitRepo(t, map[string]string{".goalierc": goalierc, "a.go": "package a\n"})
			code, _, stderr := runGoalie(t, dir, "check", "--all")
			want := strings.TrimSuffix(strings.TrimPrefix(stderr, "goalie: "), "\n")
			if code != 2 || want == "" {
				t.Fatalf("goalie check = %d, %q, want 2 and an error", code, stderr)
			}
			client := startServe(t, dir)
			client.initialize()

			result := client.callTool("rules", map[string]any{"dir": dir})
			if !result.IsError || result.Content[0].Text != want {
				t.Errorf("rules = %+v, want the error %q", result, want)
			}
		})
	}
}
