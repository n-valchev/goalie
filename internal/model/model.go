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
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Package string `json:"package,omitempty"`
	Foreign bool   `json:"foreign,omitempty"`
}

type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type Source struct {
	Dir     string
	Files   []string // Sorted slash-separated paths relative to Dir, as files.Find returns them.
	Module  string
	Title   string
	Shared  []string // Patterns for files.MatchesAny.
	Foreign []string // Module paths.
	Omit    []string // Patterns for files.MatchesAny, matched against a class's path.
	Order   []string
}

func Build(source Source) (Model, error) {
	imports := map[string][]string{}
	classes := map[string]*Class{}
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
		parsed, err := parseFile(fset, source.Dir, file)
		if err != nil {
			return Model{}, err
		}
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
				result.Edges = append(result.Edges, Edge{class.ID, target, kindDependency})
			}
		}
	}
	for _, edge := range result.Edges {
		if !scanned[edge.To] {
			scanned[edge.To] = true
			result.Classes = append(result.Classes, Class{ID: edge.To, Name: edge.To, Foreign: true})
		}
	}
	slices.SortFunc(result.Classes, byID)
	return result, nil
}

func byID(a, b Class) int { return cmp.Compare(a.ID, b.ID) }

// importTargets returns the sorted ids of the classes that the imports reach.
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
