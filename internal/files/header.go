package files

import (
	"go/build/constraint"
	"go/scanner"
	"go/token"
	"regexp"
)

// The pattern from https://go.dev/s/generatedcode.
var generatedHeader = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

// skippedByHeader reports whether the comments above the package clause rule the file out.
func skippedByHeader(source []byte, tags []string) bool {
	for _, comment := range headerComments(source) {
		if generatedHeader.MatchString(comment) {
			return true
		}
		if !constraint.IsGoBuild(comment) && !constraint.IsPlusBuild(comment) {
			continue
		}
		// A line that doesn't parse can't show that a tag is required, so the file stays scanned.
		if expr, err := constraint.Parse(comment); err == nil && requiresTag(expr, tags) {
			return true
		}
	}
	return false
}

// headerComments uses the scanner, not go/parser, because a skipped file need not parse.
// An ignored template can break even its package clause, and go/parser then drops the comments.
func headerComments(source []byte) []string {
	var comments []string
	var s scanner.Scanner
	fset := token.NewFileSet()
	s.Init(fset.AddFile("", fset.Base(), len(source)), source, nil, scanner.ScanComments)
	for {
		_, tok, text := s.Scan()
		if tok != token.COMMENT {
			return comments
		}
		comments = append(comments, text)
	}
}

// requiresTag reports whether expr can hold, but not while every one of tags is off.
// The first check keeps a line that can never hold, such as `linux && !linux`, from counting.
func requiresTag(expr constraint.Expr, tags []string) bool {
	off := map[string]bool{}
	for _, tag := range tags {
		off[tag] = false
	}
	return satisfiable(expr, map[string]bool{}) && !satisfiable(expr, off)
}

// satisfiable searches the values of the tags missing from known. One evaluation can't answer
// this: with all other tags on, `!windows` looks unbuildable, and with all off, `linux` does.
func satisfiable(expr constraint.Expr, known map[string]bool) bool {
	value, open := evaluate(expr, known)
	if open == "" {
		return value
	}
	defer delete(known, open)
	for _, guess := range []bool{true, false} {
		known[open] = guess
		if satisfiable(expr, known) {
			return true
		}
	}
	return false
}

// evaluate returns the value of expr, or the name of a tag that must be known first.
func evaluate(expr constraint.Expr, known map[string]bool) (value bool, open string) {
	switch expr := expr.(type) {
	case *constraint.TagExpr:
		value, ok := known[expr.Tag]
		if !ok {
			return false, expr.Tag
		}
		return value, ""
	case *constraint.NotExpr:
		value, open := evaluate(expr.X, known)
		return !value, open
	case *constraint.AndExpr:
		return evaluateBoth(expr.X, expr.Y, known, false)
	case *constraint.OrExpr:
		return evaluateBoth(expr.X, expr.Y, known, true)
	}
	panic("unknown build constraint expression")
}

// evaluateBoth combines two operands. Either one alone settles the result if it equals decisive.
func evaluateBoth(x, y constraint.Expr, known map[string]bool, decisive bool) (value bool, open string) {
	xValue, xOpen := evaluate(x, known)
	yValue, yOpen := evaluate(y, known)
	switch {
	case xOpen == "" && xValue == decisive, yOpen == "" && yValue == decisive:
		return decisive, ""
	case xOpen != "":
		return false, xOpen
	case yOpen != "":
		return false, yOpen
	}
	return !decisive, ""
}
