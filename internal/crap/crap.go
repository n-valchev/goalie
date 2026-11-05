// Package crap computes the Change Risk Anti-Pattern score of a function.
package crap

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"math"

	"github.com/n-valchev/goalie/internal/files"
)

const DefaultLimit = 30.0

const IgnoreDirective = "//gocrap:ignore"

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

// CheckValues is apart from Check, because gocrap checks its flags before it has read the shared
// patterns from the settings.
func (l Limits) CheckValues() error {
	// Written so that NaN, which fails every comparison, is rejected too.
	if !(l.Limit >= 0) {
		return fmt.Errorf("invalid limit %v: must not be negative", l.Limit)
	}
	if l.SharedLimit != nil && !(*l.SharedLimit >= 0) {
		return fmt.Errorf("invalid shared limit %v: must not be negative", *l.SharedLimit)
	}
	return nil
}

// Check is shared, so that godeps never grades with limits that gocrap refuses to run with.
func (l Limits) Check() error {
	if err := l.CheckValues(); err != nil {
		return err
	}
	// Otherwise the stricter limit would quietly apply to nothing.
	if l.SharedLimit != nil && len(l.Shared) == 0 {
		return errors.New("sharedLimit needs shared patterns")
	}
	return nil
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
