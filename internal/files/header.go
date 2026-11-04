package files

import (
	"fmt"
	"go/build/constraint"
	"go/scanner"
	"go/token"
	"regexp"
)

// The pattern from https://go.dev/s/generatedcode.
var generatedHeader = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

// skippedByHeader reads every comment even after one rules the file out, because a //go:build line
// that doesn't parse is an error in any file that the header alone would skip. A +build line that
// doesn't parse is no error, because go/build ignores it too.
func skippedByHeader(file string, source []byte, tags []string) (bool, error) {
	skipped := false
	for _, comment := range headerComments(source) {
		if generatedHeader.MatchString(comment.text) {
			skipped = true
		}
		goBuild := constraint.IsGoBuild(comment.text)
		if !goBuild && !constraint.IsPlusBuild(comment.text) {
			continue
		}
		expr, err := constraint.Parse(comment.text)
		switch {
		case err == nil:
			skipped = skipped || requiresTag(expr, tags)
		case goBuild:
			return false, fmt.Errorf("%s:%d: %w", file, comment.line, err)
		}
	}
	return skipped, nil
}

type headerComment struct {
	text string
	line int
}

// headerComments uses the scanner, not go/parser, because a skipped file need not parse.
// An ignored template can break even its package clause, and go/parser then drops the comments.
func headerComments(source []byte) []headerComment {
	var comments []headerComment
	var s scanner.Scanner
	fset := token.NewFileSet()
	s.Init(fset.AddFile("", fset.Base(), len(source)), source, nil, scanner.ScanComments)
	for {
		pos, tok, text := s.Scan()
		if tok != token.COMMENT {
			return comments
		}
		// The scanner applies //line directives, and an error must point into the file on disk.
		comments = append(comments, headerComment{text, fset.PositionFor(pos, false).Line})
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
