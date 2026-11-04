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
		left := Side{all[match.first], newFunctions[match.first]}
		right := Side{all[match.second], newFunctions[match.second]}
		pairs = append(pairs, orient(left, right, match.score))
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
