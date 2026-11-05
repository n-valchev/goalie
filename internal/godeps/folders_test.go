package godeps_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

type box struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Parent       string `json:"parent"`
	Shared       bool   `json:"shared"`
	PackageCount int    `json:"packageCount"`
}

type placedClass struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Package string `json:"package"`
	Foreign bool   `json:"foreign"`
}

type folderModel struct {
	Order    []string      `json:"order"`
	Packages []box         `json:"packages"`
	Classes  []placedClass `json:"classes"`
	Edges    []edge        `json:"edges"`
}

func readFolders(t *testing.T, dir string, args ...string) folderModel {
	t.Helper()
	stdout, _ := run(t, dir, 0, append([]string{"--format", "json"}, args...)...)
	var result folderModel
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not a model: %v\n%s", err, stdout)
	}
	return result
}

func wantBoxes(t *testing.T, got folderModel, want ...box) {
	t.Helper()
	if !slices.Equal(got.Packages, want) {
		t.Errorf("packages = %+v, want %+v", got.Packages, want)
	}
}

func wantPlaced(t *testing.T, got folderModel, want ...placedClass) {
	t.Helper()
	if !slices.Equal(got.Classes, want) {
		t.Errorf("classes = %+v, want %+v", got.Classes, want)
	}
}

func TestEachFolderOnAClassPathIsABoxThatCountsTheClassesBelowIt(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":               goMod,
		"billing/billing.go":   "package billing\n",
		"billing/invoice/a.go": "package invoice\n",
		"billing/tax/eu/eu.go": "package eu\n",
		"web/server.go":        "package web\n",
	})
	got := readFolders(t, dir)
	wantBoxes(t, got,
		box{ID: "billing", Name: "billing", PackageCount: 3},
		box{ID: "billing/invoice", Name: "invoice", Parent: "billing", PackageCount: 1},
		box{ID: "billing/tax", Name: "tax", Parent: "billing", PackageCount: 1},
		box{ID: "billing/tax/eu", Name: "eu", Parent: "billing/tax", PackageCount: 1},
		box{ID: "web", Name: "web", PackageCount: 1},
	)
	wantPlaced(t, got,
		placedClass{ID: "example.com/shop/billing", Name: "billing", Path: "billing", Package: "billing"},
		placedClass{ID: "example.com/shop/billing/invoice", Name: "invoice", Path: "billing/invoice", Package: "billing/invoice"},
		placedClass{ID: "example.com/shop/billing/tax/eu", Name: "eu", Path: "billing/tax/eu", Package: "billing/tax/eu"},
		placedClass{ID: "example.com/shop/web", Name: "web", Path: "web", Package: "web"},
	)
}

func TestAClassAtTheRootHasNoBox(t *testing.T) {
	dir := writeTree(t, map[string]string{"go.mod": goMod, "shop.go": "package shop\n"})
	got := readFolders(t, dir)
	wantBoxes(t, got)
	wantPlaced(t, got, placedClass{ID: "example.com/shop", Name: "shop", Path: "."})
}

func TestSharedClassesHangUnderOneTopLevelBox(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":         goMod,
		"web/web.go":     "package web\n",
		"lib/auth/a.go":  "package auth\n",
		"lib/log/log.go": "package log\n",
		"lib/plain/p.go": "package plain\n",
		".goalierc":      `{"shared": ["lib/auth/**", "lib/log/**"]}`,
	})
	got := readFolders(t, dir)
	wantBoxes(t, got,
		box{ID: ":shared", Name: "shared", Shared: true, PackageCount: 2},
		box{ID: ":shared/lib", Name: "lib", Parent: ":shared", Shared: true, PackageCount: 2},
		box{ID: ":shared/lib/auth", Name: "auth", Parent: ":shared/lib", Shared: true, PackageCount: 1},
		box{ID: ":shared/lib/log", Name: "log", Parent: ":shared/lib", Shared: true, PackageCount: 1},
		box{ID: "lib", Name: "lib", PackageCount: 1},
		box{ID: "lib/plain", Name: "plain", Parent: "lib", PackageCount: 1},
		box{ID: "web", Name: "web", PackageCount: 1},
	)
	wantPlaced(t, got,
		placedClass{ID: "example.com/shop/lib/auth", Name: "auth", Path: "lib/auth", Package: ":shared/lib/auth"},
		placedClass{ID: "example.com/shop/lib/log", Name: "log", Path: "lib/log", Package: ":shared/lib/log"},
		placedClass{ID: "example.com/shop/lib/plain", Name: "plain", Path: "lib/plain", Package: "lib/plain"},
		placedClass{ID: "example.com/shop/web", Name: "web", Path: "web", Package: "web"},
	)
}

func TestOneMatchingFileMakesTheWholeClassShared(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod, "a/a.go": "package a\n", "a/common.go": "package a\n",
	})
	got := readFolders(t, dir, "--shared", "a/common.go")
	wantPlaced(t, got, placedClass{ID: "example.com/shop/a", Name: "a", Path: "a", Package: ":shared/a"})
}

func TestASharedClassAtTheRootSitsInTheSharedBox(t *testing.T) {
	dir := writeTree(t, map[string]string{"go.mod": goMod, "shop.go": "package shop\n"})
	got := readFolders(t, dir, "--shared", "shop.go")
	wantBoxes(t, got, box{ID: ":shared", Name: "shared", Shared: true, PackageCount: 1})
	wantPlaced(t, got, placedClass{ID: "example.com/shop", Name: "shop", Path: ".", Package: ":shared"})
}

func TestTheSharedFlagReplacesTheSetting(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod, "a/a.go": "package a\n", "b/b.go": "package b\n", ".goalierc": `{"shared": ["a/**"]}`,
	})
	wantBoxes(t, readFolders(t, dir, "--shared", "b/**"),
		box{ID: ":shared", Name: "shared", Shared: true, PackageCount: 1},
		box{ID: ":shared/b", Name: "b", Parent: ":shared", Shared: true, PackageCount: 1},
		box{ID: "a", Name: "a", PackageCount: 1},
	)
}

func TestAForeignModuleIsOneClassThatTakesImportsAtOrBelowIt(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a/a.go":    "package a\n\nimport (\n\t_ \"example.com/lib\"\n\t_ \"example.com/lib/sub/deep\"\n)\n",
		"b/b.go":    "package b\n\nimport _ \"example.com/lib/sub\"\n",
		"z/z.go":    "package z\n\nimport _ \"example.com/shop/a\"\n",
		".goalierc": `{"godeps": {"foreign": ["example.com/lib"]}}`,
	})
	got := readFolders(t, dir)
	wantPlaced(t, got,
		placedClass{ID: "example.com/lib", Name: "example.com/lib", Foreign: true},
		placedClass{ID: "example.com/shop/a", Name: "a", Path: "a", Package: "a"},
		placedClass{ID: "example.com/shop/b", Name: "b", Path: "b", Package: "b"},
		placedClass{ID: "example.com/shop/z", Name: "z", Path: "z", Package: "z"},
	)
	want := []edge{
		{"example.com/shop/a", "example.com/lib", "dependency"},
		{"example.com/shop/b", "example.com/lib", "dependency"},
		dependency("z", "a"),
	}
	if !slices.Equal(got.Edges, want) {
		t.Errorf("edges = %+v, want %+v", got.Edges, want)
	}
	for _, folder := range got.Packages {
		if folder.PackageCount != 1 {
			t.Errorf("box %+v counts a foreign class", folder)
		}
	}
}

func TestAForeignModuleMatchesWholePathElementsOnly(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a/a.go":    "package a\n\nimport _ \"example.com/library\"\n",
		".goalierc": `{"godeps": {"foreign": ["example.com/lib"]}}`,
	})
	got := readFolders(t, dir)
	wantPlaced(t, got, placedClass{ID: "example.com/shop/a", Name: "a", Path: "a", Package: "a"})
	if len(got.Edges) != 0 {
		t.Errorf("edges = %+v, want none", got.Edges)
	}
}

func TestAForeignModuleThatNothingImportsIsLeftOut(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a/a.go":    "package a\n",
		".goalierc": `{"godeps": {"foreign": ["example.com/lib"]}}`,
	})
	wantPlaced(t, readFolders(t, dir), placedClass{ID: "example.com/shop/a", Name: "a", Path: "a", Package: "a"})
}

func TestAnImportBelowTwoForeignModulesMakesOneEdgeToTheNearest(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a/a.go":    "package a\n\nimport _ \"example.com/lib/sub/x\"\n",
		".goalierc": `{"godeps": {"foreign": ["example.com/lib", "example.com/lib/sub"]}}`,
	})
	got := readFolders(t, dir)
	want := []edge{{"example.com/shop/a", "example.com/lib/sub", "dependency"}}
	if !slices.Equal(got.Edges, want) {
		t.Errorf("edges = %+v, want %+v", got.Edges, want)
	}
}

func TestEdgesToForeignModulesAreSortedByModule(t *testing.T) {
	// The import paths sort the other way round from the modules they belong to.
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a/a.go":    "package a\n\nimport (\n\t_ \"example.com/x-y\"\n\t_ \"example.com/x/z\"\n)\n",
		".goalierc": `{"godeps": {"foreign": ["example.com/x-y", "example.com"]}}`,
	})
	got := readFolders(t, dir)
	want := []edge{
		{"example.com/shop/a", "example.com", "dependency"},
		{"example.com/shop/a", "example.com/x-y", "dependency"},
	}
	if !slices.Equal(got.Edges, want) {
		t.Errorf("edges = %+v, want %+v", got.Edges, want)
	}
}

func TestAForeignEntryNeverTakesTheModulesOwnPackages(t *testing.T) {
	tests := []struct{ name, foreign string }{
		{"the module", "example.com/shop"},
		{"above the module", "example.com"},
		{"a package of the module", "example.com/shop/gen"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{
				"go.mod":     goMod,
				"shop.go":    "package shop\n",
				"a/a.go":     "package a\n\nimport (\n\t_ \"example.com/shop\"\n\t_ \"example.com/shop/gen\"\n)\n",
				"gen/gen.go": "// Code generated by x. DO NOT EDIT.\n\npackage gen\n",
				".goalierc":  `{"godeps": {"foreign": ["` + test.foreign + `"]}}`,
			})
			got := readFolders(t, dir)
			wantPlaced(t, got,
				placedClass{ID: "example.com/shop", Name: "shop", Path: "."},
				placedClass{ID: "example.com/shop/a", Name: "a", Path: "a", Package: "a"},
			)
			want := []edge{{"example.com/shop/a", "example.com/shop", "dependency"}}
			if !slices.Equal(got.Edges, want) {
				t.Errorf("edges = %+v, want %+v", got.Edges, want)
			}
		})
	}
}

func TestAnOmittedClassAndItsEdgesAreGone(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":       goMod,
		"a/a.go":       "package a\n\nimport (\n\t_ \"example.com/shop/b\"\n\t_ \"example.com/shop/gen\"\n)\n",
		"b/b.go":       "package b\n",
		"gen/gen.go":   "package gen\n\nimport (\n\t_ \"example.com/lib\"\n\t_ \"example.com/shop/b\"\n)\n",
		"gen/sub/s.go": "package sub\n",
		".goalierc":    `{"godeps": {"omit": ["gen/**"], "foreign": ["example.com/lib"]}}`,
	})
	got := readFolders(t, dir)
	wantBoxes(t, got, box{ID: "a", Name: "a", PackageCount: 1}, box{ID: "b", Name: "b", PackageCount: 1})
	wantPlaced(t, got,
		placedClass{ID: "example.com/shop/a", Name: "a", Path: "a", Package: "a"},
		placedClass{ID: "example.com/shop/b", Name: "b", Path: "b", Package: "b"},
	)
	if want := []edge{dependency("a", "b")}; !slices.Equal(got.Edges, want) {
		t.Errorf("edges = %+v, want %+v", got.Edges, want)
	}
}

func TestOmitPatternsMatchTheClassPathByTheFileRules(t *testing.T) {
	tree := map[string]string{
		"go.mod":       goMod,
		"shop.go":      "package shop\n",
		"gen/g.go":     "package gen\n",
		"gen/sub/s.go": "package sub\n",
		"web/gen/w.go": "package gen\n",
		"web/web.go":   "package web\n",
	}
	tests := []struct {
		name, pattern string
		want          []string
	}{
		{"a folder and all below it", "gen/**", []string{".", "web", "web/gen"}},
		{"no slash is the last folder at any depth", "gen", []string{".", "gen/sub", "web"}},
		{"a full path is that folder alone", "web/gen", []string{".", "gen", "gen/sub", "web"}},
		{"any depth and all below", "**/gen/**", []string{".", "web"}},
		{"the root class", ".", []string{"gen", "gen/sub", "web", "web/gen"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, tree)
			write(t, dir, ".goalierc", `{"godeps": {"omit": ["`+test.pattern+`"]}}`)
			var got []string
			for _, class := range readFolders(t, dir).Classes {
				got = append(got, class.Path)
			}
			if !slices.Equal(got, test.want) {
				t.Errorf("paths = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAnOmittedFileIsNotParsed(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod, "a/a.go": "package a\n", "gen/g.go": "package gen\n\nfunc broken( {\n",
		".goalierc": `{"godeps": {"omit": ["gen/**"]}}`,
	})
	wantPlaced(t, readFolders(t, dir), placedClass{ID: "example.com/shop/a", Name: "a", Path: "a", Package: "a"})
}

func TestAnOmittedFolderStillCountsForTheSharedWarning(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod, "a/a.go": "package a\n", "lib/l.go": "package lib\n",
		".goalierc": `{"shared": ["lib/**"], "godeps": {"omit": ["lib/**"]}}`,
	})
	if _, stderr := run(t, dir, 0, "--format", "json"); stderr != "" {
		t.Errorf("stderr = %q, want nothing", stderr)
	}
}

func TestTheOrderSettingReachesTheModelUnchanged(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod, "web/w.go": "package web\n", "api/a.go": "package api\n", "lib/l.go": "package lib\n",
		".goalierc": `{"shared": ["lib/**"], "godeps": {"order": ["web", "shared", "api"]}}`,
	})
	stdout, stderr := run(t, dir, 0, "--format", "json")
	var got folderModel
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	if want := []string{"web", "shared", "api"}; !slices.Equal(got.Order, want) {
		t.Errorf("order = %q, want %q", got.Order, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want nothing", stderr)
	}
}

func TestAnOrderNameThatMatchesNoTopLevelBoxWarnsAndExitsCleanly(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod, "shop.go": "package shop\n", "web/api/a.go": "package api\n",
		".goalierc": `{"godeps": {"order": ["nope", "api", "web"]}}`,
	})
	stdout, stderr := run(t, dir, 0, "--format", "json")
	want := "godeps: order name \"nope\" matches no top-level box\ngodeps: order name \"api\" matches no top-level box\n"
	if stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	if !strings.Contains(stdout, `"order": [`) {
		t.Errorf("stdout = %s; want the order kept unchanged", stdout)
	}
}

func TestAnOrderNameCanNameAnyTopLevelNodeOfThePage(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod, "main.go": "package app\n", "web/a.go": "package web\n\nimport _ \"example.com/lib/x\"\n",
		".goalierc": `{"godeps": {"foreign": ["example.com/lib"], "order": ["app", "example.com/lib", "web"]}}`,
	})
	if _, stderr := run(t, dir, 0, "--format", "json"); stderr != "" {
		t.Errorf("stderr = %q, want nothing", stderr)
	}
}

func TestAnOrderNameOfAClassInsideABoxWarns(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod, "web/a.go": "package inner\n", ".goalierc": `{"godeps": {"order": ["inner"]}}`,
	})
	_, stderr := run(t, dir, 0, "--format", "json")
	if want := "godeps: order name \"inner\" matches no top-level box\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}

func TestFileSelectionDecidesTheBoxes(t *testing.T) {
	tests := []struct {
		name, config, ignore string
		args                 []string
	}{
		{"paths setting", `{"paths": ["keep"]}`, "", nil},
		{"path argument", `{}`, "", []string{"keep"}},
		{"skip files", `{"skipFiles": ["drop/**"]}`, "", nil},
		{"skip tags", `{"skipTags": ["legacy"]}`, "", nil},
		{"ignore file", `{}`, "drop/**\n", nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{
				"go.mod": goMod, "keep/k.go": "package keep\n", "drop/d.go": "//go:build legacy\n\npackage drop\n",
				".goalierc": test.config, ".goalieignore": test.ignore,
			})
			wantBoxes(t, readFolders(t, dir, test.args...), box{ID: "keep", Name: "keep", PackageCount: 1})
		})
	}
}

func TestHelpStatesWhatOmitPatternsCover(t *testing.T) {
	stdout, _ := run(t, t.TempDir(), 0, "--help")
	for _, want := range []string{"foreign", "omit", "order", "gen/**"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q; want %q in it", stdout, want)
		}
	}
}

func TestASharedPatternThatMatchesNoFileWarnsAndExitsCleanly(t *testing.T) {
	dir := writeTree(t, map[string]string{"go.mod": goMod, "a/a.go": "package a\n", ".goalierc": `{"shared": ["lib"]}`})
	_, stderr := run(t, dir, 0, "--format", "json")
	want := "godeps: shared pattern \"lib\" matches no file; to match a folder, write folder/**\n"
	if stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}
