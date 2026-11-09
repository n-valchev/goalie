// Package extract parses Go files and finds their functions, methods, and struct types.
package extract

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
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
	excused   excuses
}

func (f Function) Lines() int {
	return f.EndLine - f.StartLine + 1
}

func (f Function) Line(pos token.Pos) int {
	return fileLine(f.fset, pos)
}

// Type is a named struct type, declared at the top level of a file.
type Type struct {
	File      string
	Package   string // As in Function, so that a type finds its methods.
	Name      string
	StartLine int
	EndLine   int
	// Fields names an embedded field by its type name, as Go does: sync.Mutex is the field Mutex.
	Fields  []string
	excused excuses
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
	parsed    map[string]func() (parsedFile, error)
	// skipUnparsed is for an agent's half-finished edit, which must not fail the whole check. CI
	// leaves it off, so that a file that doesn't build can't pass.
	skipUnparsed bool
	unparsed     map[string]bool
}

// parsedFile holds the functions and the types of a file, so that the function rules and the type
// rules share one parse.
type parsedFile struct {
	functions []Function
	types     []Type
}

func NewParser(directive Directive) *Parser {
	return &Parser{directive: directive, parsed: map[string]func() (parsedFile, error){}, unparsed: map[string]bool{}}
}

// SkipUnparsed makes the parser leave out a file with a syntax error, and list it in Unparsed. A
// mistake in a directive still fails, because it is no half-finished edit.
func (p *Parser) SkipUnparsed() {
	p.skipUnparsed = true
}

// Unparsed returns the files that the parser left out, sorted.
func (p *Parser) Unparsed() []string {
	p.lock.Lock()
	defer p.lock.Unlock()
	files := make([]string, 0, len(p.unparsed))
	for file := range p.unparsed {
		files = append(files, file)
	}
	slices.Sort(files)
	return files
}

// Parse reads files, given relative to dir, and returns their functions in file order, apart from
// those excused from rule.
func (p *Parser) Parse(dir string, files []string, rule string) ([]Function, error) {
	parsed, err := p.parseAll(dir, files)
	return notExcused(parsed, func(file parsedFile) []Function { return file.functions }, rule), err
}

// Types reads files as Parse does, and returns their struct types, apart from those excused from rule.
func (p *Parser) Types(dir string, files []string, rule string) ([]Type, error) {
	parsed, err := p.parseAll(dir, files)
	return notExcused(parsed, func(file parsedFile) []Type { return file.types }, rule), err
}

// excuses holds the rule names that the directives above a declaration list.
type excuses []string

func (e excuses) from(rule string) bool {
	return slices.Contains(e, rule)
}

type excusable interface {
	excusedFrom(rule string) bool
}

func notExcused[T excusable](parsed []parsedFile, of func(parsedFile) []T, rule string) []T {
	var kept []T
	for _, file := range parsed {
		for _, item := range of(file) {
			if !item.excusedFrom(rule) {
				kept = append(kept, item)
			}
		}
	}
	return kept
}

func (f Function) excusedFrom(rule string) bool { return f.excused.from(rule) }

func (t Type) excusedFrom(rule string) bool { return t.excused.from(rule) }

func (p *Parser) parseAll(dir string, files []string) ([]parsedFile, error) {
	parsed := make([]parsedFile, len(files))
	errs := make([]error, len(files))
	gofiles.Parallel(len(files), func(i int) {
		parsed[i], errs[i] = p.parseOnce(dir, files[i])
	})
	var kept []parsedFile
	// The first error in file order, so that it matches a serial parse.
	for i, err := range errs {
		var syntax scanner.ErrorList
		if err != nil && p.skipUnparsed && errors.As(err, &syntax) {
			p.lock.Lock()
			p.unparsed[files[i]] = true
			p.lock.Unlock()
			continue
		}
		if err != nil {
			return nil, err
		}
		kept = append(kept, parsed[i])
	}
	return kept, nil
}

func (p *Parser) parseOnce(dir, file string) (parsedFile, error) {
	path := filepath.Join(dir, file)
	p.lock.Lock()
	parse, ok := p.parsed[path]
	if !ok {
		parse = sync.OnceValues(func() (parsedFile, error) {
			return parseFile(dir, file, p.directive)
		})
		p.parsed[path] = parse
	}
	p.lock.Unlock()
	return parse()
}

func parseFile(dir, file string, directive Directive) (parsedFile, error) {
	source, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(file)))
	if err != nil {
		return parsedFile{}, err
	}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, source, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return parsedFile{}, err
	}
	imports := importNames(parsed)
	pkg := path.Join(path.Dir(file), parsed.Name.Name)
	var result parsedFile
	for _, decl := range parsed.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.TYPE {
			types, err := directive.types(fset, gen)
			if err != nil {
				return parsedFile{}, err
			}
			for i := range types {
				types[i].File, types[i].Package = file, pkg
			}
			result.types = append(result.types, types...)
			continue
		}
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		// Before the body check, because a directive with a mistake fails on any function.
		excused, err := directive.read(fset, fn.Doc)
		if err != nil {
			return parsedFile{}, err
		}
		if fn.Body == nil {
			continue
		}
		result.functions = append(result.functions, Function{
			File:      file,
			Package:   pkg,
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
	return result, nil
}

// types reads the directive above every type, struct or not, because a directive with a mistake
// fails on any declaration. The directive of an ungrouped type sits on the declaration, not the spec.
func (d Directive) types(fset *token.FileSet, gen *ast.GenDecl) ([]Type, error) {
	var types []Type
	for _, spec := range gen.Specs {
		spec := spec.(*ast.TypeSpec)
		doc := spec.Doc
		if !gen.Lparen.IsValid() {
			doc = gen.Doc
		}
		excused, err := d.read(fset, doc)
		if err != nil {
			return nil, err
		}
		structType, ok := spec.Type.(*ast.StructType)
		if !ok {
			continue
		}
		types = append(types, Type{
			Name:      spec.Name.Name,
			StartLine: fileLine(fset, spec.Pos()),
			EndLine:   fileLine(fset, spec.End()),
			Fields:    fieldNames(structType),
			excused:   excused,
		})
	}
	return types, nil
}

func fieldNames(structType *ast.StructType) []string {
	var names []string
	for _, field := range structType.Fields.List {
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
		if len(field.Names) == 0 {
			names = append(names, embeddedName(field.Type))
		}
	}
	return names
}

func embeddedName(expr ast.Expr) string {
	for {
		switch typ := expr.(type) {
		case *ast.StarExpr:
			expr = typ.X
		case *ast.IndexExpr:
			expr = typ.X
		case *ast.IndexListExpr:
			expr = typ.X
		case *ast.SelectorExpr:
			return typ.Sel.Name
		case *ast.Ident:
			return typ.Name
		default:
			return ""
		}
	}
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

func (d Directive) read(fset *token.FileSet, doc *ast.CommentGroup) ([]string, error) {
	if doc == nil {
		return nil, nil
	}
	var excused []string
	for _, comment := range doc.List {
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
