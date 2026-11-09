// Package rules holds the measures of the function, file, and type rules. It works on Go code and
// knows nothing about Git, settings, or output, which internal/check adds once for every rule.
package rules

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/n-valchev/goalie/internal/crap"
	"github.com/n-valchev/goalie/internal/extract"
)

type Measurement struct {
	Value      int
	Line       int
	Parameters []Parameter // The parameters that the value counts, for a rule whose result must name them.
}

type Parameter struct {
	Name string
	Type string
}

// Lines counts blank lines and comments too, because an agent reads them all.
func Lines(fn extract.Function) Measurement {
	return Measurement{Value: fn.Lines(), Line: fn.StartLine}
}

// FileMeasurement holds Functions only for a rule whose result must name how many functions the
// value covers.
type FileMeasurement struct {
	Value     float64
	Functions int
}

// FileLines counts a last line with no newline too, because an editor shows it as a line.
func FileLines(src []byte) (FileMeasurement, bool) {
	lines := bytes.Count(src, []byte("\n"))
	if len(src) > 0 && src[len(src)-1] != '\n' {
		lines++
	}
	return FileMeasurement{Value: float64(lines)}, true
}

// MeanComplexity gives a file with no function the value 0, not no value. Every limit is at least 0,
// so the file never reports, and a change that adds complex functions to it still crosses the limit,
// as a new file does.
func MeanComplexity(src []byte) (FileMeasurement, bool) {
	file, err := parser.ParseFile(token.NewFileSet(), "", src, parser.SkipObjectResolution)
	if err != nil {
		return FileMeasurement{}, false
	}
	var total, functions int
	for _, decl := range file.Decls {
		// A function with no body has no complexity, and counting it as 1 would lower the mean.
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
			total += crap.Complexity(fn)
			functions++
		}
	}
	if functions == 0 {
		return FileMeasurement{}, true
	}
	return FileMeasurement{Value: float64(total) / float64(functions), Functions: functions}, true
}

// TypeMeasurement holds the method names of each group, ordered by name, and the groups ordered by
// their first method.
type TypeMeasurement struct {
	Value  int
	Groups [][]string
}

// LowCohesion goes by method name, so that the platform variants of one method in two files count
// once. A method that only other methods call stays in, because the call joins it to them.
func LowCohesion(typ extract.Type, methods []extract.Function) TypeMeasurement {
	c := cohesion{fields: setOf(typ.Fields...), methods: map[string]bool{}, groups: unionFind{}, kept: map[string]bool{}}
	for _, method := range methods {
		c.methods[method.Name] = true
	}
	for _, method := range methods {
		c.read(method)
	}
	return c.measure()
}

type cohesion struct {
	fields, methods map[string]bool
	groups          unionFind
	kept            map[string]bool // The methods that use a field, call a method, or are called.
	passers         []string
}

type nodeKind int

const (
	methodNode nodeKind = iota
	fieldNode
	// embeddedNode stands for every embedded field at once, because an embedded field promotes what
	// it holds, and with no types the rule can't tell which embedded field holds it.
	embeddedNode
)

type node struct {
	kind nodeKind
	name string
}

func (c *cohesion) read(method extract.Function) {
	receiver := receiverName(method.Decl)
	if receiver == "" {
		return
	}
	ast.Inspect(method.Decl.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if x, ok := n.X.(*ast.Ident); ok && x.Name == receiver {
				c.join(method.Name, c.nodeOf(n.Sel.Name))
			}
		case *ast.CallExpr:
			if slices.ContainsFunc(n.Args, func(arg ast.Expr) bool { return isName(arg, receiver) }) {
				c.passers = append(c.passers, method.Name)
				c.kept[method.Name] = true
			}
		}
		return true
	})
}

func (c *cohesion) nodeOf(selected string) node {
	switch {
	case c.fields[selected]:
		return node{fieldNode, selected}
	case c.methods[selected]:
		return node{methodNode, selected}
	}
	return node{kind: embeddedNode}
}

func (c *cohesion) join(method string, other node) {
	c.groups.union(node{methodNode, method}, other)
	c.kept[method] = true
	if other.kind == methodNode {
		c.kept[other.name] = true
	}
}

// isName counts &s and *s as s, because each passes the whole receiver.
func isName(expr ast.Expr, name string) bool {
	if unary, ok := expr.(*ast.UnaryExpr); ok && unary.Op == token.AND {
		expr = unary.X
	}
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == name
}

func (c *cohesion) measure() TypeMeasurement {
	if len(c.kept) < 2 {
		return TypeMeasurement{}
	}
	// The callee may use any field, so passing the whole receiver joins every group.
	for _, passer := range c.passers {
		for name := range c.kept {
			c.groups.union(node{methodNode, passer}, node{methodNode, name})
		}
	}
	byRoot := map[node][]string{}
	for name := range c.kept {
		root := c.groups.find(node{methodNode, name})
		byRoot[root] = append(byRoot[root], name)
	}
	var result TypeMeasurement
	for _, group := range byRoot {
		slices.Sort(group)
		result.Groups = append(result.Groups, group)
	}
	slices.SortFunc(result.Groups, func(a, b []string) int { return strings.Compare(a[0], b[0]) })
	result.Value = len(result.Groups)
	return result
}

func receiverName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 || len(fn.Recv.List[0].Names) == 0 {
		return ""
	}
	return fn.Recv.List[0].Names[0].Name
}

type unionFind map[node]node

func (u unionFind) find(n node) node {
	for {
		parent, ok := u[n]
		if !ok || parent == n {
			return n
		}
		n = parent
	}
}

func (u unionFind) union(a, b node) {
	u[u.find(a)] = u.find(b)
}

// Parameters counts an unnamed parameter as 1, so that func(int, string) is 2, as it is to a caller.
func Parameters(fn extract.Function) Measurement {
	count := 0
	for _, field := range fn.Decl.Type.Params.List {
		count += max(len(field.Names), 1)
	}
	return Measurement{Value: count, Line: fn.StartLine}
}

// ComplexCondition gives only the worst condition, because one result for each condition would fill
// an agent's context with the same advice. A return or an assignment doesn't count, because naming
// a boolean expression is often the fix.
func ComplexCondition(fn extract.Function) Measurement {
	worst := Measurement{Line: fn.StartLine}
	measure := func(condition ...ast.Expr) {
		if len(condition) == 0 || condition[0] == nil {
			return
		}
		count := 0
		for _, expression := range condition {
			count += operators(expression)
		}
		if count > worst.Value {
			worst = Measurement{Value: count, Line: fn.Line(condition[0].Pos())}
		}
	}
	ast.Inspect(fn.Decl.Body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.IfStmt:
			measure(node.Cond)
		case *ast.ForStmt:
			measure(node.Cond)
		case *ast.CaseClause:
			// One condition, because a reader must hold every expression of the case in mind at once.
			measure(node.List...)
		}
		return true
	})
	return worst
}

// operators also counts inside a closure in the condition, because a reader must follow it too.
func operators(condition ast.Expr) int {
	count := 0
	ast.Inspect(condition, func(node ast.Node) bool {
		if binary, ok := node.(*ast.BinaryExpr); ok && (binary.Op == token.LAND || binary.Op == token.LOR) {
			count++
		}
		return true
	})
	return count
}

// CognitiveComplexity follows the SonarSource measure, except that operators add nothing, because
// complex-condition reports them, and that a Go guard costs 0, so that idiomatic error checks don't
// add up to a result.
func CognitiveComplexity(fn extract.Function) Measurement {
	return Measurement{Value: cognitive(fn.Decl.Body, 0), Line: fn.StartLine}
}

// cognitive skips root itself, so that a caller can score what is inside a statement that it has
// already counted.
func cognitive(root ast.Node, nesting int) int {
	if root == nil {
		return 0
	}
	total := 0
	ast.Inspect(root, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.IfStmt:
			if node != root {
				if isGuard(node) {
					// The guard costs 0, but a closure in its init can still hold branches.
					total += cognitive(node.Init, nesting+1)
				} else {
					total += ifChain(node, nesting, 1+nesting)
				}
				return false
			}
		case *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			if node != root {
				total += 1 + nesting + cognitive(node, nesting+1)
				return false
			}
		case *ast.FuncLit:
			total += cognitive(node.Body, nesting+1)
			return false
		}
		return true
	})
	return total
}

// ifChain charges an else if only 1, because a reader takes the chain as one choice.
func ifChain(node *ast.IfStmt, nesting, cost int) int {
	total := cost + cognitive(node.Init, nesting+1) + cognitive(node.Cond, nesting+1) +
		cognitive(node.Body, nesting+1)
	switch next := node.Else.(type) {
	case *ast.IfStmt:
		total += ifChain(next, nesting, 1)
	case *ast.BlockStmt:
		total += 1 + cognitive(next, nesting+1)
	}
	return total
}

// isGuard goes by names alone, because the parser has no types: err != nil may test any type.
func isGuard(node *ast.IfStmt) bool {
	body := node.Body.List
	if node.Else != nil || !isGuardCondition(node.Cond) || len(body) == 0 {
		return false
	}
	blocks := 0
	ast.Inspect(node.Body, func(node ast.Node) bool {
		if _, ok := node.(*ast.BlockStmt); ok {
			blocks++
		}
		return true
	})
	// The one block is the body itself.
	return blocks == 1 && exits(body[len(body)-1])
}

func isGuardCondition(condition ast.Expr) bool {
	switch condition := condition.(type) {
	case *ast.BinaryExpr:
		name, isName := condition.X.(*ast.Ident)
		other, isNil := condition.Y.(*ast.Ident)
		return condition.Op == token.NEQ && isName && (name.Name == "err" || strings.HasSuffix(name.Name, "Err")) &&
			isNil && other.Name == "nil"
	case *ast.UnaryExpr:
		name, isName := condition.X.(*ast.Ident)
		return condition.Op == token.NOT && isName && name.Name == "ok"
	}
	return false
}

func exits(statement ast.Stmt) bool {
	switch statement := statement.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.BranchStmt:
		return statement.Tok == token.BREAK || statement.Tok == token.CONTINUE
	case *ast.ExprStmt:
		call, isCall := statement.X.(*ast.CallExpr)
		if !isCall {
			return false
		}
		name, isName := call.Fun.(*ast.Ident)
		return isName && name.Name == "panic"
	}
	return false
}

// PrimitiveArguments leaves out a parameter that is never used, because go vet and other linters
// report it. A call into an imported package or a built-in function is a real use, because there the
// value leaves the repo's code and the basic type is required.
func PrimitiveArguments(fn extract.Function) Measurement {
	var passed []Parameter
	for _, field := range fn.Decl.Type.Params.List {
		typ, ok := field.Type.(*ast.Ident)
		if !ok || !basicTypes[typ.Name] {
			continue
		}
		for _, param := range field.Names {
			if param.Name != "_" && onlyPassedDown(fn, param.Name) {
				passed = append(passed, Parameter{param.Name, typ.Name})
			}
		}
	}
	return Measurement{Value: len(passed), Line: fn.StartLine, Parameters: passed}
}

var basicTypes = setOf("bool", "string", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16",
	"uint32", "uint64", "uintptr", "byte", "rune", "float32", "float64", "complex64", "complex128")

// builtins holds the predeclared functions, and any and error, because a conversion to them is a
// real use too.
var builtins = setOf("append", "cap", "clear", "close", "complex", "copy", "delete", "imag", "len", "make",
	"max", "min", "new", "panic", "print", "println", "real", "recover", "any", "error")

func setOf(names ...string) map[string]bool {
	set := map[string]bool{}
	for _, name := range names {
		set[name] = true
	}
	return set
}

// onlyPassedDown goes by name, because the parser resolves no objects, so a closure that declares
// the same name counts as a use of the parameter.
func onlyPassedDown(fn extract.Function, name string) bool {
	used, passed := 0, 0
	fields := map[*ast.Ident]bool{}
	ast.Inspect(fn.Decl.Body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.SelectorExpr:
			// Sel is a field or method name, never the parameter.
			fields[node.Sel] = true
		case *ast.CallExpr:
			if callsRepoCode(fn, node.Fun) {
				for _, arg := range node.Args {
					if ident, ok := arg.(*ast.Ident); ok && ident.Name == name {
						passed++
					}
				}
			}
		case *ast.Ident:
			if node.Name == name && !fields[node] {
				used++
			}
		}
		return true
	})
	return used > 0 && used == passed
}

// callsRepoCode treats a conversion to a type that no package declares, such as []byte(s), as a
// built-in call, and any other call that it can't place, such as UserID(id), as a call into the repo.
func callsRepoCode(fn extract.Function, callee ast.Expr) bool {
	switch callee := callee.(type) {
	case *ast.Ident:
		return !builtins[callee.Name] && !basicTypes[callee.Name]
	case *ast.SelectorExpr:
		pkg, ok := callee.X.(*ast.Ident)
		return !ok || !slices.Contains(fn.Imports, pkg.Name)
	case *ast.IndexExpr:
		return callsRepoCode(fn, callee.X)
	case *ast.IndexListExpr:
		return callsRepoCode(fn, callee.X)
	case *ast.ArrayType, *ast.MapType, *ast.ChanType, *ast.FuncType, *ast.InterfaceType:
		return false
	}
	return true
}

// IsConstructor needs the capital, so that Newline, which builds nothing, keeps the lower limit of
// an ordinary function. A method is never a constructor: its receiver already exists.
func IsConstructor(fn extract.Function) bool {
	rest, ok := strings.CutPrefix(fn.Name, "New")
	if !ok || fn.Decl.Recv != nil {
		return false
	}
	first, _ := utf8.DecodeRuneInString(rest)
	return rest == "" || unicode.IsUpper(first)
}

func IsNotConstructor(fn extract.Function) bool {
	return !IsConstructor(fn)
}
