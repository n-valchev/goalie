package model

import (
	"cmp"
	"slices"
	"strings"

	"github.com/n-valchev/goalie/internal/toolreport"
)

// Duplicate is the other side of a dup pair. ID is the class of that side. It is left out when
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

// placeOps leaves out assembly stubs, because neither the crap rule nor the dup rule can report one.
func placeOps(result *Model) map[opPosition][]placedOp {
	ops := map[opPosition][]placedOp{}
	for i := range result.Classes {
		class := &result.Classes[i]
		for j := range class.Ops {
			if op := &class.Ops[j]; !op.NoBody {
				key := opPosition{op.File, op.Line}
				ops[key] = append(ops[key], placedOp{class.ID, op})
			}
		}
	}
	return ops
}

// JoinDuplicates returns how many sides of the pairs match no op.
func JoinDuplicates(result *Model, source Source, pairs []toolreport.DryPair) int {
	ops := placeOps(result)
	unmatched := 0
	join := func(side, other toolreport.DrySide, score float64) {
		found, ok := sideOp(ops, side)
		if !ok {
			unmatched++
			return
		}
		// The op keeps its mark when the other side is off the map, because the copy still exists.
		entry := Duplicate{"", other.Function, other.File, other.StartLine, source.URL(other.File, other.StartLine), score}
		if target, ok := sideOp(ops, other); ok {
			entry.ID, entry.Name = target.class, target.op.Name
		}
		found.op.Duplicates = append(found.op.Duplicates, entry)
	}
	for _, pair := range pairs {
		join(pair.Left, pair.Right, pair.Score)
		join(pair.Right, pair.Left, pair.Score)
	}
	for _, candidates := range ops {
		for _, placed := range candidates {
			slices.SortFunc(placed.op.Duplicates, compareDuplicates)
		}
	}
	countDuplicates(result)
	return unmatched
}

// countDuplicates writes a count of zero too, so that a reader can tell a clean box from a run with
// no report. A foreign class has no ops to count.
func countDuplicates(result *Model) {
	boxes := boxesByID(result)
	for _, box := range boxes {
		box.DuplicateCount = new(int)
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

// sideOp leaves the op in place, because one function can be a side of many pairs. A side holds
// the bare name, and an op's name starts with its receiver.
func sideOp(ops map[opPosition][]placedOp, side toolreport.DrySide) (placedOp, bool) {
	candidates := ops[opPosition{side.File, side.StartLine}]
	i := pickOp(candidates, func(name string) bool { return name[strings.LastIndex(name, ".")+1:] == side.Function })
	if i < 0 {
		return placedOp{}, false
	}
	return candidates[i], true
}

// pickOp matches on position, which avoids differences in naming. The name only tells apart two
// functions on one line. It returns -1 for an empty list.
func pickOp(candidates []placedOp, named func(name string) bool) int {
	if len(candidates) == 0 {
		return -1
	}
	return max(slices.IndexFunc(candidates, func(placed placedOp) bool { return named(placed.op.Name) }), 0)
}

func compareDuplicates(a, b Duplicate) int {
	return cmp.Or(cmp.Compare(a.ID, b.ID), cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
}
