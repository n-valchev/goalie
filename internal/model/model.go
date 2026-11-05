// Package model holds the open map format of godeps and builds it from Go files. It knows nothing
// about HTML.
package model

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const version = 1

const kindDependency = "dependency"

type Model struct {
	Version int     `json:"version"`
	Title   string  `json:"title"`
	Module  string  `json:"module"`
	Classes []Class `json:"classes"`
	Edges   []Edge  `json:"edges"`
}

// Class is one Go package. It has that name because Package in this model is a folder box.
type Class struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type Source struct {
	Dir    string
	Files  []string // Sorted slash-separated paths relative to Dir, as files.Find returns them.
	Module string
	Title  string
}

func Build(source Source) (Model, error) {
	imports := map[string][]string{}
	classes := map[string]*Class{}
	fset := token.NewFileSet()
	for _, file := range source.Files {
		// The import path of such a file can't be worked out from the module path.
		if strings.HasPrefix(file, "../") {
			return Model{}, fmt.Errorf("%s is outside the run directory", file)
		}
		parsed, err := parseFile(fset, source.Dir, file)
		if err != nil {
			return Model{}, err
		}
		folder := path.Dir(file)
		class := classes[folder]
		if class == nil {
			class = &Class{ID: classID(source.Module, folder), Path: folder}
			classes[folder] = class
		}
		// A folder can also hold a tagged `main` tool. The other clause is the one that imports use.
		if class.Name == "" || class.Name == "main" {
			class.Name = parsed.Name.Name
		}
		imports[class.ID] = append(imports[class.ID], importPaths(parsed)...)
	}
	result := Model{Version: version, Title: source.Title, Module: source.Module, Classes: []Class{}, Edges: []Edge{}}
	scanned := map[string]bool{}
	for _, class := range classes {
		result.Classes = append(result.Classes, *class)
		scanned[class.ID] = true
	}
	slices.SortFunc(result.Classes, func(a, b Class) int { return cmp.Compare(a.ID, b.ID) })
	for _, class := range result.Classes {
		targets := imports[class.ID]
		slices.Sort(targets)
		for _, target := range slices.Compact(targets) {
			// A tool in the same folder can import its library, and that is no edge between classes.
			if scanned[target] && target != class.ID {
				result.Edges = append(result.Edges, Edge{class.ID, target, kindDependency})
			}
		}
	}
	return result, nil
}

func parseFile(fset *token.FileSet, dir, file string) (*ast.File, error) {
	source, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(file)))
	if err != nil {
		return nil, err
	}
	return parser.ParseFile(fset, file, source, parser.SkipObjectResolution)
}

func classID(module, folder string) string {
	if folder == "." {
		return module
	}
	return module + "/" + folder
}

func importPaths(file *ast.File) []string {
	paths := make([]string, 0, len(file.Imports))
	for _, spec := range file.Imports {
		// The parser has already rejected a path that doesn't unquote.
		value, _ := strconv.Unquote(spec.Path.Value)
		paths = append(paths, value)
	}
	return paths
}
