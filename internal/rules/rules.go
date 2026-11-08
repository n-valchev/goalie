// Package rules holds the measures of the function rules. It works on parsed Go code and knows
// nothing about Git, settings, or output, which internal/check adds once for every rule.
package rules

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/n-valchev/goalie/internal/extract"
)

// Lines counts blank lines and comments too, because an agent reads them all.
func Lines(fn extract.Function) int {
	return fn.Lines()
}

// Parameters counts an unnamed parameter as 1, so that func(int, string) is 2, as it is to a caller.
func Parameters(fn extract.Function) int {
	count := 0
	for _, field := range fn.Decl.Type.Params.List {
		count += max(len(field.Names), 1)
	}
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
