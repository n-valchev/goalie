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

	"github.com/n-valchev/goalie/internal/files"
)

const version = 1

const kindDependency = "dependency"

type Model struct {
	Version  int       `json:"version"`
	Title    string    `json:"title"`
	Module   string    `json:"module"`
	Order    []string  `json:"order,omitempty"`
	Packages []Package `json:"packages"`
	Classes  []Class   `json:"classes"`
	Edges    []Edge    `json:"edges"`
}

// Class is one Go package. It has that name because Package in this model is a folder box.
type Class struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Path           string `json:"path,omitempty"`
	Package        string `json:"package,omitempty"`
	Foreign        bool   `json:"foreign,omitempty"`
	DuplicateCount *int   `json:"duplicateCount,omitempty"`
	Grade          Grade  `json:"grade,omitempty"`
	Types          []Type `json:"types"`
	Ops            []Op   `json:"ops"`
}

type Edge struct {
	From       string      `json:"from"`
	To         string      `json:"to"`
	Kind       string      `json:"kind"`
	Violations []Violation `json:"violations,omitempty"`
}

type Source struct {
	Dir       string   // Absolute, because editor links hold absolute paths.
	Files     []string // Sorted slash-separated paths relative to Dir, as files.Find returns them.
	Module    string
	Title     string
	Shared    []string // Patterns for files.MatchesAny.
	Foreign   []string // Module paths.
	Omit      []string // Patterns for files.MatchesAny, matched against a class's path.
	Order     []string
	EditorURL string // Template with {file} and {line}.
}

func Build(source Source) (Model, error) {
	imports := map[string][]string{}
	classes := map[string]*Class{}
	var embeds []fileEmbeds
	shared := map[string]bool{}
	fset := token.NewFileSet()
	for _, file := range source.Files {
		// The import path of such a file can't be worked out from the module path.
		if strings.HasPrefix(file, "../") {
			return Model{}, fmt.Errorf("%s is outside the run directory", file)
		}
		folder := path.Dir(file)
		if files.MatchesAny(source.Omit, folder) {
			continue
		}
		parsed, reader, err := parseFile(fset, source, file)
		if err != nil {
			return Model{}, err
		}
		class := classes[folder]
		if class == nil {
			class = &Class{ID: classID(source.Module, folder), Path: folder, Types: []Type{}, Ops: []Op{}}
			classes[folder] = class
		}
		// A folder can also hold a tagged `main` tool. The other clause is the one that imports use.
		if class.Name == "" || class.Name == "main" {
			class.Name = parsed.Name.Name
		}
		imports[class.ID] = append(imports[class.ID], importPaths(parsed)...)
		class.Types = append(class.Types, reader.types(parsed)...)
		class.Ops = append(class.Ops, reader.ops(parsed)...)
		embeds = append(embeds, readEmbeds(class.ID, parsed))
		if files.MatchesAny(source.Shared, file) {
			shared[class.ID] = true
		}
	}
	result := Model{
		Version: version,
		Title:   source.Title,
		Module:  source.Module,
		Order:   source.Order,
		Classes: []Class{},
		Edges:   []Edge{},
	}
	scanned := map[string]bool{}
	for _, class := range classes {
		result.Classes = append(result.Classes, *class)
		scanned[class.ID] = true
	}
	slices.SortFunc(result.Classes, byID)
	result.Packages = placeClasses(result.Classes, shared)
	for _, class := range result.Classes {
		for _, target := range importTargets(imports[class.ID], source, scanned) {
			// A tool in the same folder can import its library, and that is no edge between classes.
			if target != class.ID {
				result.Edges = append(result.Edges, Edge{From: class.ID, To: target, Kind: kindDependency})
			}
		}
	}
	// Before the foreign classes join, because a composition edge only joins classes in the module.
	result.Edges = append(result.Edges, compositionEdges(embeds, result.Classes)...)
	slices.SortFunc(result.Edges, compareEdges)
	result.Edges = slices.CompactFunc(result.Edges, func(a, b Edge) bool { return compareEdges(a, b) == 0 })
	for _, edge := range result.Edges {
		if !scanned[edge.To] {
			scanned[edge.To] = true
			result.Classes = append(result.Classes, Class{ID: edge.To, Name: edge.To, Foreign: true, Types: []Type{}, Ops: []Op{}})
		}
	}
	slices.SortFunc(result.Classes, byID)
	return result, nil
}

func byID(a, b Class) int { return cmp.Compare(a.ID, b.ID) }

func importTargets(imports []string, source Source, scanned map[string]bool) []string {
	var targets []string
	for _, imported := range imports {
		if scanned[imported] {
			targets = append(targets, imported)
		} else if module := foreignModule(imported, source); module != "" {
			targets = append(targets, module)
		}
	}
	slices.Sort(targets)
	return slices.Compact(targets)
}

// foreignModule returns the nearest foreign entry that holds the import, or "". The run's own
// module comes first, because an unscanned package of it must make no edge, and a foreign class
// with the id of a real class would break the unique ids.
func foreignModule(imported string, source Source) string {
	if isAtOrBelow(imported, source.Module) {
		return ""
	}
	nearest := ""
	for _, entry := range source.Foreign {
		if isAtOrBelow(imported, entry) && len(entry) > len(nearest) {
			nearest = entry
		}
	}
	return nearest
}

func isAtOrBelow(imported, root string) bool {
	return imported == root || strings.HasPrefix(imported, root+"/")
}

func parseFile(fset *token.FileSet, source Source, file string) (*ast.File, fileReader, error) {
	content, err := os.ReadFile(filepath.Join(source.Dir, filepath.FromSlash(file)))
	if err != nil {
		return nil, fileReader{}, err
	}
	parsed, err := parser.ParseFile(fset, file, content, parser.ParseComments|parser.SkipObjectResolution)
	return parsed, fileReader{fset, file, content, source.URL}, err
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
		paths = append(paths, importPath(spec))
	}
	return paths
}

func importPath(spec *ast.ImportSpec) string {
	// The parser has already rejected a path that doesn't unquote.
	value, _ := strconv.Unquote(spec.Path.Value)
	return value
}
