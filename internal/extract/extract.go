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
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

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

func (f Function) Lines() int {
	return f.EndLine - f.StartLine + 1
}

// Parse reads files, given relative to dir, and returns their functions in file order.
// A function under a comment that starts with directive is left out. Each tool has its own, because
// a reason to skip a function in one check says nothing about the other.
func Parse(dir string, files []string, directive string) ([]Function, error) {
	type result struct {
		functions []Function
		err       error
	}
	results := make([]result, len(files))
	var next atomic.Int64
	var workers sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), len(files)) {
		workers.Go(func() {
			for {
				i := int(next.Add(1)) - 1
				if i >= len(files) {
					return
				}
				functions, err := parseFile(dir, files[i], directive)
				results[i] = result{functions, err}
			}
		})
	}
	workers.Wait()
	// Results stay in file order, so the first error and the function order match a serial parse.
	var functions []Function
	for _, result := range results {
		if result.err != nil {
			return nil, result.err
		}
		functions = append(functions, result.functions...)
	}
	return functions, nil
}

func parseFile(dir, file, directive string) ([]Function, error) {
	source, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(file)))
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, source, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var functions []Function
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		// Before the body check, because a directive with no reason fails on any function.
		ignored, err := hasIgnoreDirective(fset, fn, directive)
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
			Receiver:  ReceiverType(fn),
			StartLine: fileLine(fset, fn.Pos()),
			EndLine:   fileLine(fset, fn.Body.Rbrace),
			Decl:      fn,
		})
	}
	return functions, nil
}

// fileLine ignores //line directives, because a span must point into the file on disk.
func fileLine(fset *token.FileSet, pos token.Pos) int {
	return fset.PositionFor(pos, false).Line
}

// ReceiverType drops the pointer and type parameters, because platform variants of one method
// need not agree on them: `(s *Set[T])` in one file and `(s Set[K])` in another name the same type.
func ReceiverType(fn *ast.FuncDecl) string {
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

// HasDirective accepts a directive with no reason, because that error belongs to the tool that owns
// the directive, and a reader of it has no check to fail.
func HasDirective(fn *ast.FuncDecl, directive string) bool {
	if fn.Doc == nil {
		return false
	}
	return slices.ContainsFunc(fn.Doc.List, func(comment *ast.Comment) bool {
		return strings.Fields(comment.Text)[0] == directive
	})
}

func hasIgnoreDirective(fset *token.FileSet, fn *ast.FuncDecl, directive string) (bool, error) {
	if fn.Doc == nil {
		return false, nil
	}
	ignored := false
	for _, comment := range fn.Doc.List {
		words := strings.Fields(comment.Text)
		if words[0] != directive {
			continue
		}
		if len(words) == 1 {
			position := fset.PositionFor(comment.Pos(), false)
			return false, fmt.Errorf("%s:%d: %s needs a reason", position.Filename, position.Line, directive)
		}
		ignored = true
	}
	return ignored, nil
}
