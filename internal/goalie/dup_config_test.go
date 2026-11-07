package goalie_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// configTree holds one duplicate pair. sumFunc has 12 lines.
func configTree(t *testing.T, config string) string {
	t.Helper()
	return writeTree(t, map[string]string{
		".goalierc": config,
		"one.go":    "package a\n\n" + sumFunc("One"),
		"two.go":    "package a\n\n" + sumFunc("Two"),
	})
}

func wantCode(t *testing.T, dir string, want int, args ...string) (stdout, stderr string) {
	t.Helper()
	code, stdout, stderr := runDup(t, dir, args...)
	if code != want {
		t.Errorf("exit code = %d, want %d; stdout = %q, stderr = %q", code, want, stdout, stderr)
	}
	return stdout, stderr
}

func TestConfigPathsLimitTheScan(t *testing.T) {
	dir := writeTree(t, map[string]string{
		".goalierc":             `{"paths": ["products/billing", "pkg"]}`,
		"products/billing/a.go": "package a\n\n" + sumFunc("One"),
		"pkg/b.go":              "package b\n\n" + sumFunc("Two"),
		"products/search/c.go":  "package c\n\n" + sumFunc("Three"),
	})
	stdout, _ := wantCode(t, dir, 1)
	if strings.Contains(stdout, "products/search") || !strings.Contains(stdout, "products/billing/a.go") {
		t.Errorf("stdout = %q; want only the listed directories", stdout)
	}
}

func TestAMissingConfigPathIsAnError(t *testing.T) {
	wantCode(t, configTree(t, `{"paths": ["missing"]}`), 2)
}

func TestConfigSetsEachSetting(t *testing.T) {
	tests := []struct {
		name, config string
		wantCode     int
	}{
		{"no settings", `{}`, 1},
		{"minLines", `{"rules": {"dup": {"minLines": 13}}}`, 0},
		{"minNodes", `{"rules": {"dup": {"minNodes": 1000}}}`, 0},
		{"mode", `{"rules": {"dup": {"mode": "warn"}}}`, 0},
		{"skipFiles", `{"skipFiles": ["two.go"]}`, 0},
		{"null means not set", `{"rules": {"dup": {"minLines": null}}, "skipFiles": null}`, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wantCode(t, configTree(t, test.config), test.wantCode)
		})
	}
}

func TestConfigSetsTheThreshold(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"one.go": "package a\n\n" + sumFunc("One"),
		"two.go": "package a\n\n" + strings.Replace(sumFunc("Two"), "value > 0", "value > 0 && value < 9", 1),
	})
	wantCode(t, dir, 0)
	write(t, dir, ".goalierc", `{"rules": {"dup": {"threshold": 0.5}}}`)
	wantCode(t, dir, 1)
	wantCode(t, dir, 0, "--limit", "0.82")
}

func TestConfigSetsTheWeakLimits(t *testing.T) {
	dir := mappingTree(t)
	write(t, dir, ".goalierc", `{"rules": {"dup": {"weakAnchors": 1}}}`)
	if _, stderr := wantCode(t, dir, 0); stderr != "goalie: 1 weak finding dropped\n" {
		t.Errorf("stderr = %q; want one finding dropped", stderr)
	}
	write(t, dir, ".goalierc", `{"rules": {"dup": {"weakAnchors": 1, "weakLiterals": 0}}}`)
	wantCode(t, dir, 1)
}

func TestConfigIncludesTests(t *testing.T) {
	dir := writeTree(t, map[string]string{
		".goalierc":   `{"rules": {"dup": {"includeTests": true}}}`,
		"one.go":      "package a\n\n" + sumFunc("One"),
		"two_test.go": "package a\n\n" + sumFunc("Two"),
	})
	wantCode(t, dir, 1)
}

func TestConfigSkipsTags(t *testing.T) {
	dir := configTree(t, `{"skipTags": ["integration"]}`)
	write(t, dir, "two.go", "//go:build integration\n\npackage a\n\n"+sumFunc("Two"))
	wantCode(t, dir, 0)
	write(t, dir, ".goalierc", `{"skipTags": ["other"]}`)
	wantCode(t, dir, 1)
}

func TestConfigBaseTurnsOnNewCodeMode(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		".goalierc": `{"base": "HEAD"}`,
		"one.go":    "package a\n\n" + sumFunc("One"),
		"two.go":    "package a\n\n" + sumFunc("Two"),
	})
	code, stdout, stderr := runGoalie(t, dir, "check", "dup")
	if code != 0 || stdout != "" || stderr != "goalie: dup: No new functions found.\n" {
		t.Errorf("exit code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
}

func TestTheLimitFlagReplacesTheThreshold(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"a flag", []string{"--limit", "0.82"}},
		{"a flag equal to the default still counts", []string{"--limit=0.82"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wantCode(t, configTree(t, `{"rules": {"dup": {"threshold": 1}}}`), 1, test.args...)
		})
	}
}

func TestAllSkipPatternsApplyTogether(t *testing.T) {
	dir := writeTree(t, map[string]string{
		".goalierc":     `{"skipFiles": ["one.go", "three.go"]}`,
		".goalieignore": "two.go\n",
		"one.go":        "package a\n\n" + sumFunc("One"),
		"two.go":        "package a\n\n" + sumFunc("Two"),
		"three.go":      "package a\n\n" + sumFunc("Three"),
		"kept.go":       "package a\n\n" + sumFunc("Kept"),
	})
	wantReport(t, dir, 0, "")
}

func TestConfigIsReadOnlyFromTheRunDirectory(t *testing.T) {
	dir := configTree(t, `{"rules": {"dup": {"mode": "warn"}}}`)
	write(t, dir, "pkg/one.go", "package a\n\n"+sumFunc("One"))
	write(t, dir, "pkg/two.go", "package a\n\n"+sumFunc("Two"))
	wantCode(t, filepath.Join(dir, "pkg"), 1)

	nested := writeTree(t, map[string]string{
		"pkg/.goalierc": `{"rules": {"dup": {"mode": "warn"}}}`,
		"pkg/one.go":    "package a\n\n" + sumFunc("One"),
		"pkg/two.go":    "package a\n\n" + sumFunc("Two"),
	})
	wantCode(t, nested, 1)
}

func TestBadConfigsAreErrors(t *testing.T) {
	tests := []struct{ name, config, want string }{
		{"empty file", "", ".goalierc"},
		{"not JSON", "weakAnchors: 1\n", ".goalierc:1:"},
		{"syntax error names its line", "{\n  \"paths\": [],\n  oops\n}\n", ".goalierc:3:"},
		{"cut off", `{"minLines": 9`, ".goalierc"},
		{"content after the object", `{} {}`, ".goalierc"},
		{"not an object", `[]`, ".goalierc"},
		{"unknown setting", `{"weakLabels": 1}`, "weakLabels"},
		{"unknown dup setting", `{"rules": {"dup": {"weakLabels": 1}}}`, "weakLabels"},
		{"unknown crap setting", `{"rules": {"crap": {"limits": 1}}}`, "limits"},
		{"unknown godeps setting", `{"godeps": {"titel": "x"}}`, "titel"},
		{"dup setting at the top level", `{"threshold": 0.9}`, "threshold"},
		{"wrong type names its line", "{\n  \"paths\": \"a\"\n}\n", ".goalierc:2:"},
		{"fraction for a count", `{"rules": {"dup": {"minLines": 1.5}}}`, ".goalierc"},
		{"threshold too high", `{"rules": {"dup": {"threshold": 2}}}`, "threshold"},
		{"negative lines", `{"rules": {"dup": {"minLines": -1}}}`, "minLines"},
		{"negative nodes", `{"rules": {"dup": {"minNodes": -1}}}`, "minNodes"},
		{"negative anchors", `{"rules": {"dup": {"weakAnchors": -1}}}`, "weakAnchors"},
		{"weak literals too high", `{"rules": {"dup": {"weakLiterals": 1.5}}}`, "weakLiterals"},
		{"bad pattern", `{"skipFiles": ["!keep.go"]}`, ".goalierc"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr := runDup(t, configTree(t, test.config), "--warn-only")
			if code != 2 || stdout != "" || !strings.Contains(stderr, test.want) {
				t.Errorf("exit code = %d, stdout = %q, stderr = %q; want exit 2 with %q", code, stdout, stderr, test.want)
			}
		})
	}
}

func TestAFlagDoesNotHideABadConfigValue(t *testing.T) {
	wantCode(t, configTree(t, `{"rules": {"dup": {"minLines": -1}}}`), 2, "--limit", "0.82")
}

func TestAFlagCanReplaceABadConfigValue(t *testing.T) {
	wantCode(t, configTree(t, `{"rules": {"dup": {"threshold": 2}}}`), 1, "--limit", "0.82")
}

func TestAnUnreadableConfigIsAnError(t *testing.T) {
	dir := writeTree(t, map[string]string{"one.go": "package a\n\n" + sumFunc("One")})
	if err := os.Mkdir(filepath.Join(dir, ".goalierc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, stderr := wantCode(t, dir, 2); !strings.Contains(stderr, ".goalierc") {
		t.Errorf("stderr = %q; want the file named", stderr)
	}
}

func TestHelpWorksWithABadConfig(t *testing.T) {
	stdout, _ := wantCode(t, configTree(t, "oops"), 0, "--help")
	if !strings.Contains(stdout, ".goalierc") {
		t.Errorf("stdout = %q; want .goalierc listed", stdout)
	}
}

func TestGoalieAcceptsTheSettingsOfGodeps(t *testing.T) {
	wantCode(t, configTree(t, `{"godeps": {"title": "x"}}`), 1)
}
