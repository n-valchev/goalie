package check

import (
	"fmt"
	"math"

	"github.com/n-valchev/goalie/internal/config"
	"github.com/n-valchev/goalie/internal/crap"
	"github.com/n-valchev/goalie/internal/extract"
)

type estimate struct {
	File           string  `json:"file"`
	Line           int     `json:"line"`
	Function       string  `json:"function"`
	Complexity     int     `json:"complexity"`
	CanPass        bool    `json:"canPass"`
	NeededCoverage float64 `json:"neededCoverage,omitempty"`
	Limit          float64 `json:"limit"`
}

func (c Crap) estimates(functions []extract.Function) []Result {
	results := []Result{}
	for _, fn := range functions {
		complexity, limit := crap.Complexity(fn.Decl), c.Limits.For(fn.File)
		needed, canPass := crap.CoverageNeeded(complexity, limit)
		if canPass && needed == 0 {
			continue
		}
		// Up, because a value rounded down could still score over the limit.
		found := estimate{fn.File, fn.StartLine, name(fn), complexity, canPass, math.Ceil(needed*100) / 100, limit}
		results = append(results, found.result())
	}
	return results
}

func (e estimate) result() Result {
	result := Result{Rule: CrapRule, Value: float64(e.Complexity), Limit: e.Limit, File: e.File, Line: e.Line, Details: e}
	if !e.CanPass {
		result.Message = fmt.Sprintf("%s can't pass at any coverage, so split it (complexity %d, limit %g)",
			e.Function, e.Complexity, e.Limit)
		return result
	}
	// Warn in every mode: the function may already have this coverage, and only a test run can tell.
	result.Mode = config.ModeWarn
	result.Message = fmt.Sprintf("%s needs at least %.0f%% coverage (complexity %d, limit %g)",
		e.Function, e.NeededCoverage*100, e.Complexity, e.Limit)
	return result
}
