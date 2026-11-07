package godeps_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

type op struct {
	Name     string `json:"name"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	EndLine  int    `json:"endLine"`
	Exported bool   `json:"exported"`
}

type field struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type method struct {
	Name      string `json:"name"`
	Signature string `json:"signature"`
}

type typeDecl struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	File    string   `json:"file"`
	Line    int      `json:"line"`
	Fields  []field  `json:"fields"`
	Methods []method `json:"methods"`
}

type declarations struct {
	ID    string     `json:"id"`
	Types []typeDecl `json:"types"`
	Ops   []op       `json:"ops"`
}

func readDeclarations(t *testing.T, dir, id string) declarations {
	t.Helper()
	stdout, _ := run(t, dir, 0, "--format", "json")
	var result struct {
		Classes []declarations `json:"classes"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not a model: %v\n%s", err, stdout)
	}
	for _, class := range result.Classes {
		if class.ID == id {
			return class
		}
	}
	t.Fatalf("no class %q in %s", id, stdout)
	return declarations{}
}

func wantOps(t *testing.T, got declarations, want ...op) {
	t.Helper()
	if !slices.Equal(got.Ops, want) {
		t.Errorf("ops = %+v, want %+v", got.Ops, want)
	}
}

func wantTypes(t *testing.T, got declarations, want ...typeDecl) {
	t.Helper()
	if len(got.Types) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got.Types, want) {
		t.Errorf("types = %+v, want %+v", got.Types, want)
	}
}

func TestOpsUseTheNamesOfTheCrapRule(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a/a.go": `package a

type Set[K comparable, V any] struct{}

type List[T any] struct{}

type order struct{}

// Total has a doc comment, which is not part of the op.
func Total() int {
	return 1
}

func (o order) Sum() int { return 1 }

func (o *order) reset() {
}

func (s *Set[K, V]) Add() {}

func (l List[T]) Len() int {
	return 0
}
`,
	})
	wantOps(t, readDeclarations(t, dir, "example.com/shop/a"),
		op{"Total", "a/a.go", 10, 12, true},
		op{"order.Sum", "a/a.go", 14, 14, true},
		op{"order.reset", "a/a.go", 16, 17, false},
		op{"Set.Add", "a/a.go", 19, 19, true},
		op{"List.Len", "a/a.go", 21, 23, true},
	)
}

func TestAFunctionWithNoBodyIsAnOp(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a/a.go": "package a\n\nfunc stub(\n\tx int,\n) int\n",
	})
	wantOps(t, readDeclarations(t, dir, "example.com/shop/a"), op{"stub", "a/a.go", 3, 5, false})
}

func TestALineDirectiveDoesNotMoveAnOp(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a/a.go": "package a\n\n//line other.go:100\nfunc F() {\n}\n",
	})
	wantOps(t, readDeclarations(t, dir, "example.com/shop/a"), op{"F", "a/a.go", 4, 5, true})
}

func TestOpsAreSortedByFileThenLine(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a/z.go": "package a\n\nfunc A() {}\n",
		"a/b.go": "package a\n\nfunc Z() {}\n\nfunc Y() {}\n",
	})
	wantOps(t, readDeclarations(t, dir, "example.com/shop/a"),
		op{"Z", "a/b.go", 3, 3, true},
		op{"Y", "a/b.go", 5, 5, true},
		op{"A", "a/z.go", 3, 3, true},
	)
}

func TestAClassWithNoDeclarationsHasEmptyLists(t *testing.T) {
	dir := writeTree(t, map[string]string{"go.mod": goMod, "a/a.go": "package a\n"})
	got := readDeclarations(t, dir, "example.com/shop/a")
	if got.Types == nil || got.Ops == nil {
		t.Errorf("types = %#v, ops = %#v, want empty arrays", got.Types, got.Ops)
	}
}

func TestAForeignClassHasEmptyLists(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a/a.go":    "package a\n\nimport _ \"example.com/lib\"\n",
		".goalierc": `{"godeps": {"foreign": ["example.com/lib"]}}`,
	})
	got := readDeclarations(t, dir, "example.com/lib")
	if got.Types == nil || got.Ops == nil {
		t.Errorf("types = %#v, ops = %#v, want empty arrays", got.Types, got.Ops)
	}
}

func TestStructsKeepTheSourceTextOfTheirFieldTypes(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a/a.go": `package a

import (
	"sync"
	"time"
)

type Order struct {
	ID         string
	Lines, Max []int
	Seen       map[string]*time.Time ` + "`json:\"seen\"`" + `
	Done       chan<- struct{}
	sync.Mutex
	*Base
	Page[int]
	_ int
}

type Base struct{}

type Page[T any] struct {
	Items []T
}

type empty struct{}
`,
	})
	wantTypes(t, readDeclarations(t, dir, "example.com/shop/a"),
		typeDecl{Name: "Order", Kind: "struct", File: "a/a.go", Line: 8, Fields: []field{
			{"ID", "string"},
			{"Lines", "[]int"},
			{"Max", "[]int"},
			{"Seen", "map[string]*time.Time"},
			{"Done", "chan<- struct{}"},
			{"Mutex", "sync.Mutex"},
			{"Base", "*Base"},
			{"Page", "Page[int]"},
			{"_", "int"},
		}},
		typeDecl{Name: "Base", Kind: "struct", File: "a/a.go", Line: 19},
		typeDecl{Name: "Page", Kind: "struct", File: "a/a.go", Line: 21, Fields: []field{{"Items", "[]T"}}},
		typeDecl{Name: "empty", Kind: "struct", File: "a/a.go", Line: 25},
	)
}

func TestInterfacesListMethodsAndEmbeddedInterfaces(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a/a.go": `package a

import "io"

type Store[K comparable, V any] interface {
	io.Closer
	Reader
	Get(key K) (V, error)
	Put(key K,
		value V) error
	reset()
}

type Reader interface {
	Read() []byte
}

type Number interface {
	~int | ~float64
}

type any2 interface{}
`,
	})
	wantTypes(t, readDeclarations(t, dir, "example.com/shop/a"),
		typeDecl{Name: "Store", Kind: "interface", File: "a/a.go", Line: 5, Methods: []method{
			{Name: "io.Closer"},
			{Name: "Reader"},
			{"Get", "(key K) (V, error)"},
			{"Put", "(key K,\n\t\tvalue V) error"},
			{"reset", "()"},
		}},
		typeDecl{Name: "Reader", Kind: "interface", File: "a/a.go", Line: 14, Methods: []method{{"Read", "() []byte"}}},
		typeDecl{Name: "Number", Kind: "interface", File: "a/a.go", Line: 18, Methods: []method{{Name: "~int | ~float64"}}},
		typeDecl{Name: "any2", Kind: "interface", File: "a/a.go", Line: 22},
	)
}

func TestOtherTypesHaveNoFieldsOrMethods(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a/a.go": `package a

type (
	ID      string
	Handler func(int) error
	Alias   = ID
	Named   Base
	Tree[T any] []T
)

type Base struct {
	X int
}
`,
	})
	wantTypes(t, readDeclarations(t, dir, "example.com/shop/a"),
		typeDecl{Name: "ID", Kind: "other", File: "a/a.go", Line: 4},
		typeDecl{Name: "Handler", Kind: "other", File: "a/a.go", Line: 5},
		typeDecl{Name: "Alias", Kind: "other", File: "a/a.go", Line: 6},
		typeDecl{Name: "Named", Kind: "other", File: "a/a.go", Line: 7},
		typeDecl{Name: "Tree", Kind: "other", File: "a/a.go", Line: 8},
		typeDecl{Name: "Base", Kind: "struct", File: "a/a.go", Line: 11, Fields: []field{{"X", "int"}}},
	)
}

func TestATypeInsideAFunctionIsNotListed(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a/a.go": "package a\n\nfunc F() {\n\ttype local struct{}\n\t_ = local{}\n}\n",
	})
	wantTypes(t, readDeclarations(t, dir, "example.com/shop/a"))
}

func TestTypesAreSortedByFileThenLine(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a/z.go": "package a\n\ntype A int\n",
		"a/b.go": "package a\n\ntype Z int\n\ntype Y int\n",
	})
	wantTypes(t, readDeclarations(t, dir, "example.com/shop/a"),
		typeDecl{Name: "Z", Kind: "other", File: "a/b.go", Line: 3},
		typeDecl{Name: "Y", Kind: "other", File: "a/b.go", Line: 5},
		typeDecl{Name: "A", Kind: "other", File: "a/z.go", Line: 3},
	)
}

func TestAnEmptyFieldListIsLeftOutOfTheJSON(t *testing.T) {
	dir := writeTree(t, map[string]string{"go.mod": goMod, "a/a.go": "package a\n\ntype A struct{}\n"})
	stdout, _ := run(t, dir, 0, "--format", "json")
	for _, unwanted := range []string{`"fields"`, `"methods"`} {
		if strings.Contains(stdout, unwanted) {
			t.Errorf("stdout has %s:\n%s", unwanted, stdout)
		}
	}
}
