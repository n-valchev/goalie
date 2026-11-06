package check

import (
	"github.com/n-valchev/goalie/internal/compare"
	"github.com/n-valchev/goalie/internal/gitchange"
)

func findNew(all []compare.Candidate, changes gitchange.Changes) compare.NewFunctions {
	var indexes []int
	for i, fn := range all {
		if changes.Touches(fn.File, fn.StartLine, fn.EndLine) {
			indexes = append(indexes, i)
		}
	}
	return compare.Only(indexes)
}
