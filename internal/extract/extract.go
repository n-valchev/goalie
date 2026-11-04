// Package extract parses Go files and finds their functions and methods.
package extract

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

const ignoreDirective = "//godry:ignore"

// Function is a function or method with a body.
type Function struct {
	File      string // Slash-separated path, as passed to Parse.
	Name      string
	StartLine int // The line of the func keyword, so the doc comment doesn't count.
	EndLine   int // The line of the closing brace.
	Decl      *ast.FuncDecl
}

// Lines returns the number of lines in the function's span.
func (f Function) Lines() int {
	return f.EndLine - f.StartLine + 1
}

// Parse reads files, given relative to dir, and returns their functions in file order.
// A function under a //godry:ignore comment is left out.
func Parse(dir string, files []string) ([]Function, error) {
	var functions []Function
	fset := token.NewFileSet()
	for _, file := range files {
		source, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(file)))
		if err != nil {
			return nil, err
		}
		parsed, err := parser.ParseFile(fset, file, source, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			// Before the body check, because a directive with no reason fails on any function.
			ignored, err := hasIgnoreDirective(fset, fn)
			if err != nil {
				return nil, err
			}
			if ignored || fn.Body == nil {
				continue
			}
			functions = append(functions, Function{
				File:      file,
				Name:      fn.Name.Name,
				StartLine: fset.Position(fn.Pos()).Line,
				EndLine:   fset.Position(fn.Body.Rbrace).Line,
				Decl:      fn,
			})
		}
	}
	return functions, nil
}

func hasIgnoreDirective(fset *token.FileSet, fn *ast.FuncDecl) (bool, error) {
	if fn.Doc == nil {
		return false, nil
	}
	ignored := false
	for _, comment := range fn.Doc.List {
		words := strings.Fields(comment.Text)
		if words[0] != ignoreDirective {
			continue
		}
		if len(words) == 1 {
			position := fset.Position(comment.Pos())
			return false, fmt.Errorf("%s:%d: %s needs a reason", position.Filename, position.Line, ignoreDirective)
		}
		ignored = true
	}
	return ignored, nil
}
