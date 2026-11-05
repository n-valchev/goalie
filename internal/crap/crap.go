// Package crap computes the Change Risk Anti-Pattern score of a function.
package crap

import (
	"go/ast"
	"go/token"
	"math"

	"github.com/n-valchev/goalie/internal/files"
)

// DefaultLimit is the score above which gocrap reports a function, when nothing sets a limit.
const DefaultLimit = 30.0

// Limits is the rule that decides each function's limit, so that godeps grades with what gocrap fails on.
type Limits struct {
	Limit       float64
	SharedLimit *float64
	Shared      []string // Patterns for files.MatchesAny.
}

func (l Limits) For(file string) float64 {
	if l.SharedLimit != nil && files.MatchesAny(l.Shared, file) {
		return *l.SharedLimit
	}
	return l.Limit
}

// Complexity counts a default clause as no branch, because it adds no path that a switch without
// one lacks. It includes closures, because their branches run as part of the function.
func Complexity(fn *ast.FuncDecl) int {
	complexity := 1
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			complexity++
		case *ast.CaseClause:
			if n.List != nil {
				complexity++
			}
		case *ast.CommClause:
			if n.Comm != nil {
				complexity++
			}
		case *ast.BinaryExpr:
			if n.Op == token.LAND || n.Op == token.LOR {
				complexity++
			}
		}
		return true
	})
	return complexity
}

// Score takes coverage as a fraction from 0 to 1.
func Score(complexity int, coverage float64) float64 {
	c := float64(complexity)
	return c*c*math.Pow(1-coverage, 3) + c
}
