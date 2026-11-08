// Package extract parses Go files and finds their functions and methods.
package extract

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode"

	gofiles "github.com/n-valchev/goalie/internal/files"
)

// Function is a function or method with a body.
type Function struct {
	File      string // Slash-separated path, as passed to Parse.
	Package   string // The folder and package clause. Each alone can repeat: `main`, or `a` and `a_test`.
	Name      string
	Receiver  string   // The type name alone, or empty for a function.
	Imports   []string // The names that the imports of the file bind. Blank and dot imports bind none.
	StartLine int      // The line of the func keyword, so the doc comment doesn't count.
	EndLine   int      // The line of the closing brace.
	Decl      *ast.FuncDecl
	fset      *token.FileSet
	excused   []string
}

func (f Function) Lines() int {
	return f.EndLine - f.StartLine + 1
}

func (f Function) Line(pos token.Pos) int {
	return fileLine(f.fset, pos)
}

type Directive struct {
	Name  string
	Rules []string // The rule names that the comment may list.
}

// Parser belongs to one run. Its rules share it, so that a run reads each file once, however many
// rules look at that file.
type Parser struct {
	directive Directive
	lock      sync.Mutex
	parsed    map[string]func() ([]Function, error)
}

func NewParser(directive Directive) *Parser {
	return &Parser{directive: directive, parsed: map[string]func() ([]Function, error){}}
}

// Parse reads files, given relative to dir, and returns their functions in file order, apart from
// those excused from rule.
func (p *Parser) Parse(dir string, files []string, rule string) ([]Function, error) {
	type result struct {
		functions []Function
		err       error
	}
	results := make([]result, len(files))
	gofiles.Parallel(len(files), func(i int) {
		functions, err := p.parseOnce(dir, files[i])
		results[i] = result{functions, err}
	})
	// Results stay in file order, so the first error and the function order match a serial parse.
	var functions []Function
	for _, result := range results {
		if result.err != nil {
			return nil, result.err
		}
		for _, fn := range result.functions {
			if !slices.Contains(fn.excused, rule) {
				functions = append(functions, fn)
			}
		}
	}
	return functions, nil
}

func (p *Parser) parseOnce(dir, file string) ([]Function, error) {
	path := filepath.Join(dir, file)
	p.lock.Lock()
	parse, ok := p.parsed[path]
	if !ok {
		parse = sync.OnceValues(func() ([]Function, error) {
			return parseFile(dir, file, p.directive)
		})
		p.parsed[path] = parse
	}
	p.lock.Unlock()
	return parse()
}

func parseFile(dir, file string, directive Directive) ([]Function, error) {
	source, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(file)))
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, source, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	imports := importNames(parsed)
	var functions []Function
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		// Before the body check, because a directive with a mistake fails on any function.
		excused, err := directive.read(fset, fn)
		if err != nil {
			return nil, err
		}
		if fn.Body == nil {
			continue
		}
		functions = append(functions, Function{
			File:      file,
			Package:   path.Join(path.Dir(file), parsed.Name.Name),
			Name:      fn.Name.Name,
			Receiver:  ReceiverType(fn),
			Imports:   imports,
			StartLine: fileLine(fset, fn.Pos()),
			EndLine:   fileLine(fset, fn.Body.Rbrace),
			Decl:      fn,
			fset:      fset,
			excused:   excused,
		})
	}
	return functions, nil
}

// importNames guesses the name of an unnamed import from its path, as goimports does, because the
// parser can't read the package clause of another package: gopkg.in/yaml.v3 binds yaml,
// github.com/mattn/go-sqlite3 binds sqlite3, and github.com/opentracing/opentracing-go binds
// opentracing.
func importNames(file *ast.File) []string {
	var names []string
	for _, spec := range file.Imports {
		if spec.Name != nil {
			if spec.Name.Name != "_" && spec.Name.Name != "." {
				names = append(names, spec.Name.Name)
			}
			continue
		}
		importPath := strings.Trim(spec.Path.Value, `"`)
		name := path.Base(importPath)
		if isMajorVersion(name) && path.Dir(importPath) != "." {
			name = path.Base(path.Dir(importPath))
		}
		name = strings.TrimPrefix(name, "go-")
		if end := strings.IndexFunc(name, notInIdentifier); end >= 0 {
			name = name[:end]
		}
		names = append(names, name)
	}
	return names
}

func notInIdentifier(r rune) bool {
	return r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

func isMajorVersion(element string) bool {
	digits, ok := strings.CutPrefix(element, "v")
	return ok && digits != "" && strings.Trim(digits, "0123456789") == ""
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

// IgnoreDirective is followed by rule names and a reason: //goalie:ignore crap,dup <reason>.
const IgnoreDirective = "//goalie:ignore"

// Excuses accepts a directive with no reason, because that error belongs to goalie check, and a
// reader of the directive has no check to fail.
func Excuses(fn *ast.FuncDecl, rule string) bool {
	if fn.Doc == nil {
		return false
	}
	return slices.ContainsFunc(fn.Doc.List, func(comment *ast.Comment) bool {
		words := strings.Fields(comment.Text)
		return words[0] == IgnoreDirective && len(words) > 1 && slices.Contains(strings.Split(words[1], ","), rule)
	})
}

func (d Directive) read(fset *token.FileSet, fn *ast.FuncDecl) ([]string, error) {
	if fn.Doc == nil {
		return nil, nil
	}
	var excused []string
	for _, comment := range fn.Doc.List {
		words := strings.Fields(comment.Text)
		if words[0] != d.Name {
			continue
		}
		rules, err := d.rulesIn(words[1:])
		if err != nil {
			position := fset.PositionFor(comment.Pos(), false)
			return nil, fmt.Errorf("%s:%d: %s %w", position.Filename, position.Line, d.Name, err)
		}
		excused = append(excused, rules...)
	}
	return excused, nil
}

func (d Directive) rulesIn(words []string) ([]string, error) {
	if len(words) == 0 {
		return nil, errors.New("needs rule names and a reason")
	}
	rules := strings.Split(words[0], ",")
	for _, rule := range rules {
		if !slices.Contains(d.Rules, rule) {
			return nil, fmt.Errorf("names unknown rule %q", rule)
		}
	}
	if len(words) == 1 {
		return nil, errors.New("needs a reason")
	}
	return rules, nil
}
