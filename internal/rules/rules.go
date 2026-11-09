// Package rules holds the measures of the function and file rules. It works on Go code and knows
// nothing about Git, settings, or output, which internal/check adds once for every rule.
package rules

import (
	"bytes"
	"go/ast"
	"go/token"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

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

// FileLines counts a last line with no newline too, because an editor shows it as a line.
func FileLines(src []byte) float64 {
	lines := bytes.Count(src, []byte("\n"))
	if len(src) > 0 && src[len(src)-1] != '\n' {
		lines++
	}
	return float64(lines)
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
