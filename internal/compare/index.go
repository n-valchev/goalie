package compare

import (
	"math"
	"slices"
	"sort"
)

// match is a pair of indexes into the candidates, with first < second.
type match struct {
	first, second int
	score         float64
}

// integerSets maps each fingerprint to one integer, so equal integers mean equal fingerprints.
// Integers rise from the rarest fingerprint to the most common, because the index needs each
// sorted set to start with the fingerprints that few functions share.
func integerSets(all []Candidate) [][]int32 {
	counts := map[string]int{}
	for _, candidate := range all {
		for print := range candidate.Fingerprints {
			counts[print]++
		}
	}
	prints := make([]string, 0, len(counts))
	for print := range counts {
		prints = append(prints, print)
	}
	sort.Slice(prints, func(i, j int) bool {
		a, b := prints[i], prints[j]
		if counts[a] != counts[b] {
			return counts[a] < counts[b]
		}
		return a < b
	})
	ids := make(map[string]int32, len(prints))
	for id, print := range prints {
		ids[print] = int32(id)
	}
	sets := make([][]int32, len(all))
	for i, candidate := range all {
		set := make([]int32, 0, len(candidate.Fingerprints))
		for print := range candidate.Fingerprints {
			set = append(set, ids[print])
		}
		slices.Sort(set)
		sets[i] = set
	}
	return sets
}

// findMatches returns every pair that scores at or above threshold, and at least one new function
// in each pair when newFunctions is non-nil. Only the functions it must check look up candidates,
// so new-code mode costs in proportion to the new functions, after one pass to build the index.
func findMatches(sets [][]int32, newFunctions map[int]bool, threshold float64) []match {
	checked := func(i int) bool { return newFunctions == nil || newFunctions[i] }
	index := buildIndex(sets, threshold)
	matches := []match{}
	seen := make([]int, len(sets))
	for i := range sets {
		seen[i] = -1
	}
	for i, set := range sets {
		if !checked(i) {
			continue
		}
		consider := func(j int) {
			// Candidates are symmetric, so a pair of two checked functions belongs to the lower one.
			if seen[j] == i || j == i || (checked(j) && j < i) {
				return
			}
			seen[j] = i
			if !sizesAllow(len(set), len(sets[j]), threshold) {
				return
			}
			if score := similarity(set, sets[j]); score >= threshold {
				matches = append(matches, match{min(i, j), max(i, j), score})
			}
		}
		// At threshold 0 a pair that shares nothing still qualifies, and no index can find it.
		if threshold <= 0 {
			for j := range sets {
				consider(j)
			}
			continue
		}
		for _, print := range set[:prefixLength(len(set), threshold)] {
			for _, j := range index[print] {
				consider(int(j))
			}
		}
	}
	return matches
}

// buildIndex lists, for each fingerprint, the functions that have it in their prefix.
func buildIndex(sets [][]int32, threshold float64) map[int32][]int32 {
	index := map[int32][]int32{}
	if threshold <= 0 {
		return index
	}
	for i, set := range sets {
		for _, print := range set[:prefixLength(len(set), threshold)] {
			index[print] = append(index[print], int32(i))
		}
	}
	return index
}

// prefixLength returns how many of a set's rarest fingerprints the index must hold.
// A qualifying pair shares at least minShared fingerprints. The rarest of those has the others
// after it in both sets, because both use one rarity order, so it lies in both prefixes. Indexing
// only the prefix keeps out the fingerprints that every function has, which would otherwise make
// every pair a candidate.
func prefixLength(size int, threshold float64) int {
	return size - minShared(size, threshold) + 1
}

// minShared returns the smallest count whose score against a set of this size reaches threshold.
// The loops correct the estimate with the same division as the score, because a count that
// rounds differently from the score would drop a pair exactly at the threshold.
func minShared(size int, threshold float64) int {
	shared := int(math.Ceil(threshold * float64(size)))
	for shared > 1 && float64(shared-1)/float64(size) >= threshold {
		shared--
	}
	for shared < size && float64(shared)/float64(size) < threshold {
		shared++
	}
	return max(shared, 1)
}

// sizesAllow reports whether the best score that two set sizes permit reaches threshold. The best
// case is the smaller set inside the larger one. Rounded division never reverses an order, so no
// real score can exceed this float.
func sizesAllow(a, b int, threshold float64) bool {
	return float64(min(a, b))/float64(max(a, b)) >= threshold
}

func similarity(a, b []int32) float64 {
	shared := 0
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch {
		case a[i] < b[j]:
			i++
		case a[i] > b[j]:
			j++
		default:
			shared++
			i++
			j++
		}
	}
	return float64(shared) / float64(len(a)+len(b)-shared)
}
