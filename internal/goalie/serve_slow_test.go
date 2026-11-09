package goalie_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/n-valchev/goalie/internal/goalie"
)

func readSlowLog(t *testing.T, dir string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, ".goalie", "slow.log"))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestServeLogsASlowCallAndStillAnswers(t *testing.T) {
	// Before startServe, so that the server has stopped when the limit goes back.
	goalie.SetSlowCall(t, 0)
	dir := gitRepo(t, map[string]string{"a.go": "package a\n\n" + sumFunc("Total"), "b.go": "package a\n"})
	client := startServe(t, dir)
	client.initialize()

	result := client.callTool("check", map[string]any{"dir": dir, "scope": "all"})
	if result.IsError || !strings.HasPrefix(result.Content[0].Text, `{"results":`) {
		t.Fatalf("check = %+v, want its answer", result)
	}
	line := `\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\S* check took \d+\.\ds, read 2 files\n`
	if got := readSlowLog(t, dir); !regexp.MustCompile(`^` + line + `$`).MatchString(got) {
		t.Errorf("slow.log = %q, want one line that matches %q", got, line)
	}
	client.callTool("check", map[string]any{"dir": dir, "scope": "all"})
	if got := readSlowLog(t, dir); !regexp.MustCompile(`^(` + line + `){2}$`).MatchString(got) {
		t.Errorf("after a second call, slow.log = %q, want two lines", got)
	}
}

func TestServeWritesAGitignoreThatHidesTheLog(t *testing.T) {
	goalie.SetSlowCall(t, 0)
	dir := gitRepo(t, map[string]string{"a.go": "package a\n"})
	client := startServe(t, dir)
	client.initialize()

	client.callTool("rules", map[string]any{"dir": dir})
	content, err := os.ReadFile(filepath.Join(dir, ".goalie", ".gitignore"))
	if err != nil || string(content) != "*\n" {
		t.Errorf(".goalie/.gitignore = %q (%v), want %q", content, err, "*\n")
	}
}

func TestServeLogsNextToTheNearestGoalierc(t *testing.T) {
	goalie.SetSlowCall(t, 0)
	dir := gitRepo(t, map[string]string{"svc/.goalierc": "{}", "svc/a.go": "package a\n"})
	client := startServe(t, dir)
	client.initialize()

	client.callTool("rules", map[string]any{"dir": filepath.Join(dir, "svc")})
	if got := readSlowLog(t, filepath.Join(dir, "svc")); !strings.Contains(got, " rules took ") {
		t.Errorf("svc/.goalie/slow.log = %q, want a line for rules", got)
	}
}

func TestServeLogsNothingForACallUnderTheLimit(t *testing.T) {
	goalie.SetSlowCall(t, time.Hour)
	dir := gitRepo(t, map[string]string{"a.go": "package a\n\n" + sumFunc("Total")})
	client := startServe(t, dir)
	client.initialize()

	client.callTool("check", map[string]any{"dir": dir, "scope": "all"})
	if _, err := os.Stat(filepath.Join(dir, ".goalie")); !os.IsNotExist(err) {
		t.Errorf(".goalie: %v, want no folder", err)
	}
}

func TestServeAnswersWhenTheLogCannotBeWritten(t *testing.T) {
	goalie.SetSlowCall(t, 0)
	dir := gitRepo(t, map[string]string{"a.go": "package a\n"})
	// A file where the folder must go.
	write(t, dir, ".goalie", "not a folder\n")
	client := startServe(t, dir)
	client.initialize()

	result := client.callTool("rules", map[string]any{"dir": dir})
	if result.IsError || !strings.HasPrefix(result.Content[0].Text, `{"rules":`) {
		t.Fatalf("rules = %+v, want its answer", result)
	}
	if got := client.stderr.String(); !strings.Contains(got, "goalie: can't log the slow call: ") {
		t.Errorf("stderr = %q, want a message about the slow-call log", got)
	}
}
