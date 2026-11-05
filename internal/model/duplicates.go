package model

import (
	"cmp"
	"slices"

	"github.com/n-valchev/goalie/internal/toolreport"
)

// Duplicate is the other side of a godry pair. ID is the class of that side. It is left out when
// that side is not on the map, as with an omitted class or a test file.
type Duplicate struct {
	ID    string  `json:"id,omitempty"`
	Name  string  `json:"name"`
	File  string  `json:"file"`
	Line  int     `json:"line"`
	URL   string  `json:"url"`
	Score float64 `json:"score"`
}

type opPosition struct {
	file string
	line int
}

type placedOp struct {
	class string
	op    *Op
}

// JoinDuplicates sets the duplicates of the ops and the counts of the classes and boxes. It returns
// how many sides of the pairs match no op.
func JoinDuplicates(result *Model, source Source, pairs []toolreport.DryPair) int {
	ops := map[opPosition]placedOp{}
	for i := range result.Classes {
		class := &result.Classes[i]
		for j := range class.Ops {
			op := &class.Ops[j]
			ops[opPosition{op.File, op.Line}] = placedOp{class.ID, op}
		}
	}
	unmatched := 0
	join := func(side, other toolreport.DrySide, score float64) {
		found, ok := ops[positionOf(side)]
		if !ok {
			unmatched++
			return
		}
		// The op keeps its mark when the other side is off the map, because the copy still exists.
		entry := Duplicate{"", other.Function, other.File, other.StartLine, source.URL(other.File, other.StartLine), score}
		if target, ok := ops[positionOf(other)]; ok {
			entry.ID, entry.Name = target.class, target.op.Name
		}
		found.op.Duplicates = append(found.op.Duplicates, entry)
	}
	for _, pair := range pairs {
		join(pair.Left, pair.Right, pair.Score)
		join(pair.Right, pair.Left, pair.Score)
	}
	for _, placed := range ops {
		slices.SortFunc(placed.op.Duplicates, compareDuplicates)
	}
	countDuplicates(result)
	return unmatched
}

// countDuplicates writes a count of zero too, so that a reader can tell a clean box from a run with
// no report. A foreign class has no ops to count.
func countDuplicates(result *Model) {
	boxes := map[string]*Package{}
	for i := range result.Packages {
		box := &result.Packages[i]
		box.DuplicateCount = new(int)
		boxes[box.ID] = box
	}
	for i := range result.Classes {
		class := &result.Classes[i]
		if class.Foreign {
			continue
		}
		count := 0
		for _, op := range class.Ops {
			if len(op.Duplicates) > 0 {
				count++
			}
		}
		class.DuplicateCount = &count
		for id := class.Package; id != ""; id = boxes[id].Parent {
			*boxes[id].DuplicateCount += count
		}
	}
}

func positionOf(side toolreport.DrySide) opPosition {
	return opPosition{side.File, side.StartLine}
}

func compareDuplicates(a, b Duplicate) int {
	return cmp.Or(cmp.Compare(a.ID, b.ID), cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
}
