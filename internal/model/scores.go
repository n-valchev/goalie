package model

import (
	"slices"

	"github.com/n-valchev/goalie/internal/crap"
	"github.com/n-valchev/goalie/internal/toolreport"
)

// Grade is a rank, so the worse of two grades is the larger. Zero is no grade, which the model
// leaves out.
type Grade int

const (
	gradeGreen Grade = iota + 1
	gradeYellow
	gradeRed
)

func (g Grade) MarshalText() ([]byte, error) {
	return []byte([...]string{"", "green", "yellow", "red"}[g]), nil
}

// Score is a pointer on Op, so that an unscored op prints no fields and a real coverage of 0 stays.
type Score struct {
	Crap       float64 `json:"crap"`
	Complexity int     `json:"complexity"`
	Coverage   float64 `json:"coverage"`
}

// JoinCrap returns how many rows match no op.
func JoinCrap(result *Model, rows []toolreport.CrapRow, limits crap.Limits) int {
	ops := placeOps(result)
	for _, candidates := range ops {
		for _, placed := range candidates {
			// The author excused it in the code, so it is not the unmeasured code that red warns of.
			if !placed.op.Ignored {
				placed.op.Grade = gradeRed
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

// takeOp removes the op, because a row scores one function and the next row on the line needs the
// other one.
func takeOp(ops map[opPosition][]placedOp, row toolreport.CrapRow) *Op {
	key := opPosition{row.File, row.Line}
	candidates := ops[key]
	i := pickOp(candidates, func(name string) bool { return name == row.Function })
	if i < 0 {
		return nil
	}
	op := candidates[i].op
	ops[key] = slices.Delete(candidates, i, i+1)
	return op
}

func gradeClasses(result *Model) {
	for i := range result.Classes {
		class := &result.Classes[i]
		for _, op := range class.Ops {
			class.Grade = max(class.Grade, op.Grade)
		}
	}
}

func gradeBoxes(result *Model) {
	boxes := boxesByID(result)
	for _, class := range result.Classes {
		for box := boxes[class.Package]; box != nil; box = boxes[box.Parent] {
			box.Grade = max(box.Grade, class.Grade)
		}
	}
}

func grade(score, limit float64) Grade {
	switch {
	case score <= limit/2:
		return gradeGreen
	case score <= limit:
		return gradeYellow
	}
	return gradeRed
}
