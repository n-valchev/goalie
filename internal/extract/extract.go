// Package extract parses Go files and finds their functions and methods.
package extract

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const ignoreDirective = "//godry:ignore"

// Function is a function or method with a body.
type Function struct {
	File      string // Slash-separated path, as passed to Parse.
	Package   string // The folder and package clause. Each alone can repeat: `main`, or `a` and `a_test`.
	Name      string
	Receiver  string // The type name alone, or empty for a function.
	StartLine int    // The line of the func keyword, so the doc comment doesn't count.
	EndLine   int    // The line of the closing brace.
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
				Package:   path.Join(path.Dir(file), parsed.Name.Name),
				Name:      fn.Name.Name,
				Receiver:  receiverType(fn),
				StartLine: fileLine(fset, fn.Pos()),
				EndLine:   fileLine(fset, fn.Body.Rbrace),
				Decl:      fn,
			})
		}
	}
	return functions, nil
}

// fileLine ignores //line directives, because a span must point into the file on disk.
func fileLine(fset *token.FileSet, pos token.Pos) int {
	return fset.PositionFor(pos, false).Line
}

// receiverType drops the pointer and type parameters, because platform variants of one method
// need not agree on them: `(s *Set[T])` in one file and `(s Set[K])` in another name the same type.
func receiverType(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	expr := fn.Recv.List[0].Type
	for {
		switch typ := expr.(type) {
		case *ast.StarExpr:
			expr = typ.X
		case *ast.ParenExpr:
			expr = typ.X
		case *ast.IndexExpr:
			expr = typ.X
		case *ast.IndexListExpr:
			expr = typ.X
		case *ast.Ident:
			return typ.Name
		default:
			return ""
		}
	}
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
