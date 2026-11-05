package godeps_test

import (
	"strings"
	"testing"
)

func smallTree(t *testing.T, config string) string {
	t.Helper()
	return writeTree(t, map[string]string{"go.mod": goMod, "a/a.go": "package a\n", ".goalierc": config})
}

func wantError(t *testing.T, dir, want string, args ...string) {
	t.Helper()
	stdout, stderr := run(t, dir, 2, args...)
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	if !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want %q in it", stderr, want)
	}
}

func TestBadFlagsAreErrors(t *testing.T) {
	tests := []struct {
		name, want string
		args       []string
	}{
		{"unknown flag", "nope", []string{"--nope"}},
		{"bad format", "--format", []string{"--format", "xml"}},
		{"text is not a format here", "--format", []string{"--format", "text"}},
		{"bad skip pattern", "!keep.go", []string{"--format", "json", "--skip-files", "!keep.go"}},
		{"bad shared pattern", "!keep", []string{"--format", "json", "--shared", "!keep"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wantError(t, smallTree(t, "{}"), test.want, test.args...)
		})
	}
}

func TestBadSettingsAreErrors(t *testing.T) {
	tests := []struct{ name, config, want string }{
		{"not JSON", "oops", ".goalierc"},
		{"unknown godeps setting", `{"godeps": {"titel": "x"}}`, "titel"},
		{"wrong type", `{"godeps": {"foreign": "x"}}`, ".goalierc"},
		{"bad format", `{"godeps": {"format": "xml"}}`, ".goalierc"},
		{"bad omit pattern", `{"godeps": {"format": "json", "omit": ["!x"]}}`, ".goalierc"},
		{"bad repo pattern", `{"skipFiles": ["!x"], "godeps": {"format": "json"}}`, ".goalierc"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wantError(t, smallTree(t, test.config), test.want)
		})
	}
}

func TestAFlagCanReplaceABadFormatSetting(t *testing.T) {
	run(t, smallTree(t, `{"godeps": {"format": "xml"}}`), 0, "--format", "json")
}

func TestAGoFileThatDoesNotParseIsAnError(t *testing.T) {
	dir := writeTree(t, map[string]string{"go.mod": goMod, "a/a.go": "package a\n\nfunc broken( {\n"})
	wantError(t, dir, "a/a.go:3:", "--format", "json")
}

func TestABrokenFunctionBodyIsAnError(t *testing.T) {
	dir := writeTree(t, map[string]string{"go.mod": goMod, "a/a.go": "package a\n\nfunc f() {\n\tif\n}\n"})
	wantError(t, dir, "a/a.go:", "--format", "json")
}

func TestAMissingGoModIsAnError(t *testing.T) {
	dir := writeTree(t, map[string]string{"a/a.go": "package a\n"})
	wantError(t, dir, "go.mod", "--format", "json")
}

func TestOnlyTheRunDirectoryIsSearchedForGoMod(t *testing.T) {
	dir := writeTree(t, map[string]string{"go.mod": goMod, "sub/a.go": "package a\n"})
	wantError(t, dir+"/sub", "go.mod", "--format", "json")
}

func TestAGoModWithNoModuleLineIsAnError(t *testing.T) {
	tests := []struct{ name, content string }{
		{"empty", ""},
		{"only go", "go 1.25\n"},
		{"commented out", "// module example.com/shop\n"},
		{"no path", "module\n"},
		{"an empty block", "module (\n)\n"},
		{"a block on one line", "module (example.com/shop)\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{"go.mod": test.content, "a/a.go": "package a\n"})
			wantError(t, dir, "go.mod", "--format", "json")
		})
	}
}

func TestTheModuleLineIsReadInEveryForm(t *testing.T) {
	tests := []struct{ name, content string }{
		{"after a comment", "// Deprecated: use another.\nmodule example.com/shop\n"},
		{"with a comment", "module example.com/shop // the shop\n"},
		{"quoted", "module \"example.com/shop\"\n"},
		{"raw quoted", "module `example.com/shop`\n"},
		{"indented with Windows line ends", "\tmodule example.com/shop\r\n"},
		{"a block", "module (\n\texample.com/shop\n)\n\ngo 1.25\n"},
		{"a block with no space", "module(\n\texample.com/shop\n)\n"},
		{"a block with comments and blank lines", "module ( // the shop\n\n\t// why\n\t\"example.com/shop\" // here\n)\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{"go.mod": test.content, "a/a.go": "package a\n"})
			if got := readModel(t, dir); got.Module != "example.com/shop" {
				t.Errorf("module = %q", got.Module)
			}
		})
	}
}

func TestAPathOutsideTheRunDirectoryIsAnError(t *testing.T) {
	dir := writeTree(t, map[string]string{"repo/go.mod": goMod, "other/a.go": "package a\n"})
	wantError(t, dir+"/repo", "../other", "--format", "json", "../other")
}

func TestHTMLIsTheDefault(t *testing.T) {
	stdout, _ := run(t, smallTree(t, "{}"), 0)
	if !strings.HasPrefix(stdout, "<!doctype html>") {
		t.Errorf("stdout = %q; want a page", stdout)
	}
}

func TestTheFormatSettingApplies(t *testing.T) {
	stdout, _ := run(t, smallTree(t, `{"godeps": {"format": "json"}}`), 0)
	if !strings.Contains(stdout, `"version": 1`) {
		t.Errorf("stdout = %q; want the model", stdout)
	}
}

func TestAFormatFlagReplacesTheSetting(t *testing.T) {
	stdout, _ := run(t, smallTree(t, `{"godeps": {"format": "json"}}`), 0, "--format", "html")
	if !strings.HasPrefix(stdout, "<!doctype html>") {
		t.Errorf("stdout = %q; want a page", stdout)
	}
}

func TestEveryGodepsSettingIsAccepted(t *testing.T) {
	config := `{"godeps": {
		"format": "json", "archGoReport": "", "crapReport": "", "dryReport": "",
		"editorURL": "idea://open?file={file}&line={line}", "title": "Shop",
		"foreign": ["example.com/lib"], "omit": ["gen/**"], "order": ["a"]
	}}`
	run(t, smallTree(t, config), 0)
}

func TestEveryFlagIsAccepted(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod, "a/a.go": "package a\n",
		"arch.json": "{}\n", "crap.json": "{\"findings\": []}\n", "dry.json": "{\"findings\": []}\n",
	})
	run(t, dir, 0, "--format", "json", "--arch-go-report", "arch.json", "--crap-report", "crap.json",
		"--dry-report", "dry.json", "--editor-url", "file://{file}", "--title", "Shop",
		"--skip-files", "x.go", "--skip-tags", "x", "--shared", "a/**")
}

func TestTheTitleComesFromTheFlagThenTheSettingThenTheModule(t *testing.T) {
	tests := []struct {
		name, config, want string
		args               []string
	}{
		{"module", `{}`, "example.com/shop", nil},
		{"setting", `{"godeps": {"title": "From file"}}`, "From file", nil},
		{"flag", `{"godeps": {"title": "From file"}}`, "From flag", []string{"--title", "From flag"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := readModel(t, smallTree(t, test.config), test.args...); got.Title != test.want {
				t.Errorf("title = %q, want %q", got.Title, test.want)
			}
		})
	}
}

func TestRepoSettingsChooseTheFiles(t *testing.T) {
	tagged := "//go:build legacy\n\npackage b\n"
	tests := []struct {
		name, config string
		args         []string
	}{
		{"paths setting", `{"paths": ["a"]}`, nil},
		{"path argument", `{}`, []string{"a"}},
		{"skip flags", `{}`, []string{"--skip-tags", "legacy", "--skip-files", "c/**"}},
		{"skip settings", `{"skipTags": ["legacy"], "skipFiles": ["c/**"]}`, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{
				"go.mod": goMod, "a/a.go": "package a\n", "b/b.go": tagged, "c/c.go": "package c\n", ".goalierc": test.config,
			})
			wantClasses(t, readModel(t, dir, test.args...), class{"example.com/shop/a", "a", "a"})
		})
	}
}

func TestTheIgnoreFileApplies(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod, "a/a.go": "package a\n", "b/b.go": "package b\n", ".goalieignore": "b/**\n",
	})
	wantClasses(t, readModel(t, dir), class{"example.com/shop/a", "a", "a"})
}

func TestHelpPrintsTheFlagsAndExitsCleanly(t *testing.T) {
	stdout, _ := run(t, t.TempDir(), 0, "--help")
	for _, want := range []string{
		"--format", "--arch-go-report", "--crap-report", "--dry-report", "--editor-url", "--title",
		"--skip-files", "--skip-tags", "--shared", ".goalierc", "Exit codes",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q; want %q listed", stdout, want)
		}
	}
}

func TestHelpWorksWithABadConfig(t *testing.T) {
	run(t, smallTree(t, "oops"), 0, "--help")
}
