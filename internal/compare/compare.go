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
	Fingerprints fingerprint.Set
	Nodes        int
}

// Side is one function of a pair.
type Side struct {
	Candidate
	New bool
}

// Pair is two functions with the same structure.
type Pair struct {
	Left, Right Side
	Score       float64
}

// Find returns the pairs in all that score at or above threshold, in report order.
// newFunctions holds indexes into all. When it is non-nil, only pairs with at least one new
// function are returned, and the new function is the left side. A nil set means whole-repo mode.
func Find(all []Candidate, newFunctions map[int]bool, threshold float64) []Pair {
	pairs := []Pair{}
	for i := range all {
		for j := i + 1; j < len(all); j++ {
			if newFunctions != nil && !newFunctions[i] && !newFunctions[j] {
				continue
			}
			score := similarity(all[i].Fingerprints, all[j].Fingerprints)
			if score >= threshold {
				left := Side{all[i], newFunctions[i]}
				right := Side{all[j], newFunctions[j]}
				pairs = append(pairs, orient(left, right, score))
			}
		}
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

func similarity(a, b fingerprint.Set) float64 {
	shared := 0
	for print := range a {
		if _, ok := b[print]; ok {
			shared++
		}
	}
	return float64(shared) / float64(len(a)+len(b)-shared)
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
