// Package fingerprint normalizes a function and produces its fingerprint set and node count.
package fingerprint

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// Set holds the printed form of every subtree and every atom of a normalized function.
type Set map[string]struct{}

const (
	nameMarker    = "name"
	literalMarker = "literal"
)

// Result is what a comparison needs to know about one function.
type Result struct {
	Fingerprints Set
	Nodes        int
	Literals     Set
	Calls        Set
}

// Shared returns the number of members that s and other have in common.
func (s Set) Shared(other Set) int {
	count := 0
	for member := range s {
		if _, found := other[member]; found {
			count++
		}
	}
	return count
}

// A frame with an empty label is a parenthesis, which hands its children to its parent.
type frame struct {
	label    string
	children []string
}

// Of returns the fingerprints of fn, and the literals and called names that normalization hides
// or keeps. It walks only the signature and body. Leaving out the function name and the receiver lets
// a renamed copy, or a method copied onto another type, score as an exact match.
func Of(fn *ast.FuncDecl) Result {
	called := map[*ast.Ident]bool{}
	set := Set{}
	literals := Set{}
	calls := Set{}
	nodes := 0
	stack := []*frame{{}}
	visit := func(n ast.Node) bool {
		if n == nil {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			parent := stack[len(stack)-1]
			if top.label == "" {
				parent.children = append(parent.children, top.children...)
				return true
			}
			printed := top.print()
			set[printed] = struct{}{}
			parent.children = append(parent.children, printed)
			return true
		}
		if _, comment := n.(*ast.CommentGroup); comment {
			return false
		}
		if _, paren := n.(*ast.ParenExpr); paren {
			stack = append(stack, &frame{})
			return true
		}
		// A call is always visited before the name it calls.
		if call, ok := n.(*ast.CallExpr); ok {
			if name := calledName(call.Fun); name != nil {
				called[name] = true
				calls[name.Name] = struct{}{}
			}
		}
		if lit, ok := n.(*ast.BasicLit); ok {
			if value, counted := literalValue(lit); counted {
				literals[value] = struct{}{}
			}
		}
		nodes++
		label := labelOf(n, called)
		if op := operatorOf(n); op != token.ILLEGAL {
			set[op.String()] = struct{}{}
			label += " " + op.String()
		}
		if detail := detailOf(n); detail != "" {
			label += " " + detail
		}
		stack = append(stack, &frame{label: label})
		return true
	}
	ast.Inspect(fn.Type, visit)
	ast.Inspect(fn.Body, visit)
	return Result{Fingerprints: set, Nodes: nodes, Literals: literals, Calls: calls}
}

// literalValue leaves out 0, 1, and "", because they appear in almost every function and would
// raise every score. nil, true, and false are identifiers, so they never reach this function.
// The kind stays in the value, so that "1" and 1 stay different.
func literalValue(lit *ast.BasicLit) (string, bool) {
	value := lit.Value
	if lit.Kind == token.STRING {
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		if value == "" {
			return "", false
		}
	}
	if lit.Kind == token.INT && (value == "0" || value == "1") {
		return "", false
	}
	return lit.Kind.String() + " " + value, true
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

// calledName finds the identifier in call position. For a method call that is the method name,
// so the receiver still normalizes. An index counts because f[int](x) is a generic call. Without
// types it can't be told from table[key](x), so renaming a table lowers the score. The other
// choice would make all generic calls look the same.
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

// detailOf returns the tree shape that the child nodes can't show, because it is in a field that
// isn't a node, or in which child slots are empty: a[1:] and a[:1] have the same children.
func detailOf(n ast.Node) string {
	switch n := n.(type) {
	case *ast.CallExpr:
		if n.Ellipsis.IsValid() {
			return token.ELLIPSIS.String()
		}
	case *ast.ChanType:
		return strconv.Itoa(int(n.Dir))
	case *ast.SliceExpr:
		return slotPattern(n.Low, n.High, n.Max)
	case *ast.ForStmt:
		return slotPattern(n.Init, n.Cond, n.Post)
	case *ast.TypeSpec:
		if n.Assign.IsValid() {
			return token.ASSIGN.String()
		}
	case *ast.ValueSpec:
		if n.Type != nil {
			return "typed"
		}
	}
	return ""
}

func slotPattern(slots ...ast.Node) string {
	var pattern strings.Builder
	for _, slot := range slots {
		if slot == nil {
			pattern.WriteByte('_')
		} else {
			pattern.WriteByte('x')
		}
	}
	return pattern.String()
}
