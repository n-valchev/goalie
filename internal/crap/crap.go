// Package crap computes the Change Risk Anti-Pattern score of a function.
package crap

import (
	"go/ast"
	"go/token"
	"math"
)

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
