// Package fingerprint normalizes a function and produces its fingerprint set and node count.
package fingerprint

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// Set holds the printed form of every subtree and every atom of a normalized function.
type Set map[string]struct{}

const (
	nameMarker    = "name"
	literalMarker = "literal"
)

type frame struct {
	label    string
	children []string
}

// Of returns the fingerprint set and node count of fn.
// It walks only the signature and body. Leaving out the function name and the receiver lets
// a renamed copy, or a method copied onto another type, score as an exact match.
func Of(fn *ast.FuncDecl) (Set, int) {
	called := calledNames(fn)
	set := Set{}
	nodes := 0
	stack := []*frame{{}}
	visit := func(n ast.Node) bool {
		if n == nil {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			printed := top.print()
			set[printed] = struct{}{}
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, printed)
			return true
		}
		if _, comment := n.(*ast.CommentGroup); comment {
			return false
		}
		nodes++
		label := labelOf(n, called)
		if op := operatorOf(n); op != token.ILLEGAL {
			set[op.String()] = struct{}{}
			label += " " + op.String()
		}
		stack = append(stack, &frame{label: label})
		return true
	}
	ast.Inspect(fn.Type, visit)
	ast.Inspect(fn.Body, visit)
	return set, nodes
}

func (f *frame) print() string {
	if isLeafLabel(f.label) {
		return f.label
	}
	return "(" + f.label + " " + strings.Join(f.children, " ") + ")"
}

func isLeafLabel(label string) bool {
	return !strings.HasPrefix(label, "*ast.")
}

// calledNames finds the identifiers in call position. For a method call that is the method name,
// so the receiver still normalizes.
func calledNames(fn *ast.FuncDecl) map[*ast.Ident]bool {
	called := map[*ast.Ident]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if name := calledName(call.Fun); name != nil {
				called[name] = true
			}
		}
		return true
	})
	return called
}

func calledName(fun ast.Expr) *ast.Ident {
	switch fun := fun.(type) {
	case *ast.Ident:
		return fun
	case *ast.SelectorExpr:
		return fun.Sel
	case *ast.ParenExpr:
		return calledName(fun.X)
	case *ast.IndexExpr:
		return calledName(fun.X)
	case *ast.IndexListExpr:
		return calledName(fun.X)
	}
	return nil
}

func labelOf(n ast.Node, called map[*ast.Ident]bool) string {
	switch n := n.(type) {
	case *ast.BasicLit:
		return literalMarker
	case *ast.Ident:
		switch {
		case called[n]:
			return n.Name
		case n.Name == "nil" || n.Name == "true" || n.Name == "false":
			return literalMarker
		}
		return nameMarker
	}
	return fmt.Sprintf("%T", n)
}

func operatorOf(n ast.Node) token.Token {
	switch n := n.(type) {
	case *ast.BinaryExpr:
		return n.Op
	case *ast.UnaryExpr:
		return n.Op
	case *ast.AssignStmt:
		return n.Tok
	case *ast.IncDecStmt:
		return n.Tok
	case *ast.BranchStmt:
		return n.Tok
	case *ast.RangeStmt:
		return n.Tok
	case *ast.GenDecl:
		return n.Tok
	}
	return token.ILLEGAL
}
