package check

import (
	"cmp"
	"fmt"
	"io"
	"math"
	"slices"

	"github.com/n-valchev/goalie/internal/crap"
	"github.com/n-valchev/goalie/internal/extract"
)

type estimate struct {
	file       string
	line       int
	function   string
	complexity int
	limit      float64
	needed     float64
	canPass    bool
}

// runPartial reports found only for a function that can't pass. A function that needs coverage may
// already have it, and only a test run can tell, so it must not fail a check in any mode.
func (c Crap) runPartial(functions []extract.Function, stdout io.Writer) (found bool, err error) {
	for _, result := range c.estimates(functions) {
		found = found || !result.canPass
		if err := result.write(stdout); err != nil {
			return false, err
		}
	}
	return found, nil
}

func (c Crap) estimates(functions []extract.Function) []estimate {
	results := []estimate{}
	for _, fn := range functions {
		complexity, limit := crap.Complexity(fn.Decl), c.Limits.For(fn.File)
		needed, canPass := crap.CoverageNeeded(complexity, limit)
		if canPass && needed == 0 {
			continue
		}
		results = append(results, estimate{fn.File, fn.StartLine, name(fn), complexity, limit, needed, canPass})
	}
	slices.SortStableFunc(results, func(a, b estimate) int {
		return cmp.Or(
			cmp.Compare(a.rank(), b.rank()),
			cmp.Compare(b.limitRatio(), a.limitRatio()),
			cmp.Compare(a.file, b.file),
			cmp.Compare(a.line, b.line),
		)
	})
	return results
}

// rank puts what can fail first. The ratio alone can't, because a limit of 0 counts as 1.
func (e estimate) rank() int {
	if e.canPass {
		return 1
	}
	return 0
}

// limitRatio counts a limit of 0 as 1, because a division by 0 would give no order.
func (e estimate) limitRatio() float64 {
	return float64(e.complexity) / cmp.Or(e.limit, 1)
}

func (e estimate) write(w io.Writer) error {
	if !e.canPass {
		_, err := fmt.Fprintf(w, "%s:%d: %s can't pass at any coverage, so split it (complexity %d, limit %g)\n",
			e.file, e.line, e.function, e.complexity, e.limit)
		return err
	}
	// Up, because a value rounded down could still score over the limit.
	_, err := fmt.Fprintf(w, "%s:%d: %s needs at least %.0f%% coverage (complexity %d, limit %g)\n",
		e.file, e.line, e.function, math.Ceil(e.needed*100), e.complexity, e.limit)
	return err
}
