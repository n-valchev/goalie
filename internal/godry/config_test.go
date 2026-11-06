package godry_test

import (
	"encoding/json"
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
	code, stdout, stderr := runGodry(t, dir, args...)
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

func TestPathsOnTheCommandLineReplaceConfigPaths(t *testing.T) {
	dir := writeTree(t, map[string]string{
		".goalierc": `{"paths": ["a", "b"]}`,
		"a/a.go":    "package a\n\n" + sumFunc("One"),
		"b/b.go":    "package b\n\n" + sumFunc("Two"),
		"c/c.go":    "package c\n\n" + joinFunc("Three"),
	})
	wantReport(t, dir, 0, "No duplicates found.\n", "c")
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
		{"minLines", `{"godry": {"minLines": 13}}`, 0},
		{"minNodes", `{"godry": {"minNodes": 1000}}`, 0},
		{"warnOnly", `{"godry": {"warnOnly": true}}`, 0},
		{"skipFiles", `{"skipFiles": ["two.go"]}`, 0},
		{"null means not set", `{"godry": {"minLines": null}, "skipFiles": null}`, 1},
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
	write(t, dir, ".goalierc", `{"godry": {"threshold": 0.5}}`)
	wantCode(t, dir, 1)
	wantCode(t, dir, 0, "--threshold", "0.82")
}

func TestConfigSetsTheFormat(t *testing.T) {
	stdout, _ := wantCode(t, configTree(t, `{"godry": {"format": "json"}}`), 1)
	var report struct {
		Findings []json.RawMessage `json:"findings"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil || len(report.Findings) != 1 {
		t.Errorf("stdout = %q, err = %v; want JSON with one finding", stdout, err)
	}
}

func TestConfigSetsTheWeakLimits(t *testing.T) {
	dir := mappingTree(t)
	write(t, dir, ".goalierc", `{"godry": {"weakAnchors": 1}}`)
	if _, stderr := wantCode(t, dir, 0); stderr != "godry: 1 weak finding dropped\n" {
		t.Errorf("stderr = %q; want one finding dropped", stderr)
	}
	write(t, dir, ".goalierc", `{"godry": {"weakAnchors": 1, "weakLiterals": 0}}`)
	wantCode(t, dir, 1)
}

func TestConfigIncludesTests(t *testing.T) {
	dir := writeTree(t, map[string]string{
		".goalierc":   `{"godry": {"includeTests": true}}`,
		"one.go":      "package a\n\n" + sumFunc("One"),
		"two_test.go": "package a\n\n" + sumFunc("Two"),
	})
	wantCode(t, dir, 1)
}

func TestConfigSkipsTags(t *testing.T) {
	dir := configTree(t, `{"skipTags": ["integration"]}`)
	write(t, dir, "two.go", "//go:build integration\n\npackage a\n\n"+sumFunc("Two"))
	wantCode(t, dir, 0)
	wantCode(t, dir, 1, "--skip-tags", "other")
}

func TestConfigBaseTurnsOnNewCodeMode(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		".goalierc": `{"godry": {"base": "HEAD"}}`,
		"one.go":    "package a\n\n" + sumFunc("One"),
		"two.go":    "package a\n\n" + sumFunc("Two"),
	})
	wantReport(t, dir, 0, "No new functions found.\n")
}

func TestFlagsReplaceConfigSettings(t *testing.T) {
	tests := []struct {
		name, config string
		args         []string
		wantCode     int
	}{
		{"min lines", `{"godry": {"minLines": 13}}`, []string{"--min-lines", "9"}, 1},
		{"min nodes", `{"godry": {"minNodes": 1000}}`, []string{"--min-nodes", "20"}, 1},
		{"warn only turned off", `{"godry": {"warnOnly": true}}`, []string{"--warn-only=false"}, 1},
		{"a flag equal to its default still counts", `{"godry": {"minLines": 13}}`, []string{"--min-lines=9"}, 1},
		{"a flag after a path", `{"godry": {"minLines": 13}}`, []string{".", "--min-lines", "9"}, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wantCode(t, configTree(t, test.config), test.wantCode, test.args...)
		})
	}
}

func TestAWeakAnchorsFlagOfZeroTurnsOffTheConfigRule(t *testing.T) {
	dir := mappingTree(t)
	write(t, dir, ".goalierc", `{"godry": {"weakAnchors": 1}}`)
	wantCode(t, dir, 1, "--weak-anchors", "0")
}

func TestAFormatFlagReplacesTheConfigFormat(t *testing.T) {
	stdout, _ := wantCode(t, configTree(t, `{"godry": {"format": "json"}}`), 1, "--format", "text")
	if !strings.Contains(stdout, "duplicates") {
		t.Errorf("stdout = %q; want the text format", stdout)
	}
}

func TestAllSkipPatternsApplyTogether(t *testing.T) {
	dir := writeTree(t, map[string]string{
		".goalierc":     `{"skipFiles": ["one.go"]}`,
		".goalieignore": "two.go\n",
		"one.go":        "package a\n\n" + sumFunc("One"),
		"two.go":        "package a\n\n" + sumFunc("Two"),
		"three.go":      "package a\n\n" + sumFunc("Three"),
		"kept.go":       "package a\n\n" + sumFunc("Kept"),
	})
	wantReport(t, dir, 0, "No duplicates found.\n", "--skip-files", "three.go")
}

func TestConfigIsReadOnlyFromTheRunDirectory(t *testing.T) {
	dir := configTree(t, `{"godry": {"warnOnly": true}}`)
	write(t, dir, "pkg/one.go", "package a\n\n"+sumFunc("One"))
	write(t, dir, "pkg/two.go", "package a\n\n"+sumFunc("Two"))
	wantCode(t, filepath.Join(dir, "pkg"), 1)

	nested := writeTree(t, map[string]string{
		"pkg/.goalierc": `{"godry": {"warnOnly": true}}`,
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
		{"unknown godry setting", `{"godry": {"weakLabels": 1}}`, "weakLabels"},
		{"unknown gocrap setting", `{"gocrap": {"limits": 1}}`, "limits"},
		{"unknown godeps setting", `{"godeps": {"titel": "x"}}`, "titel"},
		{"unknown rule", `{"rules": {"dupe": {}}}`, "dupe"},
		{"unknown mode", `{"rules": {"dup": {"mode": "warning"}}}`, "warning"},
		{"godry setting at the top level", `{"threshold": 0.9}`, "threshold"},
		{"wrong type names its line", "{\n  \"paths\": \"a\"\n}\n", ".goalierc:2:"},
		{"fraction for a count", `{"godry": {"minLines": 1.5}}`, ".goalierc"},
		{"empty base", `{"godry": {"base": ""}}`, "base"},
		{"bad format", `{"godry": {"format": "xml"}}`, ".goalierc"},
		{"threshold too high", `{"godry": {"threshold": 2}}`, "--threshold"},
		{"negative lines", `{"godry": {"minLines": -1}}`, "--min-lines"},
		{"negative nodes", `{"godry": {"minNodes": -1}}`, "--min-nodes"},
		{"negative anchors", `{"godry": {"weakAnchors": -1}}`, "--weak-anchors"},
		{"weak literals too high", `{"godry": {"weakLiterals": 1.5}}`, "--weak-literals"},
		{"bad pattern", `{"skipFiles": ["!keep.go"]}`, ".goalierc"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr := runGodry(t, configTree(t, test.config), "--warn-only")
			if code != 2 || stdout != "" || !strings.Contains(stderr, test.want) {
				t.Errorf("exit code = %d, stdout = %q, stderr = %q; want exit 2 with %q", code, stdout, stderr, test.want)
			}
		})
	}
}

func TestAFlagDoesNotHideABadConfigValue(t *testing.T) {
	wantCode(t, configTree(t, `{"godry": {"threshold": 2}}`), 2, "--min-lines", "9")
}

func TestAFlagCanReplaceABadConfigValue(t *testing.T) {
	wantCode(t, configTree(t, `{"godry": {"threshold": 2}}`), 1, "--threshold", "0.82")
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

func TestGodryAcceptsButIgnoresTheSettingsOfGoalie(t *testing.T) {
	wantCode(t, configTree(t, `{"base": "no-such-rev", "rules": {"dup": {"mode": "off", "minLines": 100}}}`), 1)
}

func TestGodryAcceptsTheSettingsOfOtherTools(t *testing.T) {
	wantCode(t, configTree(t, `{"gocrap": {"limit": 30, "warnOnly": true}, "godeps": {"title": "x"}}`), 1)
}
