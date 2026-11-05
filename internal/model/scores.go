package model

import (
	"slices"

	"github.com/n-valchev/goalie/internal/crap"
	"github.com/n-valchev/goalie/internal/toolreport"
)

const (
	gradeGreen  = "green"
	gradeYellow = "yellow"
	gradeRed    = "red"
)

// Score is a pointer on Op, so that an unscored op prints no fields and a real coverage of 0 stays.
type Score struct {
	Crap       float64 `json:"crap"`
	Complexity int     `json:"complexity"`
	Coverage   float64 `json:"coverage"`
}

// JoinCrap sets the scores and grades and returns how many rows match no op.
func JoinCrap(result *Model, rows []toolreport.CrapRow, limits crap.Limits) int {
	ops := map[opPosition][]placedOp{}
	for i := range result.Classes {
		class := &result.Classes[i]
		for j := range class.Ops {
			if op := &class.Ops[j]; !op.NoBody {
				op.Grade = gradeRed
				key := opPosition{op.File, op.Line}
				ops[key] = append(ops[key], placedOp{class.ID, op})
			}
		}
	}
	unmatched := 0
	for _, row := range rows {
		op := takeOp(ops, row)
		if op == nil {
			unmatched++
			continue
		}
		op.Score = &Score{row.Score, row.Complexity, row.Coverage}
		op.Grade = grade(row.Score, limits.For(op.File))
	}
	gradeClasses(result)
	gradeBoxes(result)
	return unmatched
}

// takeOp matches on position, which avoids differences in naming. The name only tells apart two
// functions on one line, which is why a position holds a list here and not one op as in
// JoinDuplicates: there a pair is a mark, and here the wrong row would give a wrong score.
func takeOp(ops map[opPosition][]placedOp, row toolreport.CrapRow) *Op {
	key := opPosition{row.File, row.Line}
	candidates := ops[key]
	if len(candidates) == 0 {
		return nil
	}
	named := func(placed placedOp) bool { return placed.op.Name == row.Function }
	i := max(slices.IndexFunc(candidates, named), 0)
	op := candidates[i].op
	ops[key] = slices.Delete(candidates, i, i+1)
	return op
}

func gradeClasses(result *Model) {
	for i := range result.Classes {
		class := &result.Classes[i]
		for _, op := range class.Ops {
			class.Grade = worse(class.Grade, op.Grade)
		}
	}
}

func gradeBoxes(result *Model) {
	boxes := map[string]*Package{}
	for i := range result.Packages {
		boxes[result.Packages[i].ID] = &result.Packages[i]
	}
	for _, class := range result.Classes {
		for box := boxes[class.Package]; box != nil; box = boxes[box.Parent] {
			box.Grade = worse(box.Grade, class.Grade)
		}
	}
}

func worse(a, b string) string {
	order := []string{"", gradeGreen, gradeYellow, gradeRed}
	if slices.Index(order, a) > slices.Index(order, b) {
		return a
	}
	return b
}

func grade(score, limit float64) string {
	switch {
	case score <= limit/2:
		return gradeGreen
	case score <= limit:
		return gradeYellow
	}
	return gradeRed
}
