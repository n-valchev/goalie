// Package compare finds pairs of functions at or above the similarity threshold.
package compare

import (
	"sort"

	"github.com/n-valchev/godry/internal/extract"
	"github.com/n-valchev/godry/internal/fingerprint"
)

// Candidate is a function that passed the size limits.
type Candidate struct {
	extract.Function
	fingerprint.Result
}

type Side struct {
	Candidate
	New bool
}

// Pair describes one finding. Literals is nil when neither function has a literal to compare.
type Pair struct {
	Left, Right Side
	Score       float64
	Literals    *float64
	Anchors     int
}

// NewFunctions decides which pairs a comparison reports. Build one with WholeRepo or Only.
type NewFunctions struct {
	wholeRepo bool
	indexes   map[int]bool
}

func WholeRepo() NewFunctions {
	return NewFunctions{wholeRepo: true}
}

// Only reports the pairs with at least one of the candidates at indexes, on the left side.
func Only(indexes []int) NewFunctions {
	set := make(map[int]bool, len(indexes))
	for _, i := range indexes {
		set[i] = true
	}
	return NewFunctions{indexes: set}
}

func (n NewFunctions) Empty() bool {
	return len(n.indexes) == 0
}

func (n NewFunctions) checked(i int) bool {
	return n.wholeRepo || n.indexes[i]
}

// Find returns the pairs in all that score at or above threshold, in report order.
func Find(all []Candidate, newFunctions NewFunctions, threshold float64) []Pair {
	sets := integerSets(all)
	matches := findMatches(sets, newFunctions, threshold)
	// All-pairs order, so that report order never depends on how the index found a pair.
	sort.Slice(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		if a.first != b.first {
			return a.first < b.first
		}
		return a.second < b.second
	})
	pairs := make([]Pair, 0, len(matches))
	for _, match := range matches {
		if variants(all[match.first], all[match.second]) {
			continue
		}
		left := Side{all[match.first], newFunctions.indexes[match.first]}
		right := Side{all[match.second], newFunctions.indexes[match.second]}
		pair := orient(left, right, match.score)
		pair.Literals = literalScore(left.Literals, right.Literals)
		pair.Anchors = left.Calls.Shared(right.Calls)
		pairs = append(pairs, pair)
	}
	sort.Slice(pairs, func(i, j int) bool {
		a, b := pairs[i], pairs[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if !sameLocation(a.Left, b.Left) {
			return before(a.Left, b.Left)
		}
		return before(a.Right, b.Right)
	})
	return pairs
}

// WeakLimits marks a pair as weak when it is below both limits.
type WeakLimits struct {
	Anchors  int
	Literals float64
}

// weak needs both numbers to be low, because either one alone shows real copying: a copy with new
// data has low literals, and a pasted table has no calls. Nil literals are no evidence that the
// data differs, so they keep the pair.
func (l WeakLimits) weak(pair Pair) bool {
	return pair.Anchors < l.Anchors && pair.Literals != nil && *pair.Literals < l.Literals
}

// DropWeak returns the pairs that aren't weak, and the number it dropped.
func DropWeak(pairs []Pair, limits WeakLimits) ([]Pair, int) {
	kept := make([]Pair, 0, len(pairs))
	for _, pair := range pairs {
		if !limits.weak(pair) {
			kept = append(kept, pair)
		}
	}
	return kept, len(pairs) - len(kept)
}

func literalScore(a, b fingerprint.Set) *float64 {
	shared := a.Shared(b)
	either := len(a) + len(b) - shared
	if either == 0 {
		return nil
	}
	score := float64(shared) / float64(either)
	return &score
}

// variants reports whether two functions are one function built for different platforms.
func variants(a, b Candidate) bool {
	return a.Package == b.Package && a.Name == b.Name && a.Receiver == b.Receiver
}

func orient(a, b Side, score float64) Pair {
	swap := before(b, a)
	if a.New != b.New {
		swap = b.New
	}
	if swap {
		a, b = b, a
	}
	return Pair{Left: a, Right: b, Score: score}
}

func sameLocation(a, b Side) bool {
	return a.File == b.File && a.StartLine == b.StartLine
}

func before(a, b Side) bool {
	if a.File != b.File {
		return a.File < b.File
	}
	return a.StartLine < b.StartLine
}
