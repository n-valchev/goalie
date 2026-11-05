package godeps_test

import "testing"

const baseFile = "package b\n\ntype T struct{}\n\ntype G[K any] struct{}\n\ntype P[K, V any] struct{}\n"

const embedsB = "package a\n\nimport \"example.com/shop/b\"\n\ntype S struct {\n\tb.T\n}\n"

func composition(from, to string) edge {
	return edge{"example.com/shop/" + from, "example.com/shop/" + to, "composition"}
}

func TestEveryFormOfAnEmbeddedTypeMakesACompositionEdge(t *testing.T) {
	tests := []struct{ name, field string }{
		{"value", "b.T"},
		{"pointer", "*b.T"},
		{"generic", "b.G[int]"},
		{"generic pointer", "*b.G[int]"},
		{"two type arguments", "b.P[int, string]"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{
				"go.mod": goMod,
				"a/a.go": "package a\n\nimport \"example.com/shop/b\"\n\ntype S struct {\n\t" + test.field + "\n}\n",
				"b/b.go": baseFile,
			})
			wantEdges(t, readModel[model](t, dir), composition("a", "b"), dependency("a", "b"))
		})
	}
}

func TestAnAliasedImportResolvesOnItsAlias(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a/a.go": "package a\n\nimport base \"example.com/shop/b\"\n\ntype S struct {\n\tbase.T\n}\n",
		"b/b.go": baseFile,
	})
	wantEdges(t, readModel[model](t, dir), composition("a", "b"), dependency("a", "b"))
}

func TestAnImportResolvesOnThePackageClauseNotTheFolder(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":        goMod,
		"a/a.go":        "package a\n\nimport \"example.com/shop/go-thing\"\n\ntype S struct {\n\tthing.T\n}\n",
		"go-thing/t.go": "package thing\n\ntype T struct{}\n",
	})
	wantEdges(t, readModel[model](t, dir), composition("a", "go-thing"), dependency("a", "go-thing"))
}

func TestTheFolderNameOfAnImportIsNotAQualifier(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		// The folder x holds the clause y, so x.T here means the alias, which is the class b.
		"a/a.go": "package a\n\nimport (\n\tx \"example.com/shop/b\"\n\t\"example.com/shop/x\"\n)\n\nvar _ = y.V\n\ntype S struct {\n\tx.T\n}\n",
		"b/b.go": baseFile,
		"x/x.go": "package y\n\nvar V = 1\n",
	})
	wantEdges(t, readModel[model](t, dir), composition("a", "b"), dependency("a", "b"), dependency("a", "x"))
}

func TestAnImportOfAFolderWithTwoClausesUsesTheLibraryClause(t *testing.T) {
	tests := []struct{ name, toolFile string }{
		{"main sorts first", "lib/a.go"},
		{"main sorts last", "lib/z.go"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{
				"go.mod":      goMod,
				"a/a.go":      "package a\n\nimport \"example.com/shop/lib\"\n\ntype S struct {\n\tfoo.T\n}\n",
				"lib/lib.go":  "package foo\n\ntype T struct{}\n",
				test.toolFile: "//go:build tools\n\npackage main\n",
			})
			wantEdges(t, readModel[model](t, dir), composition("a", "lib"), dependency("a", "lib"))
		})
	}
}

func TestAnEmbeddedTypeInAFileReadBeforeItsTargetStillResolves(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"z/z.go": "package zed\n\ntype T struct{}\n",
		"a/a.go": "package a\n\nimport \"example.com/shop/z\"\n\ntype S struct {\n\tzed.T\n}\n",
	})
	wantEdges(t, readModel[model](t, dir), composition("a", "z"), dependency("a", "z"))
}

func TestAStructNestedInADeclarationCounts(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a/a.go": "package a\n\nimport \"example.com/shop/b\"\n\ntype S struct {\n\tInner []struct {\n\t\tb.T\n\t}\n}\n",
		"b/b.go": baseFile,
	})
	wantEdges(t, readModel[model](t, dir), composition("a", "b"), dependency("a", "b"))
}

func TestWhatIsNotACompositionEdge(t *testing.T) {
	imports := "import \"example.com/shop/b\"\n\n"
	tests := []struct{ name, source string }{
		{"a named field", imports + "type S struct {\n\tF b.T\n}\n"},
		{"a type argument of a local type", imports + "type L[K any] struct{}\n\ntype S struct {\n\tL[b.T]\n}\n"},
		{"a dot import", "import . \"example.com/shop/b\"\n\ntype S struct {\n\tT\n}\n"},
		{"an interface", imports + "type I interface {\n\tb.R\n}\n"},
		{"a struct inside a function", imports + "func F() {\n\ttype local struct {\n\t\tb.T\n\t}\n\t_ = local{}\n}\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{
				"go.mod": goMod,
				"a/a.go": "package a\n\n" + test.source,
				"b/b.go": baseFile + "\ntype R interface{}\n",
			})
			wantEdges(t, readModel[model](t, dir), dependency("a", "b"))
		})
	}
}

func TestAnEmbeddedTypeOutsideTheModuleMakesNoEdge(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a/a.go": "package a\n\nimport (\n\t\"sync\"\n\t\"example.com/other/b\"\n)\n\ntype S struct {\n\tsync.Mutex\n\tb.T\n}\n",
		"b/b.go": baseFile,
	})
	wantEdges(t, readModel[model](t, dir))
}

func TestAnEmbeddedTypeInAClassThatWasNotScannedMakesNoEdge(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a/a.go":    embedsB,
		"b/b.go":    baseFile,
		".goalierc": `{"skipFiles": ["b/**"]}`,
	})
	wantEdges(t, readModel[model](t, dir))
}

func TestAnEmbeddedTypeInAnOmittedClassMakesNoEdge(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a/a.go":    embedsB,
		"b/b.go":    baseFile,
		".goalierc": `{"godeps": {"omit": ["b"]}}`,
	})
	wantEdges(t, readModel[model](t, dir))
}

func TestAnEmbeddedTypeOfAForeignModuleIsOnlyADependency(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a/a.go":    "package a\n\nimport \"example.com/lib\"\n\ntype S struct {\n\tlib.T\n}\n",
		".goalierc": `{"godeps": {"foreign": ["example.com/lib"]}}`,
	})
	wantEdges(t, readModel[model](t, dir), edge{"example.com/shop/a", "example.com/lib", "dependency"})
}

func TestAClassEmbeddingItsOwnTypeHasNoEdge(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a/a.go":    "package a\n\ntype T struct{}\n",
		"a/tool.go": "//go:build tools\n\npackage main\n\nimport \"example.com/shop/a\"\n\ntype S struct {\n\ta.T\n}\n",
	})
	wantEdges(t, readModel[model](t, dir))
}

func TestTwoEmbeddedTypesFromOneClassMakeOneEdge(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":  goMod,
		"a/a.go":  embedsB + "\ntype U struct {\n\tb.T\n}\n",
		"a/a2.go": "package a\n\nimport other \"example.com/shop/b\"\n\ntype V struct {\n\tother.T\n}\n",
		"b/b.go":  baseFile,
	})
	wantEdges(t, readModel[model](t, dir), composition("a", "b"), dependency("a", "b"))
}

func TestEdgesAreSortedByFromThenToThenKind(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a/a.go": "package a\n\nimport (\n\t_ \"example.com/shop/b\"\n\t\"example.com/shop/c\"\n)\n\ntype S struct {\n\tc.T\n}\n",
		"b/b.go": "package b\n\nimport \"example.com/shop/c\"\n\ntype S struct {\n\tc.T\n}\n",
		"c/c.go": "package c\n\ntype T struct{}\n",
	})
	wantEdges(t, readModel[model](t, dir),
		dependency("a", "b"), composition("a", "c"), dependency("a", "c"),
		composition("b", "c"), dependency("b", "c"),
	)
}
