// Package rules holds the measures of the function rules. It works on parsed Go code and knows
// nothing about Git, settings, or output, which internal/check adds once for every rule.
package rules

import (
	"go/ast"
	"go/token"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/n-valchev/goalie/internal/extract"
)

type Measurement struct {
	Value int
	Line  int
}

// Lines counts blank lines and comments too, because an agent reads them all.
func Lines(fn extract.Function) Measurement {
	return Measurement{fn.Lines(), fn.StartLine}
}

// Parameters counts an unnamed parameter as 1, so that func(int, string) is 2, as it is to a caller.
func Parameters(fn extract.Function) Measurement {
	count := 0
	for _, field := range fn.Decl.Type.Params.List {
		count += max(len(field.Names), 1)
	}
	return Measurement{count, fn.StartLine}
}

// ComplexCondition gives only the worst condition, because one result for each condition would fill
// an agent's context with the same advice. A return or an assignment doesn't count, because naming
// a boolean expression is often the fix.
func ComplexCondition(fn extract.Function) Measurement {
	worst := Measurement{0, fn.StartLine}
	measure := func(condition ...ast.Expr) {
		if len(condition) == 0 || condition[0] == nil {
			return
		}
		count := 0
		for _, expression := range condition {
			count += operators(expression)
		}
		if count > worst.Value {
			worst = Measurement{count, fn.Line(condition[0].Pos())}
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
