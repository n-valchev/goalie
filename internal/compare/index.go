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
		for printed := range candidate.Fingerprints {
			counts[printed]++
		}
	}
	prints := make([]string, 0, len(counts))
	for printed := range counts {
		prints = append(prints, printed)
	}
	sort.Slice(prints, func(i, j int) bool {
		a, b := prints[i], prints[j]
		if counts[a] != counts[b] {
			return counts[a] < counts[b]
		}
		return a < b
	})
	ids := make(map[string]int32, len(prints))
	for id, printed := range prints {
		ids[printed] = int32(id)
	}
	sets := make([][]int32, len(all))
	for i, candidate := range all {
		set := make([]int32, 0, len(candidate.Fingerprints))
		for printed := range candidate.Fingerprints {
			set = append(set, ids[printed])
		}
		slices.Sort(set)
		sets[i] = set
	}
	return sets
}

// findMatches looks up partners only for the functions it must check, so new-code mode costs in
// proportion to the new functions, after one pass to build the index.
func findMatches(sets [][]int32, newFunctions NewFunctions, threshold float64) []match {
	visitPartners := partnerVisitor(sets, threshold)
	matches := []match{}
	seen := make([]int, len(sets))
	for i := range sets {
		seen[i] = -1
	}
	for i, set := range sets {
		if !newFunctions.checked(i) {
			continue
		}
		visitPartners(set, func(j int) {
			// Partners are symmetric, so a pair of two checked functions belongs to the lower one.
			if seen[j] == i || j == i || (newFunctions.checked(j) && j < i) {
				return
			}
			seen[j] = i
			if !sizesAllow(len(set), len(sets[j]), threshold) {
				return
			}
			if score := similarity(set, sets[j]); score >= threshold {
				matches = append(matches, match{min(i, j), max(i, j), score})
			}
		})
	}
	return matches
}

func partnerVisitor(sets [][]int32, threshold float64) func(set []int32, visit func(int)) {
	// At threshold 0 a pair that shares nothing still qualifies, and no index can find it.
	if threshold <= 0 {
		return func(_ []int32, visit func(int)) {
			for j := range sets {
				visit(j)
			}
		}
	}
	index := map[int32][]int32{}
	for i, set := range sets {
		for _, id := range prefix(set, threshold) {
			index[id] = append(index[id], int32(i))
		}
	}
	return func(set []int32, visit func(int)) {
		for _, id := range prefix(set, threshold) {
			for _, j := range index[id] {
				visit(int(j))
			}
		}
	}
}

// prefix returns the rarest fingerprints of a set, which are all that the index must hold.
// A qualifying pair shares at least minShared fingerprints. The rarest of those has the others
// after it in both sets, because both use one rarity order, so it lies in both prefixes. Indexing
// only the prefix keeps out the fingerprints that every function has, which would otherwise make
// every pair a partner.
func prefix(set []int32, threshold float64) []int32 {
	return set[:len(set)-minShared(len(set), threshold)+1]
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
