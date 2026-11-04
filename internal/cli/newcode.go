package cli

import (
	"github.com/n-valchev/godry/internal/compare"
	"github.com/n-valchev/godry/internal/gitchange"
)

// newFunctions never returns nil, because compare.Find reads nil as whole-repo mode.
func newFunctions(all []compare.Candidate, changes gitchange.Changes) map[int]bool {
	result := map[int]bool{}
	for i, fn := range all {
		if changes.Touches(fn.File, fn.StartLine, fn.EndLine) {
			result[i] = true
		}
	}
	return result
}
